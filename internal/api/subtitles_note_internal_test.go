package api

import "testing"

func TestSubtitleNoteText(t *testing.T) {
	tests := []struct {
		name      string
		have      []string
		missing   []string
		searching bool
		want      string
	}{
		{"all there", []string{"en", "nl"}, nil, false, "Already there (en, nl)"},
		{"all there, searching is moot", []string{"en"}, nil, true, "Already there (en)"},
		{"nothing wanted", nil, nil, false, ""},
		{"none yet, not searching", nil, []string{"en"}, false, "None yet, not downloaded"},
		{"none yet, searching", nil, []string{"en", "nl"}, true, "None yet, searching for en, nl"},
		{"some there, rest missing", []string{"en"}, []string{"nl"}, false, "Already there (en), nl still missing"},
		{"some there, searching for rest", []string{"en"}, []string{"nl"}, true, "Already there (en), searching for nl"},
		{"forced slot reads plainly", []string{"en"}, []string{"en.forced"}, true, "Already there (en), searching for en forced"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := subtitleNoteText(tc.have, tc.missing, tc.searching); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
