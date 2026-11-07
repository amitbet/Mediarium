package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
	"github.com/rdborg/mediarium/internal/music"
	"github.com/rdborg/mediarium/internal/queue"
)

// An album whose files are downloaded but whose track list MusicBrainz will
// not give (it answers "busy", or does not answer at all) is not a failed
// download. The files stay where they are, the row on the Activity page says
// MusicBrainz is busy, and the import (only the import, never the download)
// is tried again every few minutes for a day. Retry on that row, or on the
// failed row it turns into after the day, runs the import again at once from
// the same files.

const (
	// musicWaitNote is said on the Activity row of an album waiting for MusicBrainz.
	musicWaitNote = "MusicBrainz is busy. Mediarium will try again later."
	// musicWaitGaveUp is the message of the failed row once the waiting is over.
	musicWaitGaveUp = "MusicBrainz stayed busy, so this album wasn't imported. The downloaded files are kept. Press Retry to try the import again."

	musicWaitEvery   = 15 * time.Minute
	musicWaitGiveUp  = 24 * time.Hour
	musicWaitJobTick = time.Minute
)

// errMusicWaiting is what runMusicPipeline answers when it left the album
// waiting for MusicBrainz instead of finishing or failing.
var errMusicWaiting = errors.New("the album is waiting for MusicBrainz")

// musicWait is one album import waiting for MusicBrainz.
type musicWait struct {
	queueID, artistID, albumID int64
	restore                    music.Status
	releaseTitle, downloadURL  string
	protocol                   indexers.Protocol
	dir                        string // where the downloaded files are
	since, next                time.Time
	tries                      int
	running                    bool // an attempt is going on right now
}

// musicWaits is the set of imports waiting for MusicBrainz. It lives in
// memory: after a restart the item comes back paused, like any download that
// was in the middle of something (Resume starts it again from its files).
type musicWaits struct {
	mu            sync.Mutex
	items         map[int64]*musicWait
	every, giveUp time.Duration // 0: musicWaitEvery and musicWaitGiveUp
}

func (m *musicWaits) interval() time.Duration {
	if m.every > 0 {
		return m.every
	}
	return musicWaitEvery
}

func (m *musicWaits) limit() time.Duration {
	if m.giveUp > 0 {
		return m.giveUp
	}
	return musicWaitGiveUp
}

// get returns a copy of the wait for a queue item.
func (m *musicWaits) get(id int64) (musicWait, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.items[id]
	if !ok {
		return musicWait{}, false
	}
	return *w, true
}

func (m *musicWaits) drop(id int64) {
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
}

// claim marks a wait as being attempted. It reports false when there is no
// such wait or an attempt is already going on.
func (m *musicWaits) claim(id int64) (musicWait, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.items[id]
	if !ok || w.running {
		return musicWait{}, false
	}
	w.running = true
	return *w, true
}

// due lists the waits whose time has come and that are not being attempted.
func (m *musicWaits) due(now time.Time, force bool) []int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []int64
	for id, w := range m.items {
		if !w.running && (force || !now.Before(w.next)) {
			ids = append(ids, id)
		}
	}
	return ids
}

// hold registers (or renews) the wait after a try that MusicBrainz did not
// answer, and reports how many tries it has had. since keeps the time of the
// first one.
func (m *musicWaits) hold(w musicWait, now time.Time) (since time.Time, tries int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = map[int64]*musicWait{}
	}
	cur, ok := m.items[w.queueID]
	if !ok {
		w.since = now
		cur = &w
		m.items[w.queueID] = cur
	}
	cur.tries++
	cur.next = now.Add(m.interval())
	cur.running = false
	return cur.since, cur.tries
}

// holdForMusicBrainz leaves an album whose import could not get its track
// list from MusicBrainz waiting for the next try. It reports false when the
// waiting is over (a day has passed), so the caller fails the download. The
// download's place in the line is given back by the caller as for any other
// pipeline that ends.
func (s *Server) holdForMusicBrainz(w musicWait, label string, cause error) bool {
	now := time.Now()
	waits := &s.music.waits
	first, tries := waits.hold(w, now)
	if now.Sub(first) >= waits.limit() {
		waits.drop(w.queueID)
		return false
	}
	if err := s.QueueRepo.SetStatus(w.queueID, queue.StatusImporting, ""); err != nil {
		slog.Warn("music: mark an import as waiting for MusicBrainz", "queueId", w.queueID, "err", err)
	}
	slog.Info("music: MusicBrainz is not answering, the import waits", "queueId", w.queueID, "album", label, "tries", tries, "err", cause)
	if tries == 1 {
		_ = s.QueueRepo.LogItemEvent(queue.ItemEvent{AlbumID: w.albumID, Kind: "waiting", Level: queue.LevelWarn,
			Message: fmt.Sprintf("%s: MusicBrainz is busy. The downloaded files are kept and Mediarium will try the import again later.", label)})
	}
	return true
}

