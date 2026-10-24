package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
)

func TestBuiltinSizeOK(t *testing.T) {
	const mb = int64(1) << 20
	cases := []struct {
		name    string
		on      bool
		size    int64
		eps     int
		minutes float64
		ok      bool
	}{
		{"on: 20 MB episode of a normal show is refused", true, 20 * mb, 1, 45, false},
		{"on: 60 MB episode of a 5 minute show passes", true, 60 * mb, 1, 5, true},
		{"on: 60 MB with an unknown runtime is refused", true, 60 * mb, 1, 0, false},
		{"off: a 20 MB release is accepted", false, 20 * mb, 1, 45, true},
		{"off: a 40 MB movie is accepted", false, 40 * mb, 0, 0, true},
		{"on: a 40 MB movie is refused", true, 40 * mb, 0, 0, false},
	}
	for _, tc := range cases {
		setMinSizeCheck(tc.on)
		if got := builtinSizeOK(quality.TierWebDL1080p, tc.size, tc.eps, tc.minutes); got != tc.ok {
			t.Errorf("%s: = %v, want %v", tc.name, got, tc.ok)
		}
	}
	setMinSizeCheck(true)
}
