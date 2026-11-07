package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/rdborg/mediarium/internal/music"
)

const filterArtist = "11111111-1111-4111-8111-111111111111"

func gid(n int) string { return fmt.Sprintf("bbbbbbbb-0000-4000-8000-%012d", n) }

// filterMB is a fake MusicBrainz with one artist, nine release groups and, for
// each group, the statuses of its releases. It counts the release look-ups
// per group and the most that ran at the same time.
type filterMB struct {
	srv      *httptest.Server
	mu       sync.Mutex
	lookups  map[string]int
	running  atomic.Int32
	maxAtOne atomic.Int32
}

var filterGroups = []struct {
	n         int
	title     string
	primary   string
	secondary []string
	date      string
	releases  []string
}{
	{1, "Studio One", "Album", nil, "1990", []string{"Official"}},
	{2, "Bootleg Instrumentals", "Album", nil, "2003", []string{"Bootleg"}},
	{3, "Proper EP", "EP", nil, "1992", []string{"Official", "Official"}},
	{4, "Proper Single", "Single", nil, "1993", []string{"Promotion", "Official"}},
	{5, "Live Show", "Album", []string{"Live"}, "1995", []string{"Official"}},
	{6, "Remixed", "Album", []string{"Remix"}, "1996", []string{"Official"}},
	{7, "Mixed Editions", "Album", nil, "1998", []string{"Bootleg", "Official"}},
	{8, "Greatest Hits", "Album", []string{"Compilation"}, "2000", []string{"Official"}},
	{9, "Bootleg EP", "EP", nil, "2004", []string{"Bootleg", "Pseudo-Release"}},
}

func newFilterMB(t *testing.T) *filterMB {
	t.Helper()
	f := &filterMB{lookups: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/2/artist/"+filterArtist, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"` + filterArtist + `","name":"Fixture Band","sort-name":"Fixture Band"}`))
	})
	mux.HandleFunc("/ws/2/release-group", func(w http.ResponseWriter, r *http.Request) {
		var parts []string
		for _, g := range filterGroups {
			sec := make([]string, len(g.secondary))
			for i, s := range g.secondary {
				sec[i] = `"` + s + `"`
			}
			parts = append(parts, fmt.Sprintf(`{"id":%q,"title":%q,"primary-type":%q,"secondary-types":[%s],"first-release-date":%q}`,
				gid(g.n), g.title, g.primary, strings.Join(sec, ","), g.date))
		}
		fmt.Fprintf(w, `{"release-group-count":%d,"release-groups":[%s]}`, len(parts), strings.Join(parts, ","))
	})
	mux.HandleFunc("/ws/2/release", func(w http.ResponseWriter, r *http.Request) {
		now := f.running.Add(1)
		defer f.running.Add(-1)
		for {
			old := f.maxAtOne.Load()
			if now <= old || f.maxAtOne.CompareAndSwap(old, now) {
				break
			}
		}
		q := r.URL.Query()
		id := q.Get("release-group")
		f.mu.Lock()
		f.lookups[id]++
		f.mu.Unlock()
		var out []string
		for _, g := range filterGroups {
			if gid(g.n) != id {
				continue
			}
			for _, st := range g.releases {
				if want := q.Get("status"); want != "" && !strings.EqualFold(want, st) {
					continue
				}
				out = append(out, fmt.Sprintf(`{"id":"r%d","status":%q}`, len(out), st))
			}
		}
		if len(out) > 1 {
			out = out[:1] // limit=1
		}
		fmt.Fprintf(w, `{"releases":[%s]}`, strings.Join(out, ","))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *filterMB) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.lookups {
		n += c
	}
	return n
}

func newFilterEnv(t *testing.T) (*musicEnv, *filterMB) {
	t.Helper()
	e := newMusicEnv(t)
	mb := newFilterMB(t)
	e.server.TestSetMusicBrainz(mb.srv.URL)
	return e, mb
}

func albumTitles(artist map[string]any) []string {
	var titles []string
	albums, _ := artist["albums"].([]any)
	for _, a := range albums {
		titles = append(titles, a.(map[string]any)["title"].(string))
	}
	sort.Strings(titles)
	return titles
}

func (e *musicEnv) artistURL(artist map[string]any) string {
	return fmt.Sprintf("%s/api/music/artists/%v", e.base, artist["id"])
}

func (e *musicEnv) setLibraryFilter(t *testing.T, f map[string]any) {
	t.Helper()
	postJSONMethod[map[string]any](t, e.client, http.MethodPut, e.base+"/api/music/filters", f, http.StatusOK)
}

func sameStrings(got, want []string) bool {
	sort.Strings(want)
	return strings.Join(got, "|") == strings.Join(want, "|")
}