// musicWaitingNow says whether an item is an album import waiting for
// MusicBrainz, for the Activity page.
func (s *Server) musicWaitingNow(item queue.Item) bool {
	if item.AlbumID == 0 || item.Status != queue.StatusImporting {
		return false
	}
	_, ok := s.music.waits.get(item.ID)
	return ok
}

// retryMusicWaits is the scheduled job: it tries the imports whose time has come.
func (s *Server) retryMusicWaits(context.Context) { s.runDueMusicWaits(false) }

func (s *Server) runDueMusicWaits(force bool) {
	for _, id := range s.music.waits.due(time.Now(), force) {
		s.attemptMusicWait(id)
	}
}

// attemptMusicWait runs the import of one waiting album again in the
// background: the files are already there, only the import is repeated.
func (s *Server) attemptMusicWait(id int64) bool {
	w, ok := s.music.waits.claim(id)
	if !ok {
		return false
	}
	s.background(func() {
		it, err := s.QueueRepo.Get(id)
		if err != nil || it.Status != queue.StatusImporting {
			s.music.waits.drop(id) // removed, stopped or paused in the meantime
			return
		}
		var runErr error
		s.runGuarded(it, func() {
			runErr = s.runMusicPipelineFrom(w.queueID, w.artistID, w.albumID, w.restore, w.releaseTitle, w.downloadURL, w.protocol, w.dir)
		})
		if !errors.Is(runErr, errMusicWaiting) {
			s.music.waits.drop(id)
		}
	})
	return true
}

// isMusicMetadataStop says a failed download stopped only because MusicBrainz
// stayed busy, so its files are good and only the import is left to do.
func isMusicMetadataStop(item queue.Item) bool {
	return item.AlbumID > 0 && item.Status == queue.StatusFailed && strings.TrimSpace(item.Error) == musicWaitGaveUp
}

// retryMusicImport is Retry on an album download that is waiting for
// MusicBrainz or gave up waiting: it repeats the import at once from the
// files already downloaded. It reports false for any other download, which
// Retry handles as before (it also returns false when the files are gone).
func (s *Server) retryMusicImport(item queue.Item) bool {
	if item.AlbumID == 0 {
		return false
	}
	if s.musicWaitingNow(item) {
		if !s.attemptMusicWait(item.ID) {
			return true // an attempt is going on right now
		}
		s.queueItemEvent(item, "retried", queue.LevelInfo, fmt.Sprintf("Trying the import again now: %s", item.ReleaseTitle))
		return true
	}
	if !isMusicMetadataStop(item) {
		return false
	}
	dir, ok := s.existingWorkDir(item.ID)
	if !ok {
		return false
	}
	if files, err := music.FindAudioFiles(dir); err != nil || len(files) == 0 {
		return false
	}
	album, err := s.MusicRepo.GetAlbum(item.AlbumID)
	if err != nil {
		return false
	}
	if running, err := s.albumDownloadInProgress(album.ID); err != nil || running != "" {
		return false // another download of the album is on: the normal Retry says so
	}
	w := musicWait{queueID: item.ID, artistID: album.ArtistID, albumID: album.ID, restore: s.restoredAlbumStatus(album),
		releaseTitle: item.ReleaseTitle, downloadURL: item.NZBURL, protocol: indexers.Protocol(item.Protocol), dir: dir}
	_ = s.MusicRepo.SetAlbumStatus(album.ID, music.StatusDownloading)
	if err := s.QueueRepo.SetStatus(item.ID, queue.StatusImporting, ""); err != nil {
		_ = s.MusicRepo.SetAlbumStatus(album.ID, w.restore)
		return false
	}
	// A fresh waiting period starts; the first try runs now.
	s.music.waits.mu.Lock()
	if s.music.waits.items == nil {
		s.music.waits.items = map[int64]*musicWait{}
	}
	w.since, w.next = time.Now(), time.Now()
	s.music.waits.items[item.ID] = &w
	s.music.waits.mu.Unlock()
	s.queueItemEvent(item, "retried", queue.LevelInfo, fmt.Sprintf("Retrying the import from the files already downloaded: %s", item.ReleaseTitle))
	return s.attemptMusicWait(item.ID)
}
