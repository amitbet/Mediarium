package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
)

func (r *removeEnv) clientActions() []string {
	r.qb.mu.Lock()
	defer r.qb.mu.Unlock()
	return append([]string(nil), r.qb.actions...)
}

func (r *removeEnv) wantAction(t *testing.T, want string) {
	t.Helper()
	r.waitFor(t, "the client to be told "+want, func() bool {
		for _, a := range r.clientActions() {
			if a == want {
				return true
			}
		}
		return false
	})
}

// Pausing a download that runs in an external client pauses the torrent there
// too (qBittorrent 5 calls it stop), and resuming the download starts it again.
func TestPauseAndResumeReachTheClient(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/pause", r.id), map[string]any{}, http.StatusOK)
	r.wantAction(t, "stop "+fixtureHash)

	// The client now lists the torrent as stopped; resuming starts it.
	r.qb.mu.Lock()
	r.qb.state = "stoppedDL"
	r.qb.mu.Unlock()
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/resume", r.id), map[string]any{}, http.StatusOK)
	r.wantAction(t, "start "+fixtureHash)
	r.qb.mu.Lock()
	deleted := len(r.qb.deleted)
	r.qb.mu.Unlock()
	if deleted != 0 {
		t.Fatalf("nothing should be deleted from the client when pausing and resuming")
	}
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", r.id), map[string]any{}, http.StatusOK)
}

// A torrent paused by hand in the client is not mistaken for a stall, and
// Mediarium does not start it again behind the person's back.
func TestTorrentPausedInTheClientIsLeftAlone(t *testing.T) {
	defer api.SetDownloadClientPollForTest(10 * time.Millisecond)()
	r := startHeld(t, map[string]any{})
	r.qb.mu.Lock()
	r.qb.state = "pausedDL"
	r.qb.mu.Unlock()
	r.waitFor(t, "a few more status checks", func() bool {
		r.qb.mu.Lock()
		defer r.qb.mu.Unlock()
		return r.qb.infos >= 12
	})
	for _, a := range r.clientActions() {
		if strings.HasPrefix(a, "start") || strings.HasPrefix(a, "resume") {
			t.Fatalf("Mediarium started a torrent the person paused: %v", r.clientActions())
		}
	}
	q := getJSON[[]map[string]any](t, r.client, r.url+"/api/queue")
	if len(q) != 1 || q[0]["status"] != "downloading" {
		t.Fatalf("the download should still be waiting on the client: %v", q)
	}
	r.call(t, http.MethodPost, fmt.Sprintf("/api/queue/%d/stop", r.id), map[string]any{}, http.StatusOK)
}
