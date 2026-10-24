package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/crypto"
	"github.com/rdborg/mediarium/internal/notify"
	"github.com/rdborg/mediarium/internal/queue"
	"github.com/rdborg/mediarium/internal/settings"
)

// A check of the TRaSH Guides that changed something sends one message to the
// targets that ticked the event, whether the job or the button made it; a
// check that changed nothing sends none unless "also tell me when nothing
// changed" is on; one that failed always sends; a target that did not choose
// the event hears nothing.
func TestTrashSyncNotification(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenTestDB(filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	box, err := crypto.LoadOrCreateKey(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan map[string]any, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		got <- m
	}))
	t.Cleanup(srv.Close)
	silent := make(chan struct{}, 10)
	quiet := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { silent <- struct{}{} }))
	t.Cleanup(quiet.Close)

	s := &Server{Settings: settings.New(db, nil), NotifyRepo: notify.NewRepo(db, box), QueueRepo: queue.NewRepo(db)}
	if _, err := s.NotifyRepo.Create(notify.Target{Name: "yes", Type: notify.TargetWebhook, Events: []string{notify.EventTrashSync}, Config: map[string]string{"url": srv.URL}}); err != nil {
		t.Fatal(err)
	}
	// Made with no events chosen: the default set, which leaves this one out.
	if _, err := s.NotifyRepo.Create(notify.Target{Name: "default", Type: notify.TargetWebhook, Config: map[string]string{"url": quiet.URL}}); err != nil {
		t.Fatal(err)
	}

	changes := func(n int) trashUpdateResult {
		res := trashUpdateResult{Checked: 40}
		for i := 1; i <= n; i++ {
			res.Changes = append(res.Changes, trashFormatChange{ID: int64(i), Name: fmt.Sprintf("Format %d", i), scores: map[int64]int{1: 100, 2: 100}})
		}
		return res
	}
	tests := []struct {
		name      string
		res       trashUpdateResult
		err       error
		daily     bool
		always    bool
		wantSent  bool
		wantTitle string
		wantInMsg []string
	}{
		{"nothing changed", trashUpdateResult{Checked: 40}, nil, true, false, false, "", nil},
		{"nothing changed, told anyway", trashUpdateResult{Checked: 40}, nil, true, true, true, "TRaSH Guides checked", []string{"TRaSH Guides checked: nothing needed updating", "40 custom formats", "The automatic update"}},
		{"the check failed", trashUpdateResult{}, errors.New("no network"), true, false, true, "TRaSH Guides check failed", []string{"TRaSH Guides check failed: no network", "The automatic update"}},
		{"the check failed, told anyway", trashUpdateResult{}, errors.New("No network."), false, true, true, "TRaSH Guides check failed", []string{"TRaSH Guides check failed: no network.", "Update from TRaSH"}},
		{"the job updated formats", changes(2), nil, true, false, true, "TRaSH Guides sync applied", []string{"2 custom formats updated", "Format 1, Format 2", "4 scores changed in 2 profiles", "The automatic update"}},
		{"the button updated more than five", changes(8), nil, false, true, true, "TRaSH Guides sync applied", []string{"8 custom formats updated", "Format 5 and 3 more", "Update from TRaSH"}},
		{"one format", changes(1), nil, false, false, true, "TRaSH Guides sync applied", []string{"1 custom format updated"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			always := "0"
			if tc.always {
				always = "1"
			}
			if err := s.Settings.Set(settings.KeyTrashSyncAlways, always, false); err != nil {
				t.Fatal(err)
			}
			s.recordTrashCheck(tc.res, tc.err, tc.daily)
			s.bg.Wait()
			if !tc.wantSent {
				if len(got) != 0 {
					t.Fatalf("a message was sent: %v", <-got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("got %d messages, want 1", len(got))
			}
			m := <-got
			if m["event"] != "trashsync" || m["title"] != tc.wantTitle {
				t.Errorf("event %v title %v", m["event"], m["title"])
			}
			text, _ := m["message"].(string)
			for _, want := range tc.wantInMsg {
				if !strings.Contains(text, want) {
					t.Errorf("message lacks %q:\n%s", want, text)
				}
			}
			if names, _ := m["customFormats"].([]any); tc.wantTitle == "TRaSH Guides sync applied" && len(names) == 0 {
				t.Errorf("the webhook has no customFormats: %v", m)
			}
		})
	}
	if len(silent) != 0 {
		t.Error("a target that did not choose the event got a message")
	}
}
