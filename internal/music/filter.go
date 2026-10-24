package music

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// What the artist pages list and what following an artist adds: the release
// types to keep, the secondary types (live, remix...) to leave out, and
// whether release groups that only exist as bootlegs are left out too. The
// defaults are the behaviour from before the choice existed: albums, EPs and
// singles, and every group with a secondary type hidden.

// SecondaryType is one secondary type a person can hide. Key is the stable
// name used in settings and the API; Name is what MusicBrainz calls it.
type SecondaryType struct {
	Key  string
	Name string
}

// SecondaryTypes are the secondary types that can be hidden one by one. The
// last, "other", stands for any type MusicBrainz adds that is not listed.
var SecondaryTypes = []SecondaryType{
	{"live", "Live"},
	{"remix", "Remix"},
	{"compilation", "Compilation"},
	{"dj-mix", "DJ-mix"},
	{"soundtrack", "Soundtrack"},
	{"demo", "Demo"},
	{"mixtape", "Mixtape/Street"},
	{"spokenword", "Spokenword"},
	{"interview", "Interview"},
	{"audiobook", "Audiobook"},
	{"audio-drama", "Audio drama"},
	{"other", "Other"},
}

// ReleaseTypes are the primary types that can be kept, in display order.
var ReleaseTypes = []string{TypeAlbum, TypeEP, TypeSingle}

// ReasonBootleg is the reason a bootleg-only release group is left out.
const ReasonBootleg = "bootleg"

// SecondaryKey turns a MusicBrainz secondary type name into its key.
func SecondaryKey(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range SecondaryTypes {
		if s.Key != "other" && strings.ToLower(s.Name) == n {
			return s.Key
		}
	}
	return "other"
}

// TypeKey maps a MusicBrainz primary type onto album, ep or single, like the
// type an album is stored with.
func TypeKey(primary string) string {
	switch strings.ToLower(primary) {
	case "ep":
		return TypeEP
	case "single":
		return TypeSingle
	}
	return TypeAlbum
}

// Group is the part of a release group the filter looks at.
type Group struct {
	PrimaryType    string
	SecondaryTypes []string
}

// Filter says which release groups pass. Types and HideSecondary hold keys.
type Filter struct {
	Types         []string `json:"types"`
	HideSecondary []string `json:"hideSecondary"`
	HideBootlegs  bool     `json:"hideBootlegs"`
}

// DefaultFilter is the choice nobody has changed: albums, EPs and singles,
// every secondary type hidden, bootlegs not looked for.
func DefaultFilter() Filter {
	f := Filter{Types: append([]string{}, ReleaseTypes...), HideSecondary: []string{}}
	for _, s := range SecondaryTypes {
		f.HideSecondary = append(f.HideSecondary, s.Key)
	}
	return f
}

// StudioOnly is the preset "Official studio releases only": albums only,
// every secondary type hidden, bootleg-only groups hidden.
func StudioOnly() Filter {
	f := DefaultFilter()
	f.Types = []string{TypeAlbum}
	f.HideBootlegs = true
	return f
}

// Normalize checks the lists and puts them in display order without
// duplicates. At least one release type has to stay.
func (f Filter) Normalize() (Filter, error) {
	types, bad := pickKeys(f.Types, ReleaseTypes)
	if bad != "" {
		return Filter{}, fmt.Errorf("unknown release type %q", bad)
	}
	if len(types) == 0 {
		return Filter{}, errors.New("choose at least one release type")
	}
	all := make([]string, len(SecondaryTypes))
	for i, s := range SecondaryTypes {
		all[i] = s.Key
	}
	hide, bad := pickKeys(f.HideSecondary, all)
	if bad != "" {
		return Filter{}, fmt.Errorf("unknown secondary type %q", bad)
	}
	return Filter{Types: types, HideSecondary: hide, HideBootlegs: f.HideBootlegs}, nil
}

// pickKeys returns the members of order that are in keys (in order's order),
// or the first key that is not in order at all.
func pickKeys(keys, order []string) ([]string, string) {
	have := map[string]bool{}
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		ok := false
		for _, o := range order {
			if o == k {
				ok = true
			}
		}
		if !ok {
			return nil, k
		}
		have[k] = true
	}
	out := []string{}
	for _, o := range order {
		if have[o] {
			out = append(out, o)
		}
	}
	return out, ""
}

