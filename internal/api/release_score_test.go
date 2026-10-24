package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
)

// The Score column shows the score the pick uses (Profile.Score). It has to
// equal the parts the details row lists: the custom formats, the preferred
// terms and the bonus of a fixed release.
func TestScoreColumnEqualsTheSumOfItsParts(t *testing.T) {
	hdr := quality.CustomFormat{ID: 1, Name: "HDR", App: quality.AppRadarr, Specs: []quality.FormatSpec{{Name: "HDR", Kind: quality.SpecReleaseTitle, Value: `\bHDR\b`}}}
	hdr.Compile()
	base := quality.Presets()[quality.Preset1080p]
	base.FormatScores = map[int64]int{1: 500}
	base = quality.AttachFormats(base, map[int64]*quality.CustomFormat{1: &hdr})
	base.Preferred = []quality.Preferred{{Term: "x265", Score: 50}, {Term: "GRP", Score: -20}, {Term: "nomatch", Score: 999}}

	withFix := base
	withFix.SkipBadReleases = true
	termsOnly := quality.Presets()[quality.Preset1080p]
	termsOnly.Preferred = []quality.Preferred{{Term: "x265", Score: 50}}

	tests := []struct {
		name    string
		profile quality.Profile
		title   string
		want    int
		fix     int
	}{
		{"a format and two terms", base, "Heat.1995.1080p.WEB-DL.HDR.x265-GRP", 500 + 50 - 20, 0},
		{"a format only", base, "Heat.1995.1080p.WEB-DL.HDR.x264-OTHER", 500, 0},
		{"terms only", base, "Heat.1995.1080p.WEB-DL.x265-GRP", 50 - 20, 0},
		{"nothing matches", base, "Heat.1995.1080p.WEB-DL.x264-OTHER", 0, 0},
		{"a fixed release adds its bonus", withFix, "Heat.1995.1080p.WEB-DL.HDR.x265.REPACK-GRP", 500 + 50 - 20 + 5, 5},
		{"a profile with terms and no formats", termsOnly, "Heat.1995.1080p.WEB-DL.x265-GRP", 50, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var p searchResultPayload
			p.setPreferredHits(tc.profile, tc.title)
			setFormatScore(&p, tc.profile, tc.title)
			if p.Score == nil {
				t.Fatal("the profile scores something, so the release has a score")
			}
			parts := p.FixScore
			if p.FormatScore != nil {
				parts += *p.FormatScore
			}
			for _, x := range p.Terms {
				parts += x.Score
			}
			if *p.Score != parts || *p.Score != tc.want || *p.Score != tc.profile.Score(tc.title) || p.FixScore != tc.fix {
				t.Fatalf("column %d, parts %d (formats %v, terms %+v, fix %d), want %d, pick uses %d", *p.Score, parts, p.FormatScore, p.Terms, p.FixScore, tc.want, tc.profile.Score(tc.title))
			}
		})
	}

	// A profile that scores nothing has no Score column.
	var p searchResultPayload
	plain := quality.Presets()[quality.Preset1080p]
	p.setPreferredHits(plain, "Heat.1995.1080p.WEB-DL.x265-GRP")
	if p.Score != nil {
		t.Fatalf("score %d for a profile with no formats or terms", *p.Score)
	}
}
