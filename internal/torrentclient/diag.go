package torrentclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	alog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/tracker"
)

// Diag says how far a torrent got in finding anyone to download from. It is
// what turns "nobody answered" into something a person can act on.
type Diag struct {
	// TrackersTried is how many trackers the torrent lists.
	TrackersTried int
	// TrackersAnswered is how many of them replied to an announce.
	TrackersAnswered int
	// TrackersFailed is how many gave an error or none at all.
	TrackersFailed int
	// PeersFound is how many peer addresses are known (from trackers or other peers).
	PeersFound int
	// PeersConnected is how many peers are connected right now.
	PeersConnected int
	// Seeds is how many of those have the whole torrent.
	Seeds int
}

// Summary is the counts in plain words, for example
// "1 of 3 trackers answered, 12 peers found, 0 connected".
func (d Diag) Summary() string {
	trackers := "no trackers listed"
	if d.TrackersTried > 0 {
		trackers = fmt.Sprintf("%d of %d %s answered", d.TrackersAnswered, d.TrackersTried, plural(d.TrackersTried, "tracker", "trackers"))
	}
	return fmt.Sprintf("%s, %d %s found, %d connected", trackers, d.PeersFound, plural(d.PeersFound, "peer", "peers"), d.PeersConnected)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Diagnose reads the numbers from the running torrent and from the tracker
// answers the engine logged for it.
func Diagnose(t *torrent.Torrent) Diag {
	st := t.Stats()
	d := Diag{PeersFound: st.TotalPeers, PeersConnected: st.ActivePeers, Seeds: st.ConnectedSeeders}
	listed := map[string]struct{}{}
	mi := t.Metainfo()
	for _, tier := range mi.UpvertedAnnounceList() {
		for _, u := range tier {
			listed[trackerName(u)] = struct{}{}
		}
	}
	answered, failed := announces.counts(t.InfoHash().HexString())
	d.TrackersTried = len(listed)
	if n := answered + failed; n > d.TrackersTried {
		d.TrackersTried = n
	}
	d.TrackersAnswered, d.TrackersFailed = answered, failed
	return d
}

// DiagError is implemented by the errors that say why a torrent found no one.
type DiagError interface {
	error
	Diagnosis() Diag
}

// NoMetadataError is ErrNoMetadata with what was tried.
type NoMetadataError struct {
	Waited time.Duration
	Diag   Diag
}

func (e *NoMetadataError) Error() string {
	return fmt.Sprintf("no metadata after %s: no peer sent the torrent's details (%s)", waitText(e.Waited), e.Diag.Summary())
}
func (e *NoMetadataError) Is(target error) bool { return target == ErrNoMetadata }
func (e *NoMetadataError) Diagnosis() Diag      { return e.Diag }

// StalledError is ErrStalled with what was tried.
type StalledError struct {
	Waited time.Duration
	Diag   Diag
}

func (e *StalledError) Error() string {
	return fmt.Sprintf("no data for %s: no one seems to be sharing it (%s)", waitText(e.Waited), e.Diag.Summary())
}
func (e *StalledError) Is(target error) bool { return target == ErrStalled }
func (e *StalledError) Diagnosis() Diag      { return e.Diag }

// waitText writes a wait as "10 minutes" or "2 hours".
func waitText(d time.Duration) string {
	switch {
	case d >= time.Hour && d%time.Hour == 0:
		h := int(d / time.Hour)
		return fmt.Sprintf("%d %s", h, plural(h, "hour", "hours"))
	case d >= time.Minute:
		m := int(d / time.Minute)
		return fmt.Sprintf("%d %s", m, plural(m, "minute", "minutes"))
	default:
		s := int(d / time.Second)
		return fmt.Sprintf("%d %s", s, plural(s, "second", "seconds"))
	}
}

// trackerName is a tracker address without its path and query, which can hold
// a private passkey. It is all that is ever kept or logged of a tracker.
func trackerName(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "tracker"
	}
	// The engine announces over udp4 and udp6 as two trackers; they are one.
	scheme := u.Scheme
	if strings.HasPrefix(scheme, "udp") {
		scheme = "udp"
	}
	return scheme + "://" + u.Host
}

// announceBook remembers, per torrent, which trackers answered an announce and
// which did not. The engine only reports this through its log, so the log
// output handler (see SetLogOutput) feeds it.
type announceBook struct {
	mu   sync.Mutex
	byIH map[string]*announceNotes
}

type announceNotes struct {
	answered map[string]int    // tracker -> peers it sent last time
	failed   map[string]string // tracker -> short reason
	touched  time.Time
}

// announceBookMax stops the book growing if torrents are not forgotten.
const announceBookMax = 512

var announces = &announceBook{byIH: map[string]*announceNotes{}}

func (b *announceBook) record(ih, tracker string, peers int, failure string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.byIH[ih]
	if n == nil {
		if len(b.byIH) >= announceBookMax {
			b.dropOldestLocked()
		}
		n = &announceNotes{answered: map[string]int{}, failed: map[string]string{}}
		b.byIH[ih] = n
	}
	n.touched = time.Now()
	if failure == "" {
		n.answered[tracker] = peers
		delete(n.failed, tracker)
	} else if _, ok := n.answered[tracker]; !ok {
		n.failed[tracker] = failure
	}
}

