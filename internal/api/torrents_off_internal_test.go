package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/indexers"
)

func TestErrorCodeForTorrentsOff(t *testing.T) {
	tests := []struct {
		msg, want string
	}{
		{errTorrentsDisabled.Error(), codeTorrentsOff},
		{fmt.Errorf("Some Show S01E01: %w", errTorrentsDisabled).Error(), codeTorrentsOff},
		{"Already downloading.", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := errorCodeFor(tt.msg); got != tt.want {
			t.Errorf("errorCodeFor(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestWriteGrabErrorSendsTheCodeAndKeepsTheText(t *testing.T) {
	rec := httptest.NewRecorder()
	writeGrabError(rec, errTorrentsDisabled, http.StatusInternalServerError)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":"torrents_off"`) || !strings.Contains(body, "Torrents are switched off.") {
		t.Fatalf("body = %s", body)
	}
	rec = httptest.NewRecorder()
	writeGrabError(rec, fmt.Errorf("disk full"), http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), `"code"`) {
		t.Fatalf("an ordinary error has no code: %s", rec.Body.String())
	}
}

func TestIndexerSetupHealth(t *testing.T) {
	now := time.Now()
	tr := indexers.NewTracker(nil, nil)
	failing := indexers.Instance{ID: 1, Name: "Failing", Enabled: true, Protocol: indexers.ProtocolUsenet, Tracker: tr}
	tr.Failed(1, fmt.Errorf("it broke"))
	works := indexers.Instance{ID: 2, Name: "Works", Enabled: true, Protocol: indexers.ProtocolUsenet, Tracker: tr}
	testFailed := indexers.Instance{ID: 3, Name: "TestFailed", Enabled: true, Protocol: indexers.ProtocolUsenet, LastTestError: "no", Tracker: tr}
	torrent := indexers.Instance{ID: 4, Name: "Tor", Enabled: true, Protocol: indexers.ProtocolTorrent, Tracker: tr}
	off := indexers.Instance{ID: 5, Name: "Off", Enabled: false, Protocol: indexers.ProtocolUsenet, Tracker: tr}
	unreadable := indexers.Instance{ID: 6, Name: "Locked", Enabled: true, Protocol: indexers.ProtocolUsenet, SecretError: "x", Tracker: tr}

	tests := []struct {
		name       string
		list       []indexers.Instance
		torrentsOn bool
		want       string // the Health id, "" for none
	}{
		{"nothing added", nil, true, "no-indexers"},
		{"all turned off", []indexers.Instance{off}, true, "indexers-off"},
		{"only an unreadable one", []indexers.Instance{unreadable}, true, "indexers-off"},
		{"only torrent indexers and torrents off", []indexers.Instance{torrent}, false, "indexers-torrents-off"},
		{"torrent indexers with torrents on, none failing", []indexers.Instance{torrent}, true, ""},
		{"every searchable one is failing", []indexers.Instance{failing, testFailed}, true, "indexers-failing"},
		{"one works, one fails", []indexers.Instance{failing, works}, true, ""},
		{"a failing usenet one with a torrent one that is not searched", []indexers.Instance{failing, torrent}, false, "indexers-failing"},
		{"a failing indexer is never called not set up", []indexers.Instance{failing}, true, "indexers-failing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var added []string
			got := indexerSetupHealth(tt.list, tt.torrentsOn, now, func(id, level, title, impact, label, path string) {
				if level != "error" || title == "" || impact == "" || label == "" || path == "" {
					t.Errorf("incomplete line: %q %q %q %q %q %q", id, level, title, impact, label, path)
				}
				added = append(added, id)
			})
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if (tt.want == "") != (len(added) == 0) || len(added) > 1 {
				t.Fatalf("added %v for %q", added, tt.want)
			}
		})
	}
}
