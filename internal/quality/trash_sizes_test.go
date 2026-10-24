package quality

import (
	"reflect"
	"testing"
)

func TestParseTrashSizes(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    map[Tier]SizeLimit
		skipped []string
		wantErr bool
	}{
		{
			name: "the slider top is no limit, the preferred size is ignored",
			in:   `{"qualities":[{"quality":"WEBDL-1080p","min":12.5,"preferred":1999,"max":2000},{"quality":"Bluray-1080p","min":50.8,"preferred":100,"max":155}]}`,
			want: map[Tier]SizeLimit{TierWebDL1080p: {Min: 12.5, Max: 0}, TierBluray1080p: {Min: 50.8, Max: 155}},
		},
		{
			name: "WEBRip joins WEBDL, and WEBDL wins when both are there",
			in:   `{"qualities":[{"quality":"WEBRip-720p","min":9,"max":50},{"quality":"WEBDL-720p","min":10,"max":60},{"quality":"WEBRip-2160p","min":30,"max":1000}]}`,
			want: map[Tier]SizeLimit{TierWebDL720p: {Min: 10, Max: 60}, TierWebDL2160p: {Min: 30, Max: 0}},
		},
		{
			name: "Sonarr's remux names",
			in:   `{"qualities":[{"quality":"Bluray-1080p Remux","min":69.1,"max":1000},{"quality":"Bluray-2160p Remux","min":187.4,"max":1000}]}`,
			want: map[Tier]SizeLimit{TierRemux1080p: {Min: 69.1}, TierRemux2160p: {Min: 187.4}},
		},
		{
			name:    "qualities Mediarium has no row for are named",
			in:      `{"qualities":[{"quality":"Bluray-576p","min":5,"max":1000},{"quality":"Bluray-480p","min":5,"max":1000},{"quality":"DVD","min":5,"max":1000},{"quality":"DVD-R","min":6,"max":1000}]}`,
			want:    map[Tier]SizeLimit{TierDVD: {Min: 5}},
			skipped: []string{"Bluray-480p", "Bluray-576p"},
		},
		{
			name: "a largest size below the smallest is raised to it",
			in:   `{"qualities":[{"quality":"SDTV","min":10,"max":4}]}`,
			want: map[Tier]SizeLimit{TierSDTV: {Min: 10, Max: 10}},
		},
		{name: "not JSON", in: `nope`, wantErr: true},
		{name: "no qualities", in: `{"qualities":[]}`, wantErr: true},
		{name: "only unknown qualities", in: `{"qualities":[{"quality":"Raw-HD","min":1,"max":2}]}`, wantErr: true},
		{name: "negative size", in: `{"qualities":[{"quality":"SDTV","min":-1,"max":2}]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTrashSizes([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got.Limits, tt.want) {
				t.Errorf("limits = %v, want %v", got.Limits, tt.want)
			}
			if !reflect.DeepEqual(got.Skipped, tt.skipped) {
				t.Errorf("skipped = %v, want %v", got.Skipped, tt.skipped)
			}
			table := SizeLimits{Limits: got.Limits}
			if err := table.Validate(); err != nil {
				t.Errorf("the result isn't a valid size table: %v", err)
			}
		})
	}
}
