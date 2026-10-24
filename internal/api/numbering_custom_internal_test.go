package api

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/animemap"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
)

func TestMapCustom(t *testing.T) {
	season := &animemap.Custom{Rules: []animemap.CustomRule{{LibrarySeason: 1, ReleaseSeason: 15}}}
	split := &animemap.Custom{Rules: []animemap.CustomRule{
		{LibrarySeason: 1, ReleaseSeason: 1, From: 1, To: 12},
		{LibrarySeason: 2, ReleaseSeason: 1, Offset: 12},
	}}
	tests := []struct {
		name     string
		c        *animemap.Custom
		release  string
		ok       bool
		season   int
		episodes []int
	}{
		{"no rules: unchanged", nil, "Show.S15E01.720p.HDTV-GRP", false, 15, []int{1}},
		{"episode", season, "Show.S15E01.720p.HDTV-GRP", true, 1, []int{1}},
		{"two episodes in one file name", season, "Show.S15E01E02.720p.HDTV-GRP", true, 1, []int{1, 2}},
		{"a season pack", season, "Show.S15.720p.HDTV-GRP", true, 1, nil},
		{"a season the rules do not name", season, "Show.S14E01.720p.HDTV-GRP", false, 14, []int{1}},
		{"continuing numbers go to the next library season", split, "Show.S01E13.720p.HDTV-GRP", true, 2, []int{1}},
		{"episodes that cross the split are left alone", split, "Show.S01E12E13.720p.HDTV-GRP", false, 1, []int{12, 13}},
		{"a pack over several seasons is left alone", season, "Show.S15-S16.720p.HDTV-GRP", false, 15, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := mapCustom(parser.Parse(tt.release), tt.c)
			if ok != tt.ok || got.Season != tt.season || !reflect.DeepEqual(got.Episodes, tt.episodes) {
				t.Fatalf("got ok=%v S%d %v, want ok=%v S%d %v", ok, got.Season, got.Episodes, tt.ok, tt.season, tt.episodes)
			}
		})
	}
}

// The person's rules win over TheXEM: a scene row that says S15E01 is another
// episode does not matter when a rule covers the name.
func TestCustomRulesBeatTheXEM(t *testing.T) {
	num := &animemap.Numbering{Scene: []animemap.SceneEpisode{{SceneSeason: 15, SceneEpisode: 1, Season: 3, Episode: 9}}}
	rel := parser.Parse("Show.S15E01.720p.HDTV-GRP")
	if got, ok := mapNumbered(rel, num, nil); !ok || got.Season != 3 {
		t.Fatalf("XEM alone: %v %v", got.Season, ok)
	}
	c := &animemap.Custom{Rules: []animemap.CustomRule{{LibrarySeason: 1, ReleaseSeason: 15}}}
	got, ok := mapCustom(rel, c)
	if !ok || got.Season != 1 || got.Episodes[0] != 1 {
		t.Fatalf("the rule: %v %v", got, ok)
	}
}

func TestCustomQueries(t *testing.T) {
	c := &animemap.Custom{
		Rules: []animemap.CustomRule{{LibrarySeason: 1, ReleaseSeason: 15}},
		Names: []string{"The Bachelor Ukraine", "Kholostyak"},
	}
	tests := []struct {
		name            string
		c               *animemap.Custom
		season, episode int
		want            []string
	}{
		{"nothing set", nil, 1, 1, nil},
		{"an episode under the rule", c, 1, 3, []string{
			"Holostyak S15E03", "The Bachelor Ukraine S15E03", "Kholostyak S15E03",
		}},
		{"a season pack", c, 1, 0, []string{"Holostyak S15", "The Bachelor Ukraine S15", "Kholostyak S15"}},
		{"a season no rule covers still gets the extra names", c, 2, 4, []string{
			"The Bachelor Ukraine S02E04", "Kholostyak S02E04",
		}},
		{"only names: library numbering", &animemap.Custom{Names: []string{"Kholostyak"}}, 1, 2, []string{"Kholostyak S01E02"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := customQueries("Holostyak", tt.c, tt.season, tt.episode)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	many := &animemap.Custom{Names: []string{"Name One", "Name Two", "Name Three", "Name Four", "Name Five", "Name Six"}}
	if got := customQueries("Show", many, 1, 1); len(got) != maxCustomQueries {
		t.Fatalf("at most %d extra searches, got %d", maxCustomQueries, len(got))
	}
}

func TestCheckCustom(t *testing.T) {
	seasons := map[int]bool{1: true, 2: true}
	rule := func(ls, rs, off, from, to int) animemap.CustomRule {
		return animemap.CustomRule{LibrarySeason: ls, ReleaseSeason: rs, Offset: off, From: from, To: to}
	}
	tests := []struct {
		name    string
		c       animemap.Custom
		wantErr string
	}{
		{"empty is fine", animemap.Custom{}, ""},
		{"a whole season", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 15, 0, 0, 0)}}, ""},
		{"a split season", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 1, 0, 1, 12), rule(2, 1, 12, 0, 0)}}, ""},
		{"season 0", animemap.Custom{Rules: []animemap.CustomRule{rule(0, 15, 0, 0, 0)}}, "library season"},
		{"release season too big", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 5000, 0, 0, 0)}}, "release season"},
		{"offset too big", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 1, 99999, 0, 0)}}, "offset"},
		{"range backwards", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 1, 0, 8, 3)}}, "comes after"},
		{"negative episode", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 1, 0, -1, 0)}}, "between 1 and"},
		{"offset that gives episode 0", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 1, -5, 3, 0)}}, "Episodes start at 1"},
		{"season the show lacks", animemap.Custom{Rules: []animemap.CustomRule{rule(7, 1, 0, 0, 0)}}, "no season 7"},
		{"two rules for the same episodes", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 15, 0, 0, 0), rule(1, 16, 0, 5, 0)}}, "same episodes"},
		{"two rules that make one release mean two episodes", animemap.Custom{Rules: []animemap.CustomRule{rule(1, 15, 0, 0, 0), rule(2, 15, 0, 0, 0)}}, "two episodes"},
		{"name too short", animemap.Custom{Names: []string{"ab"}}, "too short"},
		{"name too long", animemap.Custom{Names: []string{strings.Repeat("x", 200)}}, "at most"},
		{"too many names", animemap.Custom{Names: make([]string, 13)}, "more than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCustom(tt.c, seasons)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("got %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestMatchesShowWithOwnName(t *testing.T) {
	sr := library.Series{Title: "Холостяк", Year: 2026, Aliases: []string{"The Bachelor Ukraine"}}
	if !matchesShow("The.Bachelor.Ukraine.S15E01.1080p.WEB-GRP", sr) {
		t.Fatal("a release under the extra name must be taken for the show")
	}
	if matchesShow("The.Bachelor.S15E01.1080p.WEB-GRP", sr) {
		t.Fatal("another show must not")
	}
}
