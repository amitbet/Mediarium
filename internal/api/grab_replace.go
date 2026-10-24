package api

import (
	"errors"
	"fmt"
	"sync"

	"github.com/rdborg/mediarium/internal/queue"
)

// Cancelling the download that is in the way and grabbing another release in
// one step: the "Cancel it and grab this one" button on the release list.

// blockingDownload is what a refused grab says about the download in the way,
// so the page can name it and offer to cancel it.
type blockingDownload struct {
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Progress  float64 `json:"progress"`
	SizeBytes int64   `json:"sizeBytes"`
	Paused    bool    `json:"paused"`
}

// replaceMu keeps two replacing grabs from running at once, so the second one
// sees the first one's release as the download in the way instead of both
// cancelling and grabbing at the same time.
var replaceMu sync.Mutex

// replaceBlocking cancels the download that stops a grab, but only when it is
// the one the person named (replaceID): check is the same test the grab
// itself makes (it answers alreadyDownloadingError for a download in the way).
// A different download in the way is refused as usual, naming what is really
// there; when nothing is in the way any more, there is nothing to cancel and
// the grab goes on. The download is stopped, its partly downloaded files are
// deleted and it is taken off the list, as when it is stopped from Activity
// with the files ticked. Call it with replaceMu held, right before the grab.
func (s *Server) replaceBlocking(replaceID int64, check func() error) error {
	err := check()
	var busy alreadyDownloadingError
	if !errors.As(err, &busy) {
		return err
	}
	if busy.item.ID == 0 || busy.item.ID != replaceID {
		return err
	}
	queueControlMu.Lock()
	defer queueControlMu.Unlock()
	s.queueItemEvent(busy.item, "removed", queue.LevelInfo, fmt.Sprintf("Cancelled so another release could be grabbed: %s", busy.item.ReleaseTitle))
	s.discardItem(busy.item)
	return nil
}
