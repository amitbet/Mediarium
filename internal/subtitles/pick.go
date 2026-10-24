package subtitles

import (
	"regexp"
	"strings"

	"github.com/rdborg/mediarium/internal/parser"
)

func normalizeRelease(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// HashMatchScore is what being made for the exact video file adds to Score.
const HashMatchScore = 500

func hasToken(normalizedRelease, token string) bool {
	token = normalizeRelease(token)
	return token != "" && strings.Contains(normalizedRelease, token)
}

// Score rates how well a subtitle result fits a video file: one made for the
// exact file (a hash match) beats everything; a subtitle timed against the
// same release (matching group, source, resolution, codec) is
// far more likely to be in sync than one for a different rip, so those
// matches dominate; rating and popularity break ties.
func Score(r Result, videoName string) float64 {
	return ScoreFor(r, videoName, VideoHints{})
}

// VideoHints is what else is known about the video file besides its name. A
// library file is named by Mediarium ("The Sixth Sense (1999).mp4"), so the
// name says nothing about the release; these fill that gap.
type VideoHints struct {
	// ReleaseTitle is the title of the release the file came from, as it was
	// recorded when it was downloaded ("Movie 1999 1080p BrRip x264 - YIFY").
	ReleaseTitle string
	// Quality is the quality stored for the file ("Bluray-1080p").
	Quality string
}

var spacedGroupRe = regexp.MustCompile(`\s+-\s+([A-Za-z0-9]+)$`)

// videos are the descriptions of the video a subtitle is compared with: the
// file name, the release it was downloaded as, and its stored quality.
func (h VideoHints) videos(videoName string) []parser.Release {
	out := []parser.Release{parser.Parse(videoName)}
	if strings.TrimSpace(h.ReleaseTitle) != "" {
		// "Movie 1080p x264 - YIFY" names its group with spaces around the dash;
		// the parser wants "-YIFY".
		out = append(out, parser.Parse(spacedGroupRe.ReplaceAllString(strings.TrimSpace(h.ReleaseTitle), "-$1")))
	}
	if q := strings.TrimSpace(h.Quality); q != "" {
		// Only the source and the resolution are known from a stored quality.
		p := parser.Parse(q)
		out = append(out, parser.Release{Source: p.Source, Resolution: p.Resolution})
	}
	return out
}

// ScoreFor is Score with the extra facts about the video: the subtitle is
// compared with each description of the video and the best match counts.
func ScoreFor(r Result, videoName string, hints VideoHints) float64 {
	score := 0.0
	for _, video := range hints.videos(videoName) {
		score = max(score, releaseScore(r.Release, video))
		for _, other := range r.Releases {
			score = max(score, releaseScore(other, video))
		}
	}
	if r.HashMatch {
		score += HashMatchScore
	}
	dl := float64(r.DownloadsAll) / 1000
	if dl > 40 {
		dl = 40
	}
	if r.HearingImpaired {
		// Kept, but under an equal plain subtitle: the sound descriptions are
		// noise for most people.
		score -= hearingImpairedPenalty
	}
	return score + dl + r.Rating*2
}

// hearingImpairedPenalty puts a hearing-impaired subtitle below a plain one that fits equally well.
const hearingImpairedPenalty = 5

// releaseScore is how closely one release name matches the video's.
func releaseScore(release string, video parser.Release) float64 {
	rel := normalizeRelease(release)
	score := 0.0
	if hasToken(rel, video.Group) {
		score += 100
	}
	if hasToken(rel, video.Source) {
		score += 30
	}
	if hasToken(rel, video.Resolution) {
		score += 30
	}
	if hasToken(rel, video.Codec) {
		score += 20
	}
	return score
}

// Pick returns the best-fitting result for videoName, or nil if there are
// none.
func Pick(results []Result, videoName string) *Result {
	return PickFor(results, videoName, VideoHints{})
}

// PickFor is Pick with the extra facts about the video (see ScoreFor).
func PickFor(results []Result, videoName string, hints VideoHints) *Result {
	var best *Result
	bestScore := 0.0
	for i := range results {
		if s := ScoreFor(results[i], videoName, hints); best == nil || s > bestScore {
			bestScore = s
			best = &results[i]
		}
	}
	return best
}
