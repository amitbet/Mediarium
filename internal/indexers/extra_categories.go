package indexers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The kinds of media an indexer can be given extra categories for.
const (
	MediaMovies     = "movies"
	MediaTV         = "tv"
	MediaMusic      = "music"
	MediaBooks      = "books"
	MediaAudiobooks = "audiobooks"
)

// MediaKinds lists the kinds in the order the settings show them.
var MediaKinds = []string{MediaMovies, MediaTV, MediaMusic, MediaBooks, MediaAudiobooks}

// Limits for what a person types: category numbers are whole numbers, and
// the biggest ones indexers use for their own categories have six digits.
const (
	MaxExtraCategory     = 999999
	MaxExtraCategoriesOf = 30 // per kind of media
)

// ExtraCategories are the category numbers an indexer is searched in besides the
// standard ones, per kind of media. Newznab and Torznab indexers often file
// releases under a number of their own (105000 for TV, 7120 for German
// ebooks), and a search for the standard number never finds those.
type ExtraCategories map[string][]int

// IsMediaKind reports whether kind is one of MediaKinds.
func IsMediaKind(kind string) bool {
	for _, k := range MediaKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Normalize returns the categories with every list sorted and free of repeats,
// and the kinds with nothing in them left out. Nil when nothing is left.
func (e ExtraCategories) Normalize() ExtraCategories {
	var out ExtraCategories
	for kind, list := range e {
		seen := map[int]bool{}
		var clean []int
		for _, n := range list {
			if n > 0 && !seen[n] {
				seen[n] = true
				clean = append(clean, n)
			}
		}
		if len(clean) == 0 {
			continue
		}
		sort.Ints(clean)
		if out == nil {
			out = ExtraCategories{}
		}
		out[kind] = clean
	}
	return out
}

// Validate says, in plain words, what is wrong with the categories, or "" when
// they can be saved.
func (e ExtraCategories) Validate() string {
	for kind, list := range e {
		if !IsMediaKind(kind) {
			return fmt.Sprintf("Extra categories can be set for movies, TV, music, books and audiobooks, not for %q.", kind)
		}
		for _, n := range list {
			if n < 1 || n > MaxExtraCategory {
				return fmt.Sprintf("%d is not a category number. Category numbers are whole numbers from 1 to %d, for example 105000.", n, MaxExtraCategory)
			}
		}
		if len(e.Normalize()[kind]) > MaxExtraCategoriesOf {
			return fmt.Sprintf("That is too many extra categories for one kind of media. Use at most %d.", MaxExtraCategoriesOf)
		}
	}
	return ""
}

// Encode is the JSON that is stored: "" when there are no extra categories.
func (e ExtraCategories) Encode() string {
	n := e.Normalize()
	if len(n) == 0 {
		return ""
	}
	b, err := json.Marshal(n)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeExtraCategories reads what Encode stored. Anything that can't be read
// is treated as no extra categories: an indexer must still be searched.
func DecodeExtraCategories(s string) ExtraCategories {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var e ExtraCategories
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		return nil
	}
	for kind := range e {
		if !IsMediaKind(kind) {
			delete(e, kind)
		}
	}
	return e.Normalize()
}

// ParseCategoryList reads category numbers a person typed: whole numbers
// separated by commas, semicolons or spaces. Empty text is an empty list.
func ParseCategoryList(text string) ([]int, error) {
	var out []int
	for _, f := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' }) {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > MaxExtraCategory {
			return nil, fmt.Errorf("%q is not a category number. Use whole numbers from 1 to %d, separated by commas.", f, MaxExtraCategory)
		}
		out = append(out, n)
	}
	return out, nil
}

// mediaKindsOf says which kinds of media a search in the standard categories
// is for: 2xxx is movies, 5xxx TV, 7xxx books, 3030 audiobooks and the rest of
// 3xxx music.
func mediaKindsOf(cats []int) []string {
	var kinds []string
	add := func(k string) {
		for _, have := range kinds {
			if have == k {
				return
			}
		}
		kinds = append(kinds, k)
	}
	for _, c := range cats {
		switch {
		case c >= 2000 && c < 3000:
			add(MediaMovies)
		case c == 3030:
			add(MediaAudiobooks)
		case c >= 3000 && c < 4000:
			add(MediaMusic)
		case c >= 5000 && c < 6000:
			add(MediaTV)
		case c >= 7000 && c < 8000:
			add(MediaBooks)
		}
	}
	return kinds
}

// WithExtraCategories returns the categories to search an indexer in: the
// standard ones asked for, then the indexer's extra categories for the kinds
// of media they are for. Nothing is added to a search with no categories (it
// asks for everything), and the list given is never changed.
func (e ExtraCategories) WithExtraCategories(cats []int) []int {
	if len(e) == 0 || len(cats) == 0 {
		return cats
	}
	out := append([]int(nil), cats...)
	have := map[int]bool{}
	for _, c := range out {
		have[c] = true
	}
	for _, kind := range mediaKindsOf(cats) {
		for _, n := range e[kind] {
			if !have[n] {
				have[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}
