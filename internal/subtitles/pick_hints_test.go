package subtitles_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/subtitles"
)

func TestScoreForUsesTheDownloadedRelease(t *testing.T) {
	const movie = "The Sixth Sense (1999).mp4"
	const movieRel = "The Sixth Sense (1999) 1080p BrRip x264 - YIFY"
	const episode = "Show - S01E02 - Pilot.mkv"
	const episodeRel = "Show.S01E02.720p.WEB-DL.x264-NTb"
	tests := []struct {
		name    string
		video   string
		hints   subtitles.VideoHints
		release string
		wantPos bool
	}{
		{"tags only in the release title", movie, subtitles.VideoHints{ReleaseTitle: movieRel}, "The.Sixth.Sense.1999.1080p.BrRip.x264.YIFY", true},
		{"unrelated subtitle stays at zero", movie, subtitles.VideoHints{ReleaseTitle: movieRel}, "Sixth Sense (1999) DVD.US.Retail", false},
		{"no release title and no quality", movie, subtitles.VideoHints{}, "The.Sixth.Sense.1999.1080p.BrRip.x264.YIFY", false},
		{"episode", episode, subtitles.VideoHints{ReleaseTitle: episodeRel}, "Show.S01E02.720p.WEB-DL.x264-NTb", true},
		{"stored quality alone", movie, subtitles.VideoHints{Quality: "Bluray-1080p"}, "Movie 1999 1080p BluRay", true},
		{"stored quality does not match", movie, subtitles.VideoHints{Quality: "Bluray-1080p"}, "Movie 1999 720p HDTV", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subtitles.ScoreFor(subtitles.Result{Release: tt.release}, tt.video, tt.hints)
			if (got > 0) != tt.wantPos {
				t.Fatalf("score = %v, want positive: %v", got, tt.wantPos)
			}
		})
	}
}

func TestPickForRanksTheMatchingGroupFirst(t *testing.T) {
	results := []subtitles.Result{
		{FileID: 1, Release: "The Sixth Sense 1999 1080p BrRip x264 OTHER", DownloadsAll: 5000},
		{FileID: 2, Release: "The Sixth Sense 1999 BrRip x264 YIFY"},
	}
	best := subtitles.PickFor(results, "The Sixth Sense (1999).mp4", subtitles.VideoHints{ReleaseTitle: "The Sixth Sense (1999) 1080p BrRip x264 - YIFY"})
	if best == nil || best.FileID != 2 {
		t.Fatalf("best = %+v, want the YIFY subtitle", best)
	}
}
