package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/parser"
)

// A release that puts the maker's name in front of the title
// ("Tim.Burtons.The.Nightmare...") is the movie; other extra words are not.
func TestMatchesMoviePossessiveLead(t *testing.T) {
	cases := []struct {
		movie   library.Movie
		release string
		want    bool
	}{
		{library.Movie{Title: "The Nightmare Before Christmas", Year: 1993}, "Tim.Burtons.The.Nightmare.Before.Christmas.1993.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "The Nightmare Before Christmas", Year: 1993}, "Tim.Burtons.Nightmare.Before.Christmas.1993.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "The Nightmare Before Christmas", Year: 1993}, "The.Nightmare.Before.Christmas.1993.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "The Nightmare Before Christmas", Year: 1993}, "Tim.Burtons.The.Nightmare.Before.Christmas.1080p.BluRay.x264-GRP", false}, // no year
		{library.Movie{Title: "The Nightmare Before Christmas", Year: 1993}, "Tim.Burtons.The.Nightmare.Before.Christmas.2006.1080p.BluRay.x264-GRP", false},
		{library.Movie{Title: "Frozen", Year: 2013}, "Disneys.Frozen.2013.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "Labyrinth", Year: 1986}, "Jim.Hensons.Labyrinth.1986.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "Labyrinth", Year: 1986}, "Jim.Hensons.Classic.Labyrinth.1986.1080p.BluRay.x264-GRP", false}, // "Classic" is next to the title
		{library.Movie{Title: "Labyrinth", Year: 1986}, "Jim.Henson.Labyrinth.1986.1080p.BluRay.x264-GRP", false},          // no possessive
		{library.Movie{Title: "Avengers", Year: 2012}, "Marvels.Avengers.2012.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "Bad Boys", Year: 1995}, "Bad.Boys.for.Life.1995.1080p.BluRay.x264-GRP", false},
		{library.Movie{Title: "Bad Boys", Year: 1995}, "Star.Wars.Bad.Boys.1995.1080p.BluRay.x264-GRP", false},
		{library.Movie{Title: "Bad Boys", Year: 1995}, "Bad.Boys.1995.1080p.BluRay.x264-GRP", true},
		{library.Movie{Title: "Alien", Year: 1979}, "This.Alien.1979.1080p.BluRay.x264-GRP", false}, // a plain word, not a maker
		{library.Movie{Title: "Alien", Year: 1979}, "The.Boss.Alien.1979.1080p.BluRay.x264-GRP", false},
		{library.Movie{Title: "Alien", Year: 1979}, "Old.Mr.Big.Ridleys.Alien.1979.1080p.BluRay.x264-GRP", false},   // four extra words
		{library.Movie{Title: "Alien", Year: 0}, "Ridleys.Alien.1979.1080p.BluRay.x264-GRP", false},                 // the movie has no year: the new rule needs one
		{library.Movie{Title: "Alien", Year: 1979}, "Ridleys.Alien.Resurrection.1979.1080p.BluRay.x264-GRP", false}, // more after the title
	}
	for _, tc := range cases {
		t.Run(tc.release, func(t *testing.T) {
			if got := matchesMovie(tc.release, tc.movie); got != tc.want {
				t.Errorf("matchesMovie(%q, %q %d) = %v, want %v (parsed title %q)", tc.release, tc.movie.Title, tc.movie.Year, got, tc.want, parser.Parse(tc.release).Title)
			}
		})
	}
}
