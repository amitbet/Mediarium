package mediainfo

import "testing"

func TestTooShort(t *testing.T) {
	tests := []struct {
		name     string
		seconds  float64
		expected float64 // minutes, 0 = unknown
		kind     Kind
		want     bool
	}{
		{"fake episode of a 45 minute show", 60, 45, KindEpisode, true},
		{"real episode", 44 * 60, 45, KindEpisode, false},
		{"just under half", 22.4 * 60, 45, KindEpisode, true},
		{"exactly half", 22.5 * 60, 45, KindEpisode, false},
		{"just over half", 23 * 60, 45, KindEpisode, false},
		{"49 percent", 49 * 60, 100, KindMovie, true},
		{"50 percent", 50 * 60, 100, KindMovie, false},
		{"51 percent", 51 * 60, 100, KindMovie, false},
		{"short show is never judged", 30, 11, KindEpisode, false},
		{"at the lower limit", 5 * 60, 12, KindEpisode, true},
		{"unknown runtime, tiny episode", 90, 0, KindEpisode, true},
		{"unknown runtime, ordinary episode", 20 * 60, 0, KindEpisode, false},
		{"unknown runtime, episode just over three minutes", 181, 0, KindEpisode, false},
		{"unknown runtime, tiny movie", 5 * 60, 0, KindMovie, true},
		{"unknown runtime, movie over ten minutes", 11 * 60, 0, KindMovie, false},
		{"unknown runtime, a four minute movie", 4 * 60, 0, KindMovie, true},
		{"no duration in the file", 0, 45, KindEpisode, false},
		{"negative duration", -5, 45, KindMovie, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TooShort(tt.seconds, tt.expected, tt.kind); got != tt.want {
				t.Errorf("TooShort(%v, %v, %v) = %v, want %v", tt.seconds, tt.expected, tt.kind, got, tt.want)
			}
		})
	}
}