// Reason says why a group is left out for its types: its release type
// ("album", "ep", "single") or the key of the first secondary type that is
// hidden. "" means it passes. The bootleg test needs a lookup and is made by
// the caller (ReasonBootleg).
func (f Filter) Reason(g Group) string {
	typ := TypeKey(g.PrimaryType)
	kept := false
	for _, t := range f.Types {
		if t == typ {
			kept = true
		}
	}
	if !kept {
		return typ
	}
	for _, name := range g.SecondaryTypes {
		key := SecondaryKey(name)
		for _, h := range f.HideSecondary {
			if h == key {
				return key
			}
		}
	}
	return ""
}

// EncodeKeys writes a list of keys for storing ("none" when it is empty).
func EncodeKeys(keys []string) string {
	if len(keys) == 0 {
		return "none"
	}
	return strings.Join(keys, ",")
}

// DecodeKeys is EncodeKeys backwards. ok is false for an unset value.
func DecodeKeys(v string) (keys []string, ok bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, false
	}
	if v == "none" {
		return []string{}, true
	}
	return strings.Split(v, ","), true
}

// ArtistFilter returns the choice made for one artist, or nil when the artist
// uses the library setting.
func (r *Repo) ArtistFilter(artistID int64) (*Filter, error) {
	var types, secondary sql.NullString
	var bootlegs sql.NullBool
	err := r.db.QueryRow(`SELECT filter_types, filter_secondary, filter_bootlegs FROM artists WHERE id = ?`, artistID).Scan(&types, &secondary, &bootlegs)
	if err != nil {
		return nil, fmt.Errorf("read filter of artist %d: %w", artistID, err)
	}
	if !types.Valid && !secondary.Valid && !bootlegs.Valid {
		return nil, nil
	}
	f := DefaultFilter()
	if k, ok := DecodeKeys(types.String); ok {
		f.Types = k
	}
	if k, ok := DecodeKeys(secondary.String); ok {
		f.HideSecondary = k
	}
	if bootlegs.Valid {
		f.HideBootlegs = bootlegs.Bool
	}
	return &f, nil
}

// SetArtistFilter stores one artist's own choice; nil goes back to the
// library setting.
func (r *Repo) SetArtistFilter(artistID int64, f *Filter) error {
	var res sql.Result
	var err error
	if f == nil {
		res, err = r.db.Exec(`UPDATE artists SET filter_types = NULL, filter_secondary = NULL, filter_bootlegs = NULL WHERE id = ?`, artistID)
	} else {
		res, err = r.db.Exec(`UPDATE artists SET filter_types = ?, filter_secondary = ?, filter_bootlegs = ? WHERE id = ?`,
			EncodeKeys(f.Types), EncodeKeys(f.HideSecondary), f.HideBootlegs, artistID)
	}
	if err != nil {
		return fmt.Errorf("set filter of artist %d: %w", artistID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("set filter of artist %d: %w", artistID, sql.ErrNoRows)
	}
	return nil
}

// GroupStatusMaxAge is how long a found answer about a release group is
// trusted before it is looked up again.
const GroupStatusMaxAge = 180 * 24 * time.Hour

const statusTime = "2006-01-02 15:04:05"

// GroupStatuses returns, for the release groups asked about, whether each has
// an official release. Groups never looked at, or looked at too long ago,
// are not in the result.
func (r *Repo) GroupStatuses(mbids []string, now time.Time) (map[string]bool, error) {
	out := map[string]bool{}
	cutoff := now.Add(-GroupStatusMaxAge).UTC().Format(statusTime)
	for _, id := range mbids {
		var official bool
		err := r.db.QueryRow(`SELECT official FROM music_group_status WHERE mbid = ? AND checked_at >= ?`, id, cutoff).Scan(&official)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read status of release group %s: %w", id, err)
		}
		out[id] = official
	}
	return out, nil
}

// SetGroupStatus remembers whether a release group has an official release.
func (r *Repo) SetGroupStatus(mbid string, official bool, now time.Time) error {
	_, err := r.db.Exec(`INSERT INTO music_group_status (mbid, official, checked_at) VALUES (?, ?, ?)
		ON CONFLICT (mbid) DO UPDATE SET official = excluded.official, checked_at = excluded.checked_at`,
		mbid, official, now.UTC().Format(statusTime))
	if err != nil {
		return fmt.Errorf("save status of release group %s: %w", mbid, err)
	}
	return nil
}
