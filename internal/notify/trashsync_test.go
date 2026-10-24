package notify

import (
	"strings"
	"testing"
	"time"
)

func TestComposeTrashSync(t *testing.T) {
	links := Links{}
	tests := []struct {
		name                      string
		formats                   []string
		updated, scores, profiles int
		automatic                 bool
		wantOK                    bool
		wantLead                  string
		wantRows                  []Detail
		wantNotInMessage          string
	}{
		{name: "nothing changed", updated: 0},
		{name: "one format, no scores", formats: []string{"x265 (HD)"}, updated: 1, automatic: true, wantOK: true,
			wantLead: "1 custom format updated from the TRaSH Guides.",
			wantRows: []Detail{{"Custom formats", "x265 (HD)"}, {"Started by", "The automatic update"}}, wantNotInMessage: "Profile scores"},
		{name: "scores in profiles", formats: []string{"A", "B"}, updated: 2, scores: 3, profiles: 1, wantOK: true,
			wantLead: "2 custom formats updated from the TRaSH Guides.",
			wantRows: []Detail{{"Custom formats", "A, B"}, {"Profile scores", "3 scores changed in 1 profile"}, {"Started by", "Update from TRaSH"}}},
		{name: "exactly five names", formats: []string{"1", "2", "3", "4", "5"}, updated: 5, wantOK: true,
			wantRows: []Detail{{"Custom formats", "1, 2, 3, 4, 5"}, {"Started by", "Update from TRaSH"}}},
		{name: "more than five", formats: []string{"1", "2", "3", "4", "5", "6", "7"}, updated: 12, wantOK: true,
			wantRows: []Detail{{"Custom formats", "1, 2, 3, 4, 5 and 7 more"}, {"Started by", "Update from TRaSH"}}},
		{name: "names that are blank are skipped", formats: []string{" ", "A"}, updated: 1, wantOK: true,
			wantRows: []Detail{{"Custom formats", "A"}, {"Started by", "Update from TRaSH"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ComposeTrashSync(tc.formats, tc.updated, tc.scores, tc.profiles, tc.automatic, links, time.Now())
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if ev.Type != EventTrashSync || ev.Title != "TRaSH Guides sync applied" {
				t.Errorf("event %q, title %q", ev.Type, ev.Title)
			}
			if tc.wantLead != "" && ev.Lead != tc.wantLead {
				t.Errorf("lead %q, want %q", ev.Lead, tc.wantLead)
			}
			if len(ev.Details) != len(tc.wantRows) {
				t.Fatalf("rows %v, want %v", ev.Details, tc.wantRows)
			}
			for i, row := range tc.wantRows {
				if ev.Details[i] != row {
					t.Errorf("row %d = %v, want %v", i, ev.Details[i], row)
				}
			}
			if tc.wantNotInMessage != "" && strings.Contains(ev.Message, tc.wantNotInMessage) {
				t.Errorf("message has %q: %s", tc.wantNotInMessage, ev.Message)
			}
		})
	}
}

func TestComposeTrashSyncNothingAndFailed(t *testing.T) {
	links := Links{}
	tests := []struct {
		name string
		ev   Event
		want []string
	}{
		{"nothing changed by the job", ComposeTrashSyncNothing(40, true, links, time.Now()),
			[]string{"TRaSH Guides checked: nothing needed updating.", "Already up to date: 40 custom formats", "Started by: The automatic update"}},
		{"nothing changed by the button, none counted", ComposeTrashSyncNothing(0, false, links, time.Now()),
			[]string{"TRaSH Guides checked: nothing needed updating.", "Started by: Update from TRaSH"}},
		{"failed", ComposeTrashSyncFailed("Couldn't reach the TRaSH Guides.", false, links, time.Now()),
			[]string{"TRaSH Guides check failed: couldn't reach the TRaSH Guides.", "Started by: Update from TRaSH"}},
		{"failed without a reason", ComposeTrashSyncFailed("", true, links, time.Now()),
			[]string{"TRaSH Guides check failed."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ev.Type != EventTrashSync {
				t.Errorf("event %q", tc.ev.Type)
			}
			for _, w := range tc.want {
				if !strings.Contains(tc.ev.Message, w) {
					t.Errorf("message lacks %q:\n%s", w, tc.ev.Message)
				}
			}
		})
	}
}

func TestTrashSyncIsOptIn(t *testing.T) {
	if !OptIn(EventTrashSync) {
		t.Fatal("the TRaSH sync event must be off unless chosen")
	}
	for _, e := range DefaultEvents() {
		if e == EventTrashSync {
			t.Fatal("it is in the default events")
		}
	}
	if !(Target{}).Subscribed(EventDigest) || (Target{}).Subscribed(EventTrashSync) {
		t.Error("a target with no events chosen must get the usual ones but not this")
	}
	if !(Target{Events: []string{EventTrashSync}}).Subscribed(EventTrashSync) {
		t.Error("a target that chose it must get it")
	}
}