func (b *announceBook) dropOldestLocked() {
	var oldest string
	var at time.Time
	for ih, n := range b.byIH {
		if oldest == "" || n.touched.Before(at) {
			oldest, at = ih, n.touched
		}
	}
	delete(b.byIH, oldest)
}

func (b *announceBook) forget(ih string) {
	b.mu.Lock()
	delete(b.byIH, ih)
	b.mu.Unlock()
}

// counts says how many trackers answered and how many failed
func (b *announceBook) counts(ih string) (answered, failed int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.byIH[ih]
	if n == nil {
		return 0, 0
	}
	return len(n.answered), len(n.failed)
}

// failures lists the trackers that failed, as "host (reason)", sorted.
func (b *announceBook) failures(ih string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.byIH[ih]
	if n == nil {
		return nil
	}
	out := make([]string, 0, len(n.failed))
	for tr, why := range n.failed {
		out = append(out, tr+" ("+why+")")
	}
	sort.Strings(out)
	return out
}

// announceFailure puts an announce error in a few safe words. The raw text can
// hold the tracker's address with a passkey, so it is never kept.
func announceFailure(err error) string {
	low := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(low, "timeout"), strings.Contains(low, "timed out"):
		return "no answer in time"
	case strings.Contains(low, "refused"):
		return "refused the connection"
	case strings.Contains(low, "no such host"), strings.Contains(low, "lookup"):
		return "address not found"
	case strings.Contains(low, "unreachable"):
		return "unreachable"
	case strings.Contains(low, "status 4"), strings.Contains(low, "status 5"), strings.Contains(low, "http 4"), strings.Contains(low, "http 5"):
		return "answered with an error"
	default:
		return "failed"
	}
}

// tapAnnounce reads the engine's "announced" log line (one per tracker
// announce) and notes the outcome for the torrent it was for.
func tapAnnounce(r alog.Record) {
	if r.Text() != "announced" {
		return
	}
	rec, ok := r.SlogRecord().AsTuple()
	if !ok {
		return
	}
	var ih, tr, failure string
	var peers int
	rec.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "short infohash":
			ih = strings.ToLower(a.Value.String())
		case "url":
			tr = trackerName(a.Value.String())
		case "err":
			if err, isErr := a.Value.Any().(error); isErr && err != nil {
				failure = announceFailure(err)
			}
		case "resp":
			switch v := a.Value.Any().(type) {
			case tracker.AnnounceResponse:
				peers = len(v.Peers)
			case *tracker.AnnounceResponse:
				if v != nil {
					peers = len(v.Peers)
				}
			}
		}
		return true
	})
	if len(ih) != 40 || tr == "" {
		return
	}
	announces.record(ih, tr, peers, failure)
}

// tapHandler lets tapAnnounce look at every engine record, then passes on to
// next only what the engine's own logger passed before: warnings and worse.
// The engine writes a tracker's answer as a debug line, which is dropped by
// default, so the engine runs with a logger that lets debug lines through to
// this handler (see engineLogger).
type tapHandler struct{ next alog.Handler }

func (h tapHandler) Handle(r alog.Record) {
	tapAnnounce(r)
	if !r.Level.LessThan(alog.Warning) {
		h.next.Handle(r)
	}
}

// engineLogger is the logger the torrent engine uses: the global one, with
// the same output, plus the tracker tap.
func engineLogger() alog.Logger {
	lg := alog.Default.FilterLevel(alog.Debug)
	handlers := make([]alog.Handler, 0, len(alog.Default.Handlers))
	for _, h := range alog.Default.Handlers {
		handlers = append(handlers, tapHandler{next: h})
	}
	lg.SetHandlers(handlers...)
	return lg
}

// Debug-log rates: the first line comes shortly after a torrent starts, then
// one a minute while it waits for its details, one every five minutes while
// it downloads. A busy engine never logs more than that per torrent.
const (
	waitLogFirst    = 15 * time.Second
	waitLogEvery    = time.Minute
	downloadLogEach = 5 * time.Minute
)

// logSwarm writes one debug line on how a torrent is doing in finding peers.
func logSwarm(msg string, t *torrent.Torrent, extra ...any) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	d := Diagnose(t)
	args := []any{
		"torrent", t.Name(),
		"trackers", d.TrackersTried, "trackersAnswered", d.TrackersAnswered, "trackersFailed", d.TrackersFailed,
		"peersFound", d.PeersFound, "peersConnected", d.PeersConnected, "seeds", d.Seeds,
	}
	if f := announces.failures(t.InfoHash().HexString()); len(f) > 0 {
		if len(f) > 6 {
			f = append(f[:6], fmt.Sprintf("and %d more", len(f)-6))
		}
		args = append(args, "trackerProblems", strings.Join(f, ", "))
	}
	slog.Debug(msg, append(args, extra...)...)
}
