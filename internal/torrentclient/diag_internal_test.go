package torrentclient

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	alog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent/tracker"
	"github.com/anacrolix/torrent/types/infohash"
)

func TestDiagSummary(t *testing.T) {
	tests := []struct {
		name string
		d    Diag
		want string
	}{
		{"nothing at all", Diag{}, "no trackers listed, 0 peers found, 0 connected"},
		{"trackers silent", Diag{TrackersTried: 3}, "0 of 3 trackers answered, 0 peers found, 0 connected"},
		{"one tracker", Diag{TrackersTried: 1, TrackersAnswered: 1, PeersFound: 1}, "1 of 1 tracker answered, 1 peer found, 0 connected"},
		{"peers but no handshake", Diag{TrackersTried: 4, TrackersAnswered: 2, PeersFound: 12, PeersConnected: 3}, "2 of 4 trackers answered, 12 peers found, 3 connected"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.d.Summary(); got != tc.want {
				t.Fatalf("Summary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFailureErrorsKeepTheirSentinelAndSayWhatWasTried(t *testing.T) {
	d := Diag{TrackersTried: 3}
	meta := error(&NoMetadataError{Waited: 10 * time.Minute, Diag: d})
	if !errors.Is(meta, ErrNoMetadata) || errors.Is(meta, ErrStalled) {
		t.Fatalf("NoMetadataError must match ErrNoMetadata only: %v", meta)
	}
	if got, want := meta.Error(), "no metadata after 10 minutes: no peer sent the torrent's details (0 of 3 trackers answered, 0 peers found, 0 connected)"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	stall := error(&StalledError{Waited: 2 * time.Hour, Diag: d})
	if !errors.Is(stall, ErrStalled) || errors.Is(stall, ErrNoMetadata) {
		t.Fatalf("StalledError must match ErrStalled only: %v", stall)
	}
	if !strings.HasPrefix(stall.Error(), "no data for 2 hours: ") {
		t.Fatalf("message = %q", stall.Error())
	}
	var de DiagError
	if !errors.As(meta, &de) || de.Diagnosis().TrackersTried != 3 {
		t.Fatal("the diagnosis must be reachable through errors.As")
	}
}

func TestWaitText(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Minute, "10 minutes"}, {time.Minute, "1 minute"}, {2 * time.Hour, "2 hours"},
		{time.Hour, "1 hour"}, {90 * time.Minute, "90 minutes"}, {5 * time.Second, "5 seconds"},
	}
	for _, tc := range tests {
		if got := waitText(tc.d); got != tc.want {
			t.Errorf("waitText(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestTrackerNameDropsThePasskey(t *testing.T) {
	tests := []struct{ in, want string }{
		{"udp://tracker.example:6969/announce", "udp://tracker.example:6969"},
		{"https://t.example/abcdef0123456789/announce?passkey=secret", "https://t.example"},
		{"udp6://tracker.example:6969/announce", "udp://tracker.example:6969"},
		{"not a url", "tracker"},
	}
	for _, tc := range tests {
		if got := trackerName(tc.in); got != tc.want {
			t.Errorf("trackerName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestAnnounceFailure(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{errors.New(`Get "http://t.example/announce?passkey=secret": dial tcp: i/o timeout`), "no answer in time"},
		{errors.New("dial tcp 1.2.3.4:80: connect: connection refused"), "refused the connection"},
		{errors.New("lookup t.example: no such host"), "address not found"},
		{errors.New("tracker returned status 503"), "answered with an error"},
		{errors.New("weird"), "failed"},
	}
	for _, tc := range tests {
		got := announceFailure(tc.err)
		if got != tc.want || strings.Contains(got, "secret") {
			t.Errorf("announceFailure(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// The tap reads the engine's own "announced" log line, in the form the engine
// really writes it (through the global logger's slog front), and keeps only
// safe words.
func TestTapReadsTrackerAnnounces(t *testing.T) {
	saved := alog.Default.Handlers
	defer func() { alog.Default.Handlers = saved }()
	var buf bytes.Buffer
	SetLogOutput(&buf)
	engine := engineLogger()

	var ih infohash.T
	copy(ih[:], []byte("0123456789abcdefghij"))
	hexIH := strings.ToLower(ih.HexString())
	defer announces.forget(hexIH)

	log := engine.Slogger()
	log.Debug("announced", "short infohash", ih, "url", "udp://one.example:80/announce", "resp", tracker.AnnounceResponse{Peers: make([]tracker.Peer, 4)}, "err", error(nil))
	log.Error("announced", "short infohash", ih, "url", "https://two.example/key123/announce?passkey=secret", "resp", tracker.AnnounceResponse{}, "err", errors.New("dial tcp: i/o timeout"))
	log.Debug("something else", "short infohash", ih, "url", "udp://three.example:80/announce")

	answered, failed := announces.counts(hexIH)
	if answered != 1 || failed != 1 {
		t.Fatalf("answered, failed = %d, %d, want 1, 1", answered, failed)
	}
	fails := announces.failures(hexIH)
	if len(fails) != 1 || fails[0] != "https://two.example (no answer in time)" {
		t.Fatalf("failures = %q", fails)
	}
	// Only the error line reaches the log output, as before: the debug ones
	// are for the tap alone.
	if got := buf.String(); strings.Count(got, "announced") != 1 || !strings.Contains(got, "torrent engine [") {
		t.Fatalf("log output: %q", got)
	}

	// A tracker that failed first and answered later counts as answered.
	log.Debug("announced", "short infohash", ih, "url", "https://two.example/key123/announce?passkey=secret", "resp", tracker.AnnounceResponse{}, "err", error(nil))
	if answered, failed = announces.counts(hexIH); answered != 2 || failed != 0 {
		t.Fatalf("after recovery: %d, %d, want 2, 0", answered, failed)
	}
}

func TestAnnounceBookIsBounded(t *testing.T) {
	b := &announceBook{byIH: map[string]*announceNotes{}}
	for i := 0; i < announceBookMax+50; i++ {
		b.record(strings.Repeat("a", 36)+string(rune('a'+i%26))+string(rune('a'+i/26%26))+string(rune('a'+i/676%26))+"a", "udp://t.example:1", 0, "")
	}
	if len(b.byIH) > announceBookMax {
		t.Fatalf("book holds %d torrents, limit %d", len(b.byIH), announceBookMax)
	}
}
