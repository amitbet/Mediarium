package api_test

import (
	"net/http"
	"testing"
)

// The seeding limits in Settings: empty is unlimited and is saved as empty, 0
// is do not seed and is saved as 0, a request that leaves a limit out leaves it
// alone, and bad text is refused.
func TestSeedLimitsInSettings(t *testing.T) {
	_, base, client := loginNewServer(t)
	url := base + "/api/settings"
	read := func() (ratio, hours any) {
		got := getJSON[map[string]any](t, client, url)
		return got["torrentSeedRatioLimit"], got["torrentSeedTimeLimitH"]
	}

	if r, h := read(); r != "" || h != "" {
		t.Fatalf("a fresh install is unlimited, got %v and %v", r, h)
	}
	tests := []struct {
		name         string
		body         map[string]any
		status       int
		ratio, hours any // what is read back afterwards
	}{
		{"numbers", map[string]any{"torrentSeedRatioLimit": "2.5", "torrentSeedTimeLimitH": "48"}, http.StatusOK, "2.5", "48"},
		{"leaving them out changes nothing", map[string]any{"torrentListenPort": "51413"}, http.StatusOK, "2.5", "48"},
		{"empty clears one limit", map[string]any{"torrentSeedRatioLimit": ""}, http.StatusOK, "", "48"},
		{"0 is kept as 0", map[string]any{"torrentSeedRatioLimit": "0", "torrentSeedTimeLimitH": "0"}, http.StatusOK, "0", "0"},
		{"spaces are trimmed", map[string]any{"torrentSeedTimeLimitH": "  12 "}, http.StatusOK, "0", "12"},
		{"empty clears both", map[string]any{"torrentSeedRatioLimit": "", "torrentSeedTimeLimitH": ""}, http.StatusOK, "", ""},
		{"a negative number is refused", map[string]any{"torrentSeedRatioLimit": "-1"}, http.StatusBadRequest, "", ""},
		{"text is refused", map[string]any{"torrentSeedTimeLimitH": "soon"}, http.StatusBadRequest, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			putJSON(t, client, url, tc.body, tc.status)
			if r, h := read(); r != tc.ratio || h != tc.hours {
				t.Fatalf("read back %q and %q, want %q and %q", r, h, tc.ratio, tc.hours)
			}
		})
	}
}
