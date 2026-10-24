package quality

import (
	"reflect"
	"testing"
)

// A score of 0 is stored: the format is in the profile, doing nothing until
// someone raises it. Ids that are not real are still dropped.
func TestFormatScoresKeepZero(t *testing.T) {
	tests := []struct {
		name string
		in   map[int64]int
		want map[int64]int
	}{
		{"zero is kept", map[int64]int{1: 0, 2: 50}, map[int64]int{1: 0, 2: 50}},
		{"negative is kept", map[int64]int{3: -10000}, map[int64]int{3: -10000}},
		{"bad ids are dropped", map[int64]int{0: 5, -1: 0, 4: 0}, map[int64]int{4: 0}},
		{"empty", map[int64]int{}, map[int64]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeFormatScores(encodeFormatScores(tt.in))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("round trip = %v, want %v", got, tt.want)
			}
		})
	}
}

// A 0 entry changes no release's score.
func TestZeroEntryDoesNotChangeScore(t *testing.T) {
	p := trashProfile(t)
	title := "Night.Harbour.2020.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRP"
	before := p.Score(title)
	withZero := p
	withZero.FormatScores = map[int64]int{}
	for id, sc := range p.FormatScores {
		withZero.FormatScores[id] = sc
	}
	withZero.FormatScores[99] = 0
	if got := withZero.Score(title); got != before {
		t.Errorf("score with a 0 entry = %d, want %d", got, before)
	}
}
