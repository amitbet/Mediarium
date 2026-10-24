package organizer_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/organizer"
)

func scheme(t *testing.T, id string) organizer.Scheme {
	t.Helper()
	for _, s := range organizer.Schemes {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no scheme %q", id)
	return organizer.Scheme{}
}

// The TRaSH recommended schemes render the sample from TRaSH's naming guide
// (without the parts Mediarium can't write: audio channels, bit depth).
func TestTrashSchemes(t *testing.T) {
	full := organizer.NamingContext{
		MovieTitle: "The Movie Title", Year: 2010, Edition: "Ultimate Extended Edition", Is3D: true,
		Quality: "2160p", Source: "Bluray", Proper: true, AudioCodec: "EAC3 Atmos", HDR: "DV HDR10", Codec: "x265",
		ReleaseGroup: "RlsGrp", TMDBID: 345691,
	}
	plain := organizer.NamingContext{
		MovieTitle: "The Movie Title: Again & More", Year: 2010, Quality: "1080p", Source: "WEB-DL",
		Codec: "x264", ReleaseGroup: "RlsGrp", TMDBID: 345691,
	}
	tests := []struct {
		name string
		id   string
		ctx  organizer.NamingContext
		want string
	}{
		{"standard, everything", "trash", full, "The Movie Title (2010) - {Ultimate Extended Edition} [3D][Bluray-2160p Proper][EAC3 Atmos][DV HDR10][x265]-RlsGrp"},
		{"standard, little known", "trash", plain, "The Movie Title Again and More (2010) - [WEBDL-1080p][x264]-RlsGrp"},
		{"plex tmdb", "trash-plex-tmdb", full, "The Movie Title (2010) {tmdb-345691} - {edition-Ultimate Extended Edition} [3D][Bluray-2160p Proper][EAC3 Atmos][DV HDR10][x265]-RlsGrp"},
		{"plex tmdb, no edition", "trash-plex-tmdb", plain, "The Movie Title Again and More (2010) {tmdb-345691} - [WEBDL-1080p][x264]-RlsGrp"},
		{"emby tmdb", "trash-emby-tmdb", full, "The Movie Title (2010) [tmdb-345691] - {edition-Ultimate Extended Edition} [3D][Bluray-2160p Proper][EAC3 Atmos][DV HDR10][x265]-RlsGrp"},
		{"jellyfin tmdb", "trash-jellyfin-tmdb", full, "The Movie Title (2010) [tmdbid-345691] - {Ultimate Extended Edition} [3D][Bluray-2160p Proper][EAC3 Atmos][DV HDR10][x265]-RlsGrp"},
	}
	for _, tt := range tests {
		t.Run("movie "+tt.name, func(t *testing.T) {
			if got := organizer.Render(scheme(t, tt.id).Movie, tt.ctx); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}

	ep := organizer.NamingContext{
		SeriesTitle: "The Series Title!", Year: 2010, Season: 1, Episode: 1, EpisodeTitle: "Episode Title 1",
		Quality: "1080p", Source: "WEB-DL", Proper: true, AudioCodec: "DTS", HDR: "DV HDR10", Codec: "x264", ReleaseGroup: "RlsGrp",
	}
	bare := organizer.NamingContext{SeriesTitle: "Show", Year: 2019, Season: 12, Episode: 3, EpisodeTitle: "", Quality: "720p", Source: "HDTV"}
	eps := []struct {
		name string
		ctx  organizer.NamingContext
		want string
	}{
		{"everything", ep, "The Series Title (2010) - S01E01 - Episode Title 1 [WEBDL-1080p Proper][DTS][DV HDR10][x264]-RlsGrp"},
		{"little known", bare, "Show (2019) - S12E03 - [HDTV-720p]"},
	}
	for _, id := range []string{"trash", "trash-plex-tmdb", "trash-emby-tmdb", "trash-jellyfin-tmdb"} {
		for _, tt := range eps {
			t.Run("episode "+id+" "+tt.name, func(t *testing.T) {
				if got := organizer.Render(scheme(t, id).Episode, tt.ctx); got != tt.want {
					t.Errorf("got  %q\nwant %q", got, tt.want)
				}
			})
		}
	}
}

// Adding the TRaSH schemes leaves the earlier ones as they were.
func TestEarlierSchemesUnchanged(t *testing.T) {
	want := map[string]string{
		"plex":          "{Movie Title} ({Year})",
		"plex-editions": "{Movie CleanTitle} ({Release Year}) {edition-{Edition Tags}} [{Quality Full}]{-Release Group}",
	}
	for id, movie := range want {
		if got := scheme(t, id).Movie; got != movie {
			t.Errorf("%s movie format changed: %q", id, got)
		}
	}
}
