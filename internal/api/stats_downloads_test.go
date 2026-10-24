package api_test

import (
	"net/http"
	"testing"

	"github.com/rdborg/mediarium/internal/queue"
)

type downloadStatsAnswer struct {
	UsenetShare int `json:"usenetShare"`
	Totals      struct {
		Grabs     int   `json:"grabs"`
		Completed int   `json:"completed"`
		Failed    int   `json:"failed"`
		Bytes     int64 `json:"bytes"`
	} `json:"totals"`
	Months []struct {
		Completed int   `json:"completed"`
		Failed    int   `json:"failed"`
		Bytes     int64 `json:"bytes"`
	} `json:"months"`
}

// Clearing the finished downloads from Activity must not clear the numbers on
// the dashboard and the Statistics page; "Reset statistics" does, for
// administrators only.
func TestStatisticsSurviveClearingTheHistory(t *testing.T) {
	server, base, admin, member, _ := familyServer(t)
	enqueue := func(title string, p queue.Protocol, size int64, end queue.Status) {
		t.Helper()
		id, err := server.QueueRepo.Enqueue(queue.Item{ReleaseTitle: title, NZBURL: "http://127.0.0.1:1/x", SizeBytes: size, Protocol: p})
		if err != nil {
			t.Fatal(err)
		}
		if end != "" {
			if err := server.QueueRepo.SetStatus(id, end, ""); err != nil {
				t.Fatal(err)
			}
		}
	}
	enqueue("a", queue.ProtocolUsenet, 3000, queue.StatusCompleted)
	enqueue("b", queue.ProtocolUsenet, 1000, queue.StatusCompleted)
	enqueue("c", queue.ProtocolTorrent, 500, queue.StatusCompleted)
	enqueue("d", queue.ProtocolUsenet, 0, queue.StatusFailed)
	enqueue("e", queue.ProtocolUsenet, 0, "")

	// Clear finished, as the button on Activity > History does.
	if got := deleteReq(t, admin, base+"/api/queue"); got != http.StatusOK {
		t.Fatalf("clear finished: %d", got)
	}
	left := getJSON[[]map[string]any](t, admin, base+"/api/queue")
	if len(left) != 1 {
		t.Fatalf("the list after clearing: %v", left)
	}

	stats := getJSON[downloadStatsAnswer](t, admin, base+"/api/stats/library")
	if stats.Totals.Grabs != 5 || stats.Totals.Completed != 3 || stats.Totals.Failed != 1 || stats.Totals.Bytes != 4500 {
		t.Fatalf("totals after clearing: %+v", stats.Totals)
	}
	if stats.UsenetShare != 66 {
		t.Errorf("usenet share after clearing: %d", stats.UsenetShare)
	}
	if len(stats.Months) != 1 || stats.Months[0].Completed != 3 || stats.Months[0].Failed != 1 {
		t.Errorf("months after clearing: %+v", stats.Months)
	}

	// Only an administrator may reset.
	if code, _ := doStatus(t, member, http.MethodPost, base+"/api/stats/reset"); code != http.StatusForbidden {
		t.Fatalf("a member resetting: %d", code)
	}
	if got := getJSON[downloadStatsAnswer](t, admin, base+"/api/stats/library"); got.Totals.Completed != 3 {
		t.Fatalf("a refused reset changed the totals: %+v", got.Totals)
	}
	if code, _ := doStatus(t, admin, http.MethodPost, base+"/api/stats/reset"); code != http.StatusOK {
		t.Fatalf("reset: %d", code)
	}
	after := getJSON[downloadStatsAnswer](t, admin, base+"/api/stats/library")
	if after.Totals.Grabs != 0 || after.Totals.Completed != 0 || after.Totals.Bytes != 0 || after.UsenetShare != 0 || len(after.Months) != 0 {
		t.Fatalf("after the reset: %+v", after)
	}

	// Counting starts again from there.
	enqueue("f", queue.ProtocolTorrent, 700, queue.StatusCompleted)
	server.TestForgetLibraryStats()
	again := getJSON[downloadStatsAnswer](t, admin, base+"/api/stats/library")
	if again.Totals.Completed != 1 || again.Totals.Bytes != 700 || again.UsenetShare != 0 {
		t.Fatalf("after counting again: %+v", again)
	}
}
