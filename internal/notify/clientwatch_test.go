package notify

import (
	"strings"
	"testing"
	"time"
)

func TestClientWatch(t *testing.T) {
	type step struct {
		after time.Duration // time since the previous step
		ok    bool
		want  ClientChange
	}
	const s = time.Second
	tests := []struct {
		name  string
		steps []step
	}{
		{"healthy client says nothing", []step{{s, true, ClientSame}, {s, true, ClientSame}}},
		{"two failures in a row are not enough", []step{{0, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, true, ClientSame}}},
		{"three quick failures wait for the minimum span", []step{{0, false, ClientSame}, {5 * s, false, ClientSame}, {5 * s, false, ClientSame}, {60 * s, false, ClientWentDown}}},
		{"down once, back once", []step{
			{0, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, false, ClientWentDown},
			{30 * s, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, true, ClientCameBack}, {30 * s, true, ClientSame},
		}},
		{"a success in between starts the count over", []step{
			{0, false, ClientSame}, {40 * s, false, ClientSame}, {10 * s, true, ClientSame}, {10 * s, false, ClientSame}, {40 * s, false, ClientSame},
		}},
		{"a second outage within ten minutes waits", []step{
			{0, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, false, ClientWentDown}, {10 * s, true, ClientCameBack},
			{10 * s, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, false, ClientSame}, // down again, too soon after the last message
			{time.Minute, true, ClientSame}, // it recovered unannounced: no "back" either
		}},
		{"a held outage is told once ten minutes have passed", []step{
			{0, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, false, ClientWentDown}, {10 * s, true, ClientCameBack},
			{10 * s, false, ClientSame}, {30 * s, false, ClientSame}, {30 * s, false, ClientSame},
			{10 * time.Minute, false, ClientWentDown}, {time.Minute, false, ClientSame},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			w := &ClientWatch{Now: func() time.Time { return now }}
			for i, st := range tc.steps {
				now = now.Add(st.after)
				if got := w.Note("c", st.ok); got.Change != st.want {
					t.Fatalf("step %d: change = %v, want %v", i, got.Change, st.want)
				}
			}
		})
	}
}

func TestClientWatchKeepsClientsApart(t *testing.T) {
	now := time.Now()
	w := &ClientWatch{Now: func() time.Time { return now }}
	for i := 0; i < 3; i++ {
		w.Note("a", false)
		now = now.Add(40 * time.Second)
	}
	if got := w.Note("b", false); got.Change != ClientSame {
		t.Errorf("one client's failures counted against another: %v", got.Change)
	}
}

func TestClientWatchReportsOutageLength(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	w := &ClientWatch{Now: func() time.Time { return now }}
	w.Note("c", false)
	now = now.Add(time.Minute)
	w.Note("c", false)
	now = now.Add(time.Minute)
	w.Note("c", false)
	now = now.Add(10 * time.Minute)
	got := w.Note("c", true)
	if got.Change != ClientCameBack || got.Outage != 12*time.Minute {
		t.Errorf("got %+v, want back after 12 minutes", got)
	}
}

func TestClientMessages(t *testing.T) {
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		ev        Event
		wantTitle string
		wantLead  string
	}{
		{"down with a reason", ComposeClientDown("qBittorrent", "Connection refused.", Links{}, at), "qBittorrent can't be reached", "qBittorrent can't be reached: connection refused."},
		{"down without a reason", ComposeClientDown("SABnzbd", "", Links{}, at), "SABnzbd can't be reached", "SABnzbd can't be reached."},
		{"back after minutes", ComposeClientUp("qBittorrent", 12*time.Minute, Links{}, at), "qBittorrent is reachable again", "qBittorrent is reachable again after 12 minutes."},
		{"back after a short while", ComposeClientUp("NZBGet", 20*time.Second, Links{}, at), "NZBGet is reachable again", "NZBGet is reachable again after less than a minute."},
		{"back after hours", ComposeClientUp("NZBGet", 65*time.Minute, Links{}, at), "NZBGet is reachable again", "NZBGet is reachable again after 1 hour 5 minutes."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ev.Title != tc.wantTitle || tc.ev.Lead != tc.wantLead {
				t.Errorf("got title %q lead %q, want %q / %q", tc.ev.Title, tc.ev.Lead, tc.wantTitle, tc.wantLead)
			}
			if !strings.Contains(tc.ev.Message, tc.wantLead) {
				t.Errorf("plain message %q does not hold the sentence", tc.ev.Message)
			}
		})
	}
}

func TestOutageText(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "less than a minute"}, {29 * time.Second, "less than a minute"}, {31 * time.Second, "1 minute"}, {2 * time.Minute, "2 minutes"},
		{59 * time.Minute, "59 minutes"}, {60 * time.Minute, "1 hour"}, {121 * time.Minute, "2 hours 1 minute"}, {150 * time.Minute, "2 hours 30 minutes"},
	}
	for _, tc := range tests {
		if got := outageText(tc.d); got != tc.want {
			t.Errorf("outageText(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestClientEventsAreOptIn(t *testing.T) {
	for _, kind := range []string{EventClientDown, EventClientUp} {
		if (Target{}).Subscribed(kind) {
			t.Errorf("a target with no list of events (an older one) hears %s", kind)
		}
		if !(Target{Events: []string{kind}}).Subscribed(kind) {
			t.Errorf("a target that chose %s does not hear it", kind)
		}
		if (Target{Events: []string{EventFailed}}).Subscribed(kind) {
			t.Errorf("a target that did not choose %s hears it", kind)
		}
		for _, d := range DefaultEvents() {
			if d == kind {
				t.Errorf("%s is on by default", kind)
			}
		}
	}
	if !(Target{}).Subscribed(EventImported) {
		t.Error("an older target no longer hears about downloads")
	}
	got, err := NormalizeEvents([]string{"clientup", "failed", "clientdown"})
	if err != nil || strings.Join(got, ",") != "failed,clientdown,clientup" {
		t.Errorf("NormalizeEvents = %v, %v", got, err)
	}
}
