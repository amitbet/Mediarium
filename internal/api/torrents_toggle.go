package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/queue"
	"github.com/rdborg/mediarium/internal/settings"
)

// errTorrentsDisabled is what a torrent grab is refused with while the
// torrent switch (Settings > Downloading > Torrents) is off. It is written for the person
// reading it, so it is passed to the UI as is.
var errTorrentsDisabled = errors.New("Torrents are switched off. Switch them on in Settings > Downloading > Torrents to download torrent releases.")

// codeTorrentsOff is the stable code sent beside errTorrentsDisabled, so the
// page can offer a "Switch on torrents" button without matching the words
// (which are translated).
const codeTorrentsOff = "torrents_off"

// errorCodeFor is the stable "code" that goes with an error message, for the
// few errors the page does more with than show: "" for all the others.
func errorCodeFor(message string) string {
	if strings.Contains(message, errTorrentsDisabled.Error()) {
		return codeTorrentsOff
	}
	return ""
}

// torrentsEnabled reports the torrent on/off switch. On unless explicitly
// turned off, so upgrading never changes anyone's behavior underneath them.
// torrentReleasesOn reports whether torrent releases may be searched for and
// grabbed at all: with the built-in client on, or a debrid service or an
// external download client to send them to.
func (s *Server) torrentReleasesOn() bool {
	return s.torrentsEnabled() || s.debridOn() || s.downloadClientOn()
}

func (s *Server) torrentsEnabled() bool {
	v, _ := s.Settings.Get(settings.KeyTorrentEnabled)
	return v != "0"
}

// setTorrentsEnabled stores the switch and, when turning torrents off, shuts
// every running torrent client down (downloads in flight fail with
// errTorrentsDisabled; seeding stops).
func (s *Server) setTorrentsEnabled(enabled bool) error {
	t := &settingsSave{}
	s.stageTorrentsEnabled(t, enabled)
	return s.commitSettings(t)
}

// searchableIndexers is the indexer list searches and automation use: with
// nowhere to send torrents (the built-in client off and no debrid service or
// download client), torrent indexers are skipped so they are never queried.
func (s *Server) searchableIndexers() ([]indexers.Instance, error) {
	all, err := s.IndexerRepo.List()
	if err != nil {
		logIndexerTrouble("list", "Mediarium couldn't read the list of indexers, so automatic searches are skipped: "+err.Error())
		return nil, err
	}
	torrentsOn := s.torrentReleasesOn()
	out := make([]indexers.Instance, 0, len(all))
	for _, inst := range all {
		if inst.SecretError != "" {
			// Only the broken one is left out; the others keep working.
			logIndexerTrouble(fmt.Sprint(inst.ID), inst.SecretError)
			continue
		}
		if inst.Protocol == indexers.ProtocolTorrent && !torrentsOn {
			continue
		}
		out = append(out, inst)
	}
	return out, nil
}

var (
	indexerTroubleMu   sync.Mutex
	indexerTroubleSeen = map[string]time.Time{}
)

// logIndexerTrouble writes a line about an indexer that can't be used, at most
// once an hour for the same one, so a search that runs every few minutes does
// not fill the log.
func logIndexerTrouble(key, msg string) {
	indexerTroubleMu.Lock()
	defer indexerTroubleMu.Unlock()
	if at, ok := indexerTroubleSeen[key]; ok && time.Since(at) < time.Hour {
		return
	}
	indexerTroubleSeen[key] = time.Now()
	log.Print("indexers: " + msg)
}

// writeGrabError maps a grab failure onto a status: a refusal because torrents
// are off is a conflict with the current settings, anything else keeps
// fallback.
func writeGrabError(w http.ResponseWriter, err error, fallback int) {
	var busy alreadyDownloadingError
	if errors.As(err, &busy) {
		body := map[string]any{"error": err.Error()}
		if b := busy.item; b.ID > 0 {
			// What is in the way, so the page can offer to cancel it.
			body["blocking"] = blockingDownload{ID: b.ID, Title: b.ReleaseTitle, Progress: b.ProgressPct, SizeBytes: b.SizeBytes, Paused: b.Status == queue.StatusPaused}
		}
		writeJSON(w, http.StatusConflict, body)
		return
	}
	if errors.Is(err, errTorrentsDisabled) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, fallback, err.Error())
}