func TestDefaultFilterAddsWhatItAlwaysDid(t *testing.T) {
	e, mb := newFilterEnv(t)
	artist := e.addArtist(t, "all")
	want := []string{"Studio One", "Bootleg Instrumentals", "Proper EP", "Proper Single", "Mixed Editions", "Bootleg EP"}
	if got := albumTitles(artist); !sameStrings(got, want) {
		t.Fatalf("added %v, want %v", got, want)
	}
	if mb.total() != 0 {
		t.Errorf("the default filter must not look at single releases: %d look-ups", mb.total())
	}
	f := getJSON[map[string]any](t, e.client, e.artistURL(artist))["filter"].(map[string]any)
	if f["useLibrary"] != true || f["hideBootlegs"] != false || len(f["types"].([]any)) != 3 || len(f["hideSecondary"].([]any)) != 12 {
		t.Errorf("filter: %v", f)
	}
}

func TestLibraryFilterDecidesWhatFollowingAdds(t *testing.T) {
	e, _ := newFilterEnv(t)
	e.setLibraryFilter(t, map[string]any{"types": []string{"album", "ep"}, "hideSecondary": []string{"remix", "compilation", "other"}, "hideBootlegs": false})
	artist := e.addArtist(t, "all")
	// EPs and albums, live allowed; no single, no remix, no compilation.
	want := []string{"Studio One", "Bootleg Instrumentals", "Proper EP", "Live Show", "Mixed Editions", "Bootleg EP"}
	if got := albumTitles(artist); !sameStrings(got, want) {
		t.Fatalf("added %v, want %v", got, want)
	}
}

func TestBootlegOnlyGroupsAreLeftOutAndCheckedOnce(t *testing.T) {
	e, mb := newFilterEnv(t)
	e.setLibraryFilter(t, map[string]any{
		"types": []string{"album", "ep", "single"}, "hideBootlegs": true,
		"hideSecondary": []string{"live", "remix", "compilation", "dj-mix", "soundtrack", "demo", "mixtape", "spokenword", "interview", "audiobook", "audio-drama", "other"},
	})
	artist := e.addArtist(t, "all")
	if got := albumTitles(artist); len(got) != 0 {
		t.Fatalf("the artist page opens at once with what is certain, got %v", got)
	}
	e.server.TestWaitBackground()
	artist = getJSON[map[string]any](t, e.client, e.artistURL(artist))
	want := []string{"Studio One", "Proper EP", "Proper Single", "Mixed Editions"}
	if got := albumTitles(artist); !sameStrings(got, want) {
		t.Fatalf("after the look-ups: %v, want %v", got, want)
	}
	if mb.maxAtOne.Load() > 1 {
		t.Errorf("%d look-ups ran at the same time, they go one after the other", mb.maxAtOne.Load())
	}
	first := mb.total()
	if first == 0 {
		t.Fatal("no release was looked at")
	}

	// A refresh looks at nothing again: every answer is kept.
	e.server.TestRefreshMusicArtists(context.Background())
	if mb.total() != first {
		t.Errorf("the second pass made %d more requests", mb.total()-first)
	}
	// The bootleg-only groups were asked about once each, and no group more than twice (a second request only when the first found no official release).
	for id, n := range mb.lookups {
		if n > 2 {
			t.Errorf("%s was looked up %d times", id, n)
		}
	}
	// Adding the group by hand still works.
	post := e.artistURL(artist) + "/albums"
	postJSONMethod[map[string]any](t, e.client, http.MethodPost, post, map[string]any{"mbid": gid(2)}, http.StatusCreated)
	postJSONMethod[map[string]any](t, e.client, http.MethodPost, post, map[string]any{"mbid": gid(2)}, http.StatusConflict)
	postJSONMethod[map[string]any](t, e.client, http.MethodPost, post, map[string]any{"mbid": gid(77)}, http.StatusNotFound)
	postJSONMethod[map[string]any](t, e.client, http.MethodPost, post, map[string]any{"mbid": "nope"}, http.StatusBadRequest)
}

