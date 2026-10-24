package organizer_test

import (
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/organizer"
)

// Every shipped format, file name and folder, passes the check a person's own
// format has to pass, for movies and for shows.
func TestShippedFormatsAreValid(t *testing.T) {
	for _, s := range organizer.Schemes {
		if m := organizer.CheckFormat(s.Movie, organizer.TokenMovie); m != "" {
			t.Errorf("scheme %s movie: %s", s.ID, m)
		}
		if m := organizer.CheckFormat(s.Episode, organizer.TokenTV); m != "" {
			t.Errorf("scheme %s episode: %s", s.ID, m)
		}
		if len(s.Movie) > 500 || len(s.Episode) > 500 {
			t.Errorf("scheme %s is longer than the 500 characters a format may have", s.ID)
		}
	}
	for _, s := range organizer.FolderSchemes {
		if m := organizer.CheckFolderFormat(s.Movie, organizer.TokenMovie); m != "" {
			t.Errorf("folder scheme %s movie: %s", s.ID, m)
		}
		if m := organizer.CheckFolderFormat(s.Series, organizer.TokenTV); m != "" {
			t.Errorf("folder scheme %s series: %s", s.ID, m)
		}
	}
}

// The default folder scheme is exactly what has always been written.
func TestDefaultFolderScheme(t *testing.T) {
	d := organizer.FolderSchemes[0]
	if d.ID != "default" || d.Movie != organizer.Presets["plex"] || d.Series != organizer.TVSeriesFolder {
		t.Fatalf("default scheme = %+v", d)
	}
}

func TestFolderSchemesRender(t *testing.T) {
	movie := organizer.NamingContext{MovieTitle: "Tom & Jerry: The Movie", Year: 2021, TMDBID: 587807, Edition: "Extended", Quality: "1080p"}
	show := organizer.NamingContext{SeriesTitle: "The Show!", Year: 2019, TMDBID: 4242, Season: 1, Episode: 1}
	tests := []struct {
		id     string
		movie  string
		series string
	}{
		{"default", "Tom & Jerry: The Movie (2021)", "The Show! (2019)"},
		{"trash", "Tom and Jerry The Movie (2021)", "The Show! (2019)"},
		{"trash-plex-tmdb", "Tom and Jerry The Movie (2021) {tmdb-587807}", "The Show! (2019) {tmdb-4242}"},
		{"trash-emby-tmdb", "Tom and Jerry The Movie (2021) [tmdb-587807]", "The Show! (2019) [tmdb-4242]"},
		{"trash-jellyfin-tmdb", "Tom and Jerry The Movie (2021) [tmdbid-587807]", "The Show! (2019) [tmdbid-4242]"},
	}
	byID := map[string]organizer.FolderScheme{}
	for _, s := range organizer.FolderSchemes {
		byID[s.ID] = s
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			s, ok := byID[tt.id]
			if !ok {
				t.Fatalf("no folder scheme %q", tt.id)
			}
			if got := organizer.Render(s.Movie, movie); got != tt.movie {
				t.Errorf("movie: got %q, want %q", got, tt.movie)
			}
			if got := organizer.Render(s.Series, show); got != tt.series {
				t.Errorf("series: got %q, want %q", got, tt.series)
			}
		})
	}
}

func TestCheckFolderFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		kind   organizer.TokenKind
		want   string // part of the problem, "" when fine
	}{
		{"empty is fine", "", organizer.TokenMovie, ""},
		{"movie default", "{Movie Title} ({Year})", organizer.TokenMovie, ""},
		{"show with id", "{Series TitleYear} [tvdbid-{TvdbId}]", organizer.TokenTV, ""},
		{"slash", "{Movie Title}/{Year}", organizer.TokenMovie, "can't contain"},
		{"backslash", `{Series Title}\x`, organizer.TokenTV, "can't contain"},
		{"open brace", "{Movie Title ({Year})", organizer.TokenMovie, "{ or }"},
		{"unknown", "{Movie Title} {Made Up}", organizer.TokenMovie, "{Made Up}"},
		{"no title", "{Release Year}", organizer.TokenMovie, "movie's title"},
		{"show no title", "{Year}", organizer.TokenTV, "show's title"},
		{"season in a show folder", "{Series Title} S{Season:00}", organizer.TokenTV, "{Season}"},
		{"quality in a movie folder", "{Movie Title} [{Quality}]", organizer.TokenMovie, "{Quality}"},
		{"edition in a movie folder", "{Movie Title} {Edition Tags}", organizer.TokenMovie, "{Edition Tags}"},
		{"show token in a movie folder", "{Movie Title} {Series Title}", organizer.TokenMovie, "is for shows"},
		{"movie token in a show folder", "{Series Title} {Movie Title}", organizer.TokenTV, "is for movies"},
		{"bad padding", "{Movie Title} {Year:ab}", organizer.TokenMovie, "zeros"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := organizer.CheckFolderFormat(tt.format, tt.kind)
			if tt.want == "" && got != "" {
				t.Errorf("unexpected problem: %s", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("problem %q does not mention %q", got, tt.want)
			}
		})
	}
}
