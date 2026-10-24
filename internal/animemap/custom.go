package animemap

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Limits on what a person can set for one show.
const (
	MaxCustomRules   = 24
	MaxCustomNames   = 12
	MaxCustomNameLen = 120
	MaxCustomSeason  = 1000
	MaxCustomEpisode = 10000
	MaxCustomOffset  = 10000
)

// CustomRule says that one stretch of a show's library numbering is called
// something else in release and file names: library season LibrarySeason (the
// movie database's), episodes From to To of it (0 = no limit), is season
// ReleaseSeason in names, with every episode number Offset higher
// (release episode = library episode + Offset).
type CustomRule struct {
	LibrarySeason int `json:"librarySeason"`
	ReleaseSeason int `json:"releaseSeason"`
	Offset        int `json:"offset,omitempty"`
	From          int `json:"from,omitempty"`
	To            int `json:"to,omitempty"`
}

// Custom is a person's own numbering and extra names for one show.
type Custom struct {
	Rules []CustomRule `json:"rules"`
	Names []string     `json:"names"`
}

// Empty reports whether there is nothing set.
func (c *Custom) Empty() bool { return c == nil || (len(c.Rules) == 0 && len(c.Names) == 0) }

func (r CustomRule) coversLibrary(season, episode int) bool {
	if r.LibrarySeason != season {
		return false
	}
	return (r.From == 0 || episode >= r.From) && (r.To == 0 || episode <= r.To)
}

// ReleaseRange is the release episode numbers the rule can produce: first and
// last, last 0 for no limit.
func (r CustomRule) ReleaseRange() (first, last int) {
	first = max(r.From, 1) + r.Offset
	if r.To > 0 {
		last = r.To + r.Offset
	}
	return first, last
}

func (r CustomRule) coversRelease(season, episode int) bool {
	if r.ReleaseSeason != season {
		return false
	}
	return r.coversLibrary(r.LibrarySeason, episode-r.Offset)
}

// ToRelease gives the season and episode that release and file names use for
// an episode of the library. ok is false when no rule covers it.
func (c *Custom) ToRelease(season, episode int) (rs, re int, ok bool) {
	if c == nil {
		return 0, 0, false
	}
	for _, r := range c.Rules {
		if r.coversLibrary(season, episode) {
			return r.ReleaseSeason, episode + r.Offset, true
		}
	}
	return 0, 0, false
}

// ToLibrary gives the episode of the library that a release or file season and
// episode stands for. ok is false when no rule covers it, or the rule would
// make an episode number below 1.
func (c *Custom) ToLibrary(season, episode int) (ls, le int, ok bool) {
	if c == nil {
		return 0, 0, false
	}
	for _, r := range c.Rules {
		if r.coversRelease(season, episode) && episode-r.Offset >= 1 {
			return r.LibrarySeason, episode - r.Offset, true
		}
	}
	return 0, 0, false
}

// SeasonToLibrary gives the library season a whole release season (a season
// pack) stands for. ok is false unless every rule for that release season
// points at one library season.
func (c *Custom) SeasonToLibrary(season int) (int, bool) {
	if c == nil {
		return 0, false
	}
	dest, found := 0, false
	for _, r := range c.Rules {
		if r.ReleaseSeason != season {
			continue
		}
		if found && dest != r.LibrarySeason {
			return 0, false
		}
		dest, found = r.LibrarySeason, true
	}
	return dest, found
}

// ReleaseSeasons are the seasons release names use for a library season, in
// the order of the rules, each once.
func (c *Custom) ReleaseSeasons(librarySeason int) []int {
	if c == nil {
		return nil
	}
	var out []int
	seen := map[int]bool{}
	for _, r := range c.Rules {
		if r.LibrarySeason == librarySeason && !seen[r.ReleaseSeason] {
			seen[r.ReleaseSeason] = true
			out = append(out, r.ReleaseSeason)
		}
	}
	return out
}

// Clean trims and tidies what a person entered: names trimmed and without
// repeats, rules in a steady order. It does not judge them (see Validate).
func (c Custom) Clean() Custom {
	out := Custom{Rules: append([]CustomRule{}, c.Rules...), Names: []string{}}
	sort.SliceStable(out.Rules, func(i, j int) bool {
		a, b := out.Rules[i], out.Rules[j]
		if a.LibrarySeason != b.LibrarySeason {
			return a.LibrarySeason < b.LibrarySeason
		}
		return a.From < b.From
	})
	seen := map[string]bool{}
	for _, n := range c.Names {
		n = strings.Join(strings.Fields(n), " ")
		if k := strings.ToLower(n); n != "" && !seen[k] {
			seen[k] = true
			out.Names = append(out.Names, n)
		}
	}
	return out
}

// RangesOverlap reports whether two ranges of episode numbers share one. 0 for the
// first number means "from the start", 0 for the last "no end".
func RangesOverlap(af, al, bf, bl int) bool {
	af, bf = max(af, 1), max(bf, 1)
	if al == 0 {
		al = MaxCustomEpisode * 10
	}
	if bl == 0 {
		bl = MaxCustomEpisode * 10
	}
	return af <= bl && bf <= al
}

// ---- storage

// GetCustom returns a show's own numbering (empty when it has none).
func (r *Repo) GetCustom(seriesID int64) (Custom, error) {
	var rules, names string
	err := r.db.QueryRow(`SELECT rules, names FROM series_numbering_custom WHERE series_id = ?`, seriesID).Scan(&rules, &names)
	if errors.Is(err, sql.ErrNoRows) {
		return Custom{Rules: []CustomRule{}, Names: []string{}}, nil
	}
	if err != nil {
		return Custom{}, fmt.Errorf("read own numbering of show %d: %w", seriesID, err)
	}
	c := Custom{Rules: []CustomRule{}, Names: []string{}}
	if rules != "" {
		if err := json.Unmarshal([]byte(rules), &c.Rules); err != nil {
			c.Rules = []CustomRule{} // a damaged row is the same as none
		}
	}
	if names != "" {
		if err := json.Unmarshal([]byte(names), &c.Names); err != nil {
			c.Names = []string{}
		}
	}
	return c, nil
}

// SaveCustom stores a show's own numbering; an empty one removes the row.
func (r *Repo) SaveCustom(seriesID int64, c Custom) error {
	if c.Empty() {
		if _, err := r.db.Exec(`DELETE FROM series_numbering_custom WHERE series_id = ?`, seriesID); err != nil {
			return fmt.Errorf("clear own numbering of show %d: %w", seriesID, err)
		}
		return nil
	}
	rules, err := json.Marshal(c.Rules)
	if err != nil {
		return fmt.Errorf("save own numbering of show %d: %w", seriesID, err)
	}
	names, err := json.Marshal(c.Names)
	if err != nil {
		return fmt.Errorf("save own numbering of show %d: %w", seriesID, err)
	}
	_, err = r.db.Exec(`INSERT INTO series_numbering_custom (series_id, rules, names) VALUES (?, ?, ?)
		ON CONFLICT (series_id) DO UPDATE SET rules = excluded.rules, names = excluded.names`, seriesID, string(rules), string(names))
	if err != nil {
		return fmt.Errorf("save own numbering of show %d: %w", seriesID, err)
	}
	return nil
}
