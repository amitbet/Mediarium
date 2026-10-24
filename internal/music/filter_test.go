package music_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/music"
)

func TestFilterReason(t *testing.T) {
	cases := []struct {
		name   string
		filter music.Filter
		group  music.Group
		want   string
	}{
		{"default keeps a plain album", music.DefaultFilter(), music.Group{PrimaryType: "Album"}, ""},
		{"default keeps an EP", music.DefaultFilter(), music.Group{PrimaryType: "EP"}, ""},
		{"default keeps a single", music.DefaultFilter(), music.Group{PrimaryType: "Single"}, ""},
		{"default hides live", music.DefaultFilter(), music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Live"}}, "live"},
		{"default hides a compilation", music.DefaultFilter(), music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Compilation"}}, "compilation"},
		{"default hides a DJ-mix", music.DefaultFilter(), music.Group{PrimaryType: "Album", SecondaryTypes: []string{"DJ-mix"}}, "dj-mix"},
		{"default hides a type MusicBrainz adds later", music.DefaultFilter(), music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Field recording"}}, "other"},
		{"default hides a live single", music.DefaultFilter(), music.Group{PrimaryType: "Single", SecondaryTypes: []string{"Live"}}, "live"},
		{"the first hidden secondary type is named", music.DefaultFilter(), music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Remix", "Live"}}, "remix"},
		{"no EPs", music.Filter{Types: []string{"album", "single"}, HideSecondary: []string{}}, music.Group{PrimaryType: "EP"}, "ep"},
		{"no singles", music.Filter{Types: []string{"album", "ep"}, HideSecondary: []string{}}, music.Group{PrimaryType: "Single"}, "single"},
		{"a broadcast counts as an album", music.Filter{Types: []string{"album"}, HideSecondary: []string{}}, music.Group{PrimaryType: "Broadcast"}, ""},
		{"live allowed", music.Filter{Types: []string{"album"}, HideSecondary: []string{"remix"}}, music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Live"}}, ""},
		{"live allowed but remix hidden on the same group", music.Filter{Types: []string{"album"}, HideSecondary: []string{"remix"}}, music.Group{PrimaryType: "Album", SecondaryTypes: []string{"Live", "Remix"}}, "remix"},
		{"the type wins over the secondary type", music.Filter{Types: []string{"album"}, HideSecondary: []string{"live"}}, music.Group{PrimaryType: "EP", SecondaryTypes: []string{"Live"}}, "ep"},
		{"names are matched whatever the case", music.DefaultFilter(), music.Group{PrimaryType: "album", SecondaryTypes: []string{"mixtape/street"}}, "mixtape"},
	}
	for _, c := range cases {
		if got := c.filter.Reason(c.group); got != c.want {
			t.Errorf("%s: Reason = %q, want %q", c.name, got, c.want)
		}
	}
}

// The default filter has to do exactly what the app did before the choice
// existed: ArtistReleaseGroups kept a group when it had no secondary type at
// all, and fetched the three types.
func TestDefaultFilterReproducesTheOldBehaviour(t *testing.T) {
	f := music.DefaultFilter()
	if f.HideBootlegs {
		t.Error("bootleg-only groups must be listed by default")
	}
	for _, primary := range []string{"Album", "EP", "Single"} {
		if r := f.Reason(music.Group{PrimaryType: primary}); r != "" {
			t.Errorf("%s without a secondary type was hidden: %q", primary, r)
		}
		for _, sec := range []string{"Compilation", "Soundtrack", "Spokenword", "Interview", "Audiobook", "Audio drama", "Live", "Remix", "DJ-mix", "Mixtape/Street", "Demo", "Something new"} {
			if r := f.Reason(music.Group{PrimaryType: primary, SecondaryTypes: []string{sec}}); r == "" {
				t.Errorf("%s + %s was listed, it used to be left out", primary, sec)
			}
		}
	}
}

func TestStudioPreset(t *testing.T) {
	f := music.StudioOnly()
	if !reflect.DeepEqual(f.Types, []string{"album"}) || !f.HideBootlegs || len(f.HideSecondary) != len(music.SecondaryTypes) {
		t.Errorf("preset = %+v", f)
	}
	if _, err := f.Normalize(); err != nil {
		t.Errorf("the preset is not valid: %v", err)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name    string
		in      music.Filter
		want    music.Filter
		wantErr bool
	}{
		{"orders and removes duplicates", music.Filter{Types: []string{"single", "ALBUM", "single"}, HideSecondary: []string{"remix", "live"}},
			music.Filter{Types: []string{"album", "single"}, HideSecondary: []string{"live", "remix"}}, false},
		{"hiding nothing is allowed", music.Filter{Types: []string{"ep"}}, music.Filter{Types: []string{"ep"}, HideSecondary: []string{}}, false},
		{"no types", music.Filter{HideSecondary: []string{"live"}}, music.Filter{}, true},
		{"unknown type", music.Filter{Types: []string{"album", "boxset"}}, music.Filter{}, true},
		{"unknown secondary type", music.Filter{Types: []string{"album"}, HideSecondary: []string{"nope"}}, music.Filter{}, true},
	}
	for _, c := range cases {
		got, err := c.in.Normalize()
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v", c.name, err)
			continue
		}
		if err == nil && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestEncodeDecodeKeys(t *testing.T) {
	for _, keys := range [][]string{{"album", "ep"}, {"live"}, {}} {
		got, ok := music.DecodeKeys(music.EncodeKeys(keys))
		if !ok || !reflect.DeepEqual(got, keys) {
			t.Errorf("%v came back as %v (%v)", keys, got, ok)
		}
	}
	if _, ok := music.DecodeKeys(""); ok {
		t.Error("an unset value must stay unset")
	}
}

func TestArtistFilterIsStoredPerArtist(t *testing.T) {
	r := newRepo(t)
	a, _, err := r.AddArtist(music.Artist{MBID: "mb-a", Name: "A"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := r.ArtistFilter(a.ID); err != nil || f != nil {
		t.Fatalf("a new artist uses the library setting: %v %v", f, err)
	}
	own := music.Filter{Types: []string{"album", "ep"}, HideSecondary: []string{}, HideBootlegs: true}
	if err := r.SetArtistFilter(a.ID, &own); err != nil {
		t.Fatal(err)
	}
	got, err := r.ArtistFilter(a.ID)
	if err != nil || got == nil || !reflect.DeepEqual(*got, own) {
		t.Fatalf("stored %+v, read %+v (%v)", own, got, err)
	}
	if err := r.SetArtistFilter(a.ID, nil); err != nil {
		t.Fatal(err)
	}
	if f, _ := r.ArtistFilter(a.ID); f != nil {
		t.Fatalf("back on the library setting, got %+v", f)
	}
	if err := r.SetArtistFilter(999, nil); err == nil {
		t.Error("an unknown artist must be an error")
	}
}

func TestGroupStatusesExpire(t *testing.T) {
	r := newRepo(t)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	if err := r.SetGroupStatus("rg-official", true, now); err != nil {
		t.Fatal(err)
	}
	if err := r.SetGroupStatus("rg-bootleg", false, now); err != nil {
		t.Fatal(err)
	}
	if err := r.SetGroupStatus("rg-old", false, now.Add(-music.GroupStatusMaxAge-time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := r.GroupStatuses([]string{"rg-official", "rg-bootleg", "rg-old", "rg-never"}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"rg-official": true, "rg-bootleg": false}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// A new answer replaces the old one.
	if err := r.SetGroupStatus("rg-bootleg", true, now); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.GroupStatuses([]string{"rg-bootleg"}, now); !got["rg-bootleg"] {
		t.Error("the new answer was not kept")
	}
}
