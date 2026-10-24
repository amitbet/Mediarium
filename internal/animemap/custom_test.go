package animemap

import (
	"reflect"
	"testing"
)

func TestCustomToReleaseAndBack(t *testing.T) {
	whole := &Custom{Rules: []CustomRule{{LibrarySeason: 1, ReleaseSeason: 15}}}
	split := &Custom{Rules: []CustomRule{
		{LibrarySeason: 1, ReleaseSeason: 1, From: 1, To: 12},
		{LibrarySeason: 2, ReleaseSeason: 1, Offset: 12},
	}}
	ranged := &Custom{Rules: []CustomRule{{LibrarySeason: 1, ReleaseSeason: 15, From: 1, To: 8}}}
	tests := []struct {
		name           string
		c              *Custom
		libS, libE     int
		wantRS, wantRE int
		wantOK         bool
	}{
		{"no rules at all", nil, 1, 1, 0, 0, false},
		{"empty rules", &Custom{}, 1, 1, 0, 0, false},
		{"whole season renumbered", whole, 1, 3, 15, 3, true},
		{"a season no rule covers", whole, 2, 3, 0, 0, false},
		{"split season, first half", split, 1, 12, 1, 12, true},
		{"split season, continuing numbers", split, 2, 1, 1, 13, true},
		{"inside the range", ranged, 1, 8, 15, 8, true},
		{"past the range", ranged, 1, 9, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs, re, ok := tt.c.ToRelease(tt.libS, tt.libE)
			if ok != tt.wantOK || rs != tt.wantRS || re != tt.wantRE {
				t.Fatalf("ToRelease(%d,%d) = %d,%d,%v", tt.libS, tt.libE, rs, re, ok)
			}
			if !ok {
				return
			}
			// And back again.
			ls, le, ok := tt.c.ToLibrary(rs, re)
			if !ok || ls != tt.libS || le != tt.libE {
				t.Fatalf("ToLibrary(%d,%d) = %d,%d,%v, want %d,%d", rs, re, ls, le, ok, tt.libS, tt.libE)
			}
		})
	}
}

func TestCustomToLibrary(t *testing.T) {
	split := &Custom{Rules: []CustomRule{
		{LibrarySeason: 1, ReleaseSeason: 1, From: 1, To: 12},
		{LibrarySeason: 2, ReleaseSeason: 1, Offset: 12},
	}}
	tests := []struct {
		name         string
		c            *Custom
		relS, relE   int
		wantS, wantE int
		wantOK       bool
	}{
		{"nil", nil, 15, 1, 0, 0, false},
		{"first half", split, 1, 5, 1, 5, true},
		{"second half", split, 1, 13, 2, 1, true},
		{"later in the second half", split, 1, 20, 2, 8, true},
		{"another release season", split, 2, 1, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e, ok := tt.c.ToLibrary(tt.relS, tt.relE)
			if ok != tt.wantOK || s != tt.wantS || e != tt.wantE {
				t.Fatalf("got %d,%d,%v", s, e, ok)
			}
		})
	}
	// A release episode below the offset is not an episode of the library.
	pos := &Custom{Rules: []CustomRule{{LibrarySeason: 2, ReleaseSeason: 1, Offset: 12}}}
	if _, _, ok := pos.ToLibrary(1, 5); ok {
		t.Fatal("release episode 5 would be library episode -7")
	}
	// A negative offset turns release episodes up into higher library ones.
	neg := &Custom{Rules: []CustomRule{{LibrarySeason: 3, ReleaseSeason: 2, Offset: -4}}}
	if s, e, ok := neg.ToLibrary(2, 3); !ok || s != 3 || e != 7 {
		t.Fatalf("got %d,%d,%v", s, e, ok)
	}
}

func TestCustomSeasonsForPacks(t *testing.T) {
	c := &Custom{Rules: []CustomRule{
		{LibrarySeason: 1, ReleaseSeason: 15},
		{LibrarySeason: 2, ReleaseSeason: 16},
		{LibrarySeason: 3, ReleaseSeason: 16, Offset: 20},
	}}
	if s, ok := c.SeasonToLibrary(15); !ok || s != 1 {
		t.Fatalf("pack S15 = %d,%v", s, ok)
	}
	if _, ok := c.SeasonToLibrary(16); ok {
		t.Fatal("release season 16 spreads over two library seasons: not one pack")
	}
	if _, ok := c.SeasonToLibrary(1); ok {
		t.Fatal("a season no rule names")
	}
	if got := c.ReleaseSeasons(2); !reflect.DeepEqual(got, []int{16}) {
		t.Fatalf("release seasons of library 2: %v", got)
	}
}

func TestCustomClean(t *testing.T) {
	in := Custom{
		Rules: []CustomRule{{LibrarySeason: 2, ReleaseSeason: 1, Offset: 12}, {LibrarySeason: 1, ReleaseSeason: 1}},
		Names: []string{"  The  Bachelor Ukraine ", "the bachelor ukraine", "", "Kholostyak"},
	}.Clean()
	if in.Rules[0].LibrarySeason != 1 || len(in.Rules) != 2 {
		t.Fatalf("rules not in order: %v", in.Rules)
	}
	if want := []string{"The Bachelor Ukraine", "Kholostyak"}; !reflect.DeepEqual(in.Names, want) {
		t.Fatalf("names = %q, want %q", in.Names, want)
	}
}

func TestRangesOverlap(t *testing.T) {
	tests := []struct {
		af, al, bf, bl int
		want           bool
	}{
		{1, 12, 13, 0, false},
		{1, 12, 12, 0, true},
		{0, 0, 5, 6, true},
		{1, 5, 6, 10, false},
		{3, 3, 3, 3, true},
	}
	for _, tt := range tests {
		if got := RangesOverlap(tt.af, tt.al, tt.bf, tt.bl); got != tt.want {
			t.Errorf("RangesOverlap(%d,%d,%d,%d) = %v", tt.af, tt.al, tt.bf, tt.bl, got)
		}
	}
}
