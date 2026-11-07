package automation_test

import (
	"context"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/automation"
)

func TestStaggerFirstRuns(t *testing.T) {
	run := func(context.Context) {}
	s := automation.NewScheduler(
		automation.Job{Name: "a", Interval: time.Hour, Run: run},
		automation.Job{Name: "own", Interval: time.Hour, InitialDelay: time.Second, Run: run},
		automation.Job{Name: "b", Interval: 2 * time.Hour, Run: run},
		automation.Job{Name: "c", Interval: 3 * time.Hour, Run: run},
	)
	s.StaggerFirstRuns(time.Minute, 10*time.Second)
	want := map[string]time.Duration{
		"a":   time.Minute,
		"own": time.Second, // kept, and it takes no slot
		"b":   70 * time.Second,
		"c":   80 * time.Second,
	}
	for _, j := range s.Jobs() {
		if j.InitialDelay != want[j.Name] {
			t.Errorf("%s: first run after %v, want %v", j.Name, j.InitialDelay, want[j.Name])
		}
	}
	for _, j := range s.Jobs() {
		if j.Name == "b" && j.Interval != 2*time.Hour {
			t.Errorf("b lost its interval: %v", j.Interval)
		}
	}
}
