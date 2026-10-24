package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
)

func TestMarkBestMatch(t *testing.T) {
	hdr := quality.CustomFormat{ID: 1, Name: "HDR", App: quality.AppRadarr, Specs: []quality.FormatSpec{{Name: "HDR", Kind: quality.SpecReleaseTitle, Value: `\bHDR\b`}}}
	hdr.Compile()
	profile := quality.Presets()[quality.Preset1080p]
	profile.FormatScores = map[int64]int{1: 500}
	profile = quality.AttachFormats(profile, map[int64]*quality.CustomFormat{1: &hdr})

	tests := []struct {
		name       string
		titles     []string
		rejections map[int][]string
		blocked    map[int]bool
		want       int // index of the release that gets the label, -1 for none
	}{
		{"the higher score wins inside one quality", []string{"Heat.1995.1080p.WEB-DL.x264-GRP", "Heat.1995.1080p.WEB-DL.HDR.x264-GRP"}, nil, nil, 1},
		{"a better quality beats a higher score", []string{"Heat.1995.1080p.WEB-DL.HDR.x264-GRP", "Heat.1995.1080p.BluRay.x264-GRP"}, nil, nil, 1},
		{"a rejected release is never the best match", []string{"Heat.1995.1080p.WEB-DL.x264-GRP", "Heat.1995.1080p.WEB-DL.HDR.x264-GRP"}, map[int][]string{1: {"too big"}}, nil, 0},
		{"a blocklisted release is skipped", []string{"Heat.1995.1080p.WEB-DL.x264-GRP", "Heat.1995.1080p.WEB-DL.HDR.x264-GRP"}, nil, map[int]bool{1: true}, 0},
		{"nothing acceptable, nothing marked", []string{"Heat.1995.1080p.WEB-DL.x264-GRP"}, map[int][]string{0: {"no"}}, nil, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := results(tt.titles...)
			payloads := make([]searchResultPayload, len(found))
			for i := range payloads {
				payloads[i].Title = found[i].Title
				payloads[i].Rejections = tt.rejections[i]
				payloads[i].Blocklisted = tt.blocked[i]
			}
			markBestMatch(payloads, found, profile)
			got := -1
			for i, p := range payloads {
				if p.Best {
					if got != -1 {
						t.Fatal("more than one release marked")
					}
					got = i
				}
			}
			if got != tt.want {
				t.Fatalf("marked %d, want %d", got, tt.want)
			}
		})
	}
}
