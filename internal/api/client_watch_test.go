package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/crypto"
	"github.com/rdborg/mediarium/internal/download"
	"github.com/rdborg/mediarium/internal/notify"
	"github.com/rdborg/mediarium/internal/qbit"
	"github.com/rdborg/mediarium/internal/quality"
	"github.com/rdborg/mediarium/internal/settings"
	"github.com/rdborg/mediarium/internal/usenetclient"
)

func TestClientUnreachable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"no error", nil, false},
		{"connection refused", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"timeout", context.DeadlineExceeded, true},
		{"wrong password", fmt.Errorf("login: %w", qbit.ErrAuth), false},
		{"wrong api key", usenetclient.ErrAuth, false},
		{"stopped by the caller", context.Canceled, false},
		{"other client answered", usenetclient.ErrNotThisClient, false},
	}
	for _, tc := range tests {
		if got := clientUnreachable(tc.err); got != tc.want {
			t.Errorf("%s: clientUnreachable = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuiltinUsenetUnreachable(t *testing.T) {
	dial := fmt.Errorf("dial nntp news:119: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")})
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"server not reachable", dial, true},
		{"release is bad", &download.ReleaseError{Err: errors.New("missing articles")}, false},
		{"disk full", errors.New("no space left on device"), false},
		{"fine", nil, false},
	}
	for _, tc := range tests {
		if got := builtinUsenetUnreachable(tc.err); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A client that stays down is told once, and again when it is back; a target
// that did not choose the events hears nothing.
func TestNoteClientCheckSendsDownAndBackOnce(t *testing.T) {
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

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	s := &Server{Settings: settings.New(db, nil), NotifyRepo: notify.NewRepo(db, box)}
	s.clientWatch.Now = func() time.Time { return now }
	if _, err := s.NotifyRepo.Create(notify.Target{Name: "yes", Type: notify.TargetWebhook, Events: []string{notify.EventClientDown, notify.EventClientUp}, Config: map[string]string{"url": srv.URL}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NotifyRepo.Create(notify.Target{Name: "no", Type: notify.TargetWebhook, Events: []string{notify.EventFailed}, Config: map[string]string{"url": quiet.URL}}); err != nil {
		t.Fatal(err)
	}
	refused := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	for i := 0; i < 6; i++ {
		s.noteClientCheck(watchTorrentClient, "qBittorrent", refused)
		now = now.Add(30 * time.Second)
	}
	s.noteClientCheck(watchTorrentClient, "qBittorrent", fmt.Errorf("login: %w", qbit.ErrAuth)) // answered: changes nothing
	now = now.Add(9 * time.Minute)
	s.noteClientCheck(watchTorrentClient, "qBittorrent", nil)
	s.noteClientCheck(watchTorrentClient, "qBittorrent", nil)
	s.bg.Wait()

	var msgs []map[string]any
	for len(got) > 0 {
		msgs = append(msgs, <-got)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want one down and one back: %v", len(msgs), msgs)
	}
	if msgs[0]["event"] != "clientdown" && msgs[1]["event"] != "clientdown" {
		t.Errorf("no down message: %v", msgs)
	}
	var down, up string
	for _, m := range msgs {
		switch m["event"] {
		case "clientdown":
			down, _ = m["message"].(string)
		case "clientup":
			up, _ = m["message"].(string)
		}
	}
	if want := "qBittorrent can't be reached: "; len(down) < len(want) || down[:len(want)] != want {
		t.Errorf("down message %q", down)
	}
	if want := "qBittorrent is reachable again after 12 minutes."; up != want {
		t.Errorf("up message %q, want %q", up, want)
	}
	if len(silent) != 0 {
		t.Error("a target that did not choose these events got a message")
	}
}

func TestFormatNamesByWeight(t *testing.T) {
	got := formatNamesByWeight([]quality.FormatHit{{Name: "small", Score: 5}, {Name: "bad", Score: -1000}, {Name: "good", Score: 800}})
	if want := []string{"bad", "good", "small"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
