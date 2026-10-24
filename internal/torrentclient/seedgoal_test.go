package torrentclient_test

import (
	"testing"
	"time"

	mediarium "github.com/rdborg/mediarium/internal/torrentclient"
)

func TestGoalOf(t *testing.T) {
	tests := []struct {
		name         string
		ratio, hours float64
		want         mediarium.SeedGoal
		unlimited    bool
	}{
		{"both unlimited", -1, -1, mediarium.SeedGoal{}, true},
		{"ratio only", 2, -1, mediarium.SeedGoal{Ratio: 2}, false},
		{"hours only", -1, 1.5, mediarium.SeedGoal{Time: 90 * time.Minute}, false},
		{"both", 2, 48, mediarium.SeedGoal{Ratio: 2, Time: 48 * time.Hour}, false},
		{"ratio 0 is do not seed", 0, -1, mediarium.SeedGoal{NoSeed: true}, false},
		{"hours 0 is do not seed", -1, 0, mediarium.SeedGoal{NoSeed: true}, false},
		{"0 beats a real limit", 0, 48, mediarium.SeedGoal{NoSeed: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mediarium.GoalOf(tc.ratio, tc.hours)
			if got != tc.want {
				t.Fatalf("GoalOf(%v, %v) = %+v, want %+v", tc.ratio, tc.hours, got, tc.want)
			}
			if got.Unlimited() != tc.unlimited {
				t.Fatalf("Unlimited = %v, want %v", got.Unlimited(), tc.unlimited)
			}
			if got.NoSeed && !got.Met(0, 1<<30, 0) {
				t.Fatal("a goal of not seeding is met from the start")
			}
		})
	}
}
