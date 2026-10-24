package migrate

import (
	"encoding/json"
	"testing"
)

func TestProwlarrMinimumSeeders(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   int
	}{
		{"no such field", `[{"name":"baseUrl","value":"https://a"}]`, 0},
		{"minimumSeeders", `[{"name":"minimumSeeders","value":5}]`, 5},
		{"as text", `[{"name":"minimumSeeders","value":"12"}]`, 12},
		{"the torrent settings name", `[{"name":"torrentBaseSettings.appMinimumSeeders","value":3}]`, 3},
		{"zero", `[{"name":"minimumSeeders","value":0}]`, 0},
		{"null", `[{"name":"minimumSeeders","value":null}]`, 0},
		{"negative", `[{"name":"minimumSeeders","value":-4}]`, 0},
		{"nonsense", `[{"name":"minimumSeeders","value":"many"}]`, 0},
		{"held at the largest", `[{"name":"minimumSeeders","value":99999}]`, 1000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var x prowlarrIndexer
			if err := json.Unmarshal([]byte(`{"name":"X","fields":`+tc.fields+`}`), &x); err != nil {
				t.Fatal(err)
			}
			if got := x.minSeeders(); got != tc.want {
				t.Errorf("minSeeders = %d, want %d", got, tc.want)
			}
		})
	}
}