func TestDiscographySaysWhatIsHiddenAndWhy(t *testing.T) {
	e, mb := newFilterEnv(t)
	artist := e.addArtist(t, "all")
	url := e.artistURL(artist)
	d := getJSON[map[string]any](t, e.client, url+"/discography")
	counts := d["counts"].(map[string]any)
	if counts["live"] != float64(1) || counts["remix"] != float64(1) || counts["compilation"] != float64(1) || len(counts) != 3 || d["checking"] != float64(0) {
		t.Fatalf("counts: %v", d)
	}
	if len(d["hidden"].([]any)) != 3 {
		t.Fatalf("hidden: %v", d["hidden"])
	}
	if mb.total() != 0 {
		t.Errorf("%d release look-ups without the bootleg switch", mb.total())
	}

	// This artist alone looks for bootlegs and wants EPs only; everything in the library stays.
	got := postJSONMethod[map[string]any](t, e.client, http.MethodPut, url, map[string]any{
		"filter": map[string]any{"types": []string{"ep"}, "hideSecondary": []string{"live"}, "hideBootlegs": true},
	}, http.StatusOK)
	if f := got["filter"].(map[string]any); f["useLibrary"] != false || f["hideBootlegs"] != true {
		t.Fatalf("own filter: %v", f)
	}
	e.server.TestWaitBackground()
	if after := getJSON[map[string]any](t, e.client, url); len(albumTitles(after)) != 6 {
		t.Fatalf("changing the filter must not remove albums: %v", albumTitles(after))
	}
	d = getJSON[map[string]any](t, e.client, url+"/discography")
	counts = d["counts"].(map[string]any)
	// Not in the library and hidden: the live album, the remix and the compilation are albums, and this artist wants EPs only (the type is named, not the secondary type).
	if counts["album"] != float64(3) || len(counts) != 1 {
		t.Fatalf("counts with the own filter: %v", counts)
	}

	// Back to the library setting.
	got = postJSONMethod[map[string]any](t, e.client, http.MethodPut, url, map[string]any{"useLibraryFilter": true}, http.StatusOK)
	if got["filter"].(map[string]any)["useLibrary"] != true {
		t.Fatalf("back on the library filter: %v", got["filter"])
	}
	for _, bad := range []map[string]any{
		{"filter": map[string]any{"types": []string{}}},
		{"filter": map[string]any{"types": []string{"album"}, "hideSecondary": []string{"nonsense"}}},
	} {
		if status, _ := doJSONStatus(t, e.client, http.MethodPut, url, bad); status != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, status)
		}
	}
}

func TestDiscographyChecksBootlegsInTheBackground(t *testing.T) {
	e, mb := newFilterEnv(t)
	// Albums only at first: the EPs and singles are left out of the library.
	e.setLibraryFilter(t, map[string]any{"types": []string{"album"}, "hideSecondary": []string{"live", "remix", "compilation"}, "hideBootlegs": false})
	artist := e.addArtist(t, "all")
	url := e.artistURL(artist)
	if got := albumTitles(artist); !sameStrings(got, []string{"Studio One", "Bootleg Instrumentals", "Mixed Editions"}) {
		t.Fatalf("albums: %v", got)
	}
	postJSONMethod[map[string]any](t, e.client, http.MethodPut, url, map[string]any{
		"filter": map[string]any{"types": []string{"album", "ep", "single"}, "hideSecondary": []string{"live", "remix", "compilation"}, "hideBootlegs": true},
	}, http.StatusOK)
	e.server.TestWaitBackground() // the refresh that the change starts adds the EPs and the single

	after := getJSON[map[string]any](t, e.client, url)
	// The bootleg EP is left out, the other EP and the single are in.
	if got := albumTitles(after); !sameStrings(got, []string{"Studio One", "Bootleg Instrumentals", "Mixed Editions", "Proper EP", "Proper Single"}) {
		t.Fatalf("after the refresh: %v", got)
	}
	getJSON[map[string]any](t, e.client, url+"/discography") // starts the look-ups
	e.server.TestWaitBackground()
	d := getJSON[map[string]any](t, e.client, url+"/discography")
	if d["checking"] != float64(0) {
		t.Fatalf("still checking: %v", d)
	}
	counts := d["counts"].(map[string]any)
	if counts["bootleg"] != float64(1) || counts["live"] != float64(1) || counts["remix"] != float64(1) || counts["compilation"] != float64(1) {
		t.Fatalf("counts: %v", counts)
	}
	before := mb.total()
	getJSON[map[string]any](t, e.client, url+"/discography")
	e.server.TestWaitBackground()
	if mb.total() != before {
		t.Errorf("a second look at the page made %d more requests", mb.total()-before)
	}
}

