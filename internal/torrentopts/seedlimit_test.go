package torrentopts_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/torrentopts"
)

func TestParseLimit(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"", torrentopts.Unlimited},
		{"   ", torrentopts.Unlimited},
		{"0", 0},
		{"0.0", 0},
		{"2", 2},
		{"2.5", 2.5},
		{"1,5", 1.5},
		{" 48 ", 48},
		{"-1", torrentopts.Unlimited},
		{"abc", torrentopts.Unlimited},
		{"NaN", torrentopts.Unlimited},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := torrentopts.ParseLimit(tc.in); got != tc.want {
				t.Fatalf("ParseLimit(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSeedRuleValidateAndGoal(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	tests := []struct {
		name          string
		rule          torrentopts.SeedRule
		valid         bool
		global        [2]float64
		wantR, wantHr float64
	}{
		{"empty rule follows the global limits", torrentopts.SeedRule{}, true, [2]float64{2, 48}, 2, 48},
		{"empty rule under unlimited global stays unlimited", torrentopts.SeedRule{}, true, [2]float64{-1, -1}, -1, -1},
		{"a rule replaces its own field only", torrentopts.SeedRule{Ratio: f(1)}, true, [2]float64{2, 48}, 1, 48},
		{"0 is do not seed, also over an unlimited global", torrentopts.SeedRule{Ratio: f(0)}, true, [2]float64{-1, -1}, 0, -1},
		{"-1 is unlimited over a global limit", torrentopts.SeedRule{Hours: f(-1)}, true, [2]float64{2, 48}, 2, -1},
		{"-2 is no limit at all", torrentopts.SeedRule{Ratio: f(-2)}, false, [2]float64{}, 0, 0},
		{"a ratio over the largest", torrentopts.SeedRule{Ratio: f(101)}, false, [2]float64{}, 0, 0},
		{"too many hours", torrentopts.SeedRule{Hours: f(1e6)}, false, [2]float64{}, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate = %v, want valid %v", err, tc.valid)
			}
			if !tc.valid {
				return
			}
			r, h := tc.rule.Goal(tc.global[0], tc.global[1])
			if r != tc.wantR || h != tc.wantHr {
				t.Fatalf("Goal = %v, %v; want %v, %v", r, h, tc.wantR, tc.wantHr)
			}
		})
	}
}
