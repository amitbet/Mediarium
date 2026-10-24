package mediainfo

// Kind says what a video file is meant to be, for judging its length.
type Kind int

const (
	KindMovie   Kind = iota
	KindEpisode      // an episode of a show
)

const (
	// shortFraction: a file shorter than this part of the expected length is
	// taken for a fake, a sample or a cut-off download.
	shortFraction = 0.5
	// shortestJudged: when the expected length is below this many minutes
	// (a short, a cartoon segment) nothing is judged by length at all.
	shortestJudged = 12.0
	// When the expected length is not known, only a file this short is
	// refused: no real episode or film is that short.
	unknownFloorEpisode = 3 * 60.0
	unknownFloorMovie   = 10 * 60.0
)

// TooShort says whether a video of seconds length is too short to be the
// episode or movie it claims to be. expectedMinutes is the runtime TMDB gives
// (0 when it is not known). A length of 0 or less means the file did not say
// how long it is, and is never judged.
func TooShort(seconds, expectedMinutes float64, kind Kind) bool {
	if !(seconds > 0) {
		return false
	}
	if expectedMinutes > 0 {
		if expectedMinutes < shortestJudged {
			return false
		}
		return seconds < expectedMinutes*60*shortFraction
	}
	if kind == KindEpisode {
		return seconds < unknownFloorEpisode
	}
	return seconds < unknownFloorMovie
}