func TestMusicFilterSettings(t *testing.T) {
	server, base, admin, member, _ := familyServer(t)
	server.TestSetMusicBrainz("http://127.0.0.1:1")
	postJSONMethod[map[string]any](t, admin, http.MethodPut, base+"/api/modules", map[string]any{"music": true}, http.StatusOK)

	got := getJSON[map[string]any](t, member, base+"/api/music/filters")
	f := got["filter"].(map[string]any)
	if len(f["types"].([]any)) != 3 || len(f["hideSecondary"].([]any)) != 12 || f["hideBootlegs"] != false {
		t.Fatalf("defaults: %v", f)
	}
	studio := got["studio"].(map[string]any)
	if len(studio["types"].([]any)) != 1 || studio["hideBootlegs"] != true || len(got["secondaryTypes"].([]any)) != 12 {
		t.Fatalf("preset: %v", got)
	}
	if status, _ := doJSONStatus(t, member, http.MethodPut, base+"/api/music/filters", studio); status != http.StatusForbidden {
		t.Fatalf("a member changed the library setting: %d", status)
	}
	if status, _ := doJSONStatus(t, admin, http.MethodPut, base+"/api/music/filters", map[string]any{"types": []string{}}); status != http.StatusBadRequest {
		t.Fatalf("no types: %d", status)
	}
	if status, _ := doJSONStatus(t, admin, http.MethodPut, base+"/api/music/filters", map[string]any{"types": []string{"album"}, "hideSecondary": []string{"x"}}); status != http.StatusBadRequest {
		t.Fatalf("unknown secondary type: %d", status)
	}
	postJSONMethod[map[string]any](t, admin, http.MethodPut, base+"/api/music/filters", studio, http.StatusOK)
	got = getJSON[map[string]any](t, member, base+"/api/music/filters")
	if f := got["filter"].(map[string]any); len(f["types"].([]any)) != 1 || f["hideBootlegs"] != true {
		t.Fatalf("saved: %v", f)
	}
	// Hiding no secondary type at all is a choice too.
	postJSONMethod[map[string]any](t, admin, http.MethodPut, base+"/api/music/filters", map[string]any{"types": []string{"album", "ep"}, "hideSecondary": []string{}}, http.StatusOK)
	got = getJSON[map[string]any](t, member, base+"/api/music/filters")
	if f := got["filter"].(map[string]any); len(f["hideSecondary"].([]any)) != 0 || len(f["types"].([]any)) != 2 {
		t.Fatalf("empty list: %v", f)
	}
}

// The albums in the library that the filter leaves out of the list are the
// ones without files; an album with files always stays, and no album is ever
// removed or unmonitored by a filter.
func TestDiscographyNamesLibraryAlbumsTheFilterHides(t *testing.T) {
	e, _ := newFilterEnv(t)
	artist := e.addArtist(t, "all")
	url := e.artistURL(artist)
	d := getJSON[map[string]any](t, e.client, url+"/discography")
	if lib := d["library"].([]any); len(lib) != 0 {
		t.Fatalf("the default filter hides nothing in the library: %v", lib)
	}

	// Studio One has been downloaded; it must stay whatever the filter says.
	id := int64(artist["id"].(float64))
	albums, err := e.server.MusicRepo.ListAlbums(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range albums {
		if a.MBID == gid(1) {
			if err := e.server.MusicRepo.SetAlbumImported(a.ID, "FLAC", t.TempDir()); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Mixed Editions is only partly downloaded: one of its two tracks has a file.
	for _, a := range albums {
		if a.MBID == gid(7) {
			tracks := []music.Track{{Disc: 1, Position: 1, Title: "One"}, {Disc: 1, Position: 2, Title: "Two"}}
			if err := e.server.MusicRepo.ReplaceTracks(a.ID, tracks); err != nil {
				t.Fatal(err)
			}
			have, err := e.server.MusicRepo.ListTracks(a.ID)
			if err != nil || len(have) != 2 {
				t.Fatalf("tracks: %v %v", have, err)
			}
			if err := e.server.MusicRepo.SetTrackFile(have[0].ID, "/music/one.flac"); err != nil {
				t.Fatal(err)
			}
		}
	}
	postJSONMethod[map[string]any](t, e.client, http.MethodPut, url, map[string]any{
		"filter": map[string]any{"types": []string{"ep"}, "hideSecondary": []string{"live"}, "hideBootlegs": false},
	}, http.StatusOK)
	e.server.TestWaitBackground()
	d = getJSON[map[string]any](t, e.client, url+"/discography")
	got := map[string]string{}
	for _, x := range d["library"].([]any) {
		m := x.(map[string]any)
		got[m["mbid"].(string)] = m["hiddenBy"].(string)
	}
	want := map[string]string{gid(2): "album", gid(4): "single"}
	if len(got) != len(want) {
		t.Fatalf("library hidden: %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: hidden by %q, want %q", k, got[k], v)
		}
	}
	for _, n := range []int{1, 7} {
		if _, ok := got[gid(n)]; ok {
			t.Errorf("album %d has files (whole or partly) and must never be listed as hidden", n)
		}
	}
	if after := getJSON[map[string]any](t, e.client, url); len(albumTitles(after)) != 6 {
		t.Fatalf("a filter must not remove albums: %v", albumTitles(after))
	}
}
