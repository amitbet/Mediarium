package api

import (
	"strings"
	"testing"

	"github.com/rdborg/mediarium/internal/quality"
)

func TestStateReasons(t *testing.T) {
	tests := []struct {
		name  string
		in    searchResultPayload
		kinds []string
	}{
		{"nothing wrong", searchResultPayload{}, nil},
		{"blocklisted", searchResultPayload{Blocklisted: true}, []string{"blocklist"}},
		{"few seeders", searchResultPayload{BelowMinSeeders: true, MinSeeders: 5, Seeders: 1}, []string{"seeders"}},
		{"other language", searchResultPayload{LanguageFit: "other"}, []string{"language"}},
		{"mixed language is fine", searchResultPayload{LanguageFit: "mixed"}, nil},
		{"all three", searchResultPayload{Blocklisted: true, BelowMinSeeders: true, MinSeeders: 3, LanguageFit: "other"}, []string{"blocklist", "seeders", "language"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.in
			p.addStateReasons()
			var got []string
			for _, r := range p.Reasons {
				got = append(got, r.Kind)
				if strings.TrimSpace(r.Text) == "" || strings.Contains(r.Text, "%") {
					t.Errorf("reason %q has no proper text: %q", r.Kind, r.Text)
				}
			}
			if strings.Join(got, ",") != strings.Join(tt.kinds, ",") {
				t.Fatalf("kinds = %v, want %v", got, tt.kinds)
			}
			if len(p.Rejections) != 0 {
				t.Fatalf("state reasons must not change the rejections that pick the best match: %v", p.Rejections)
			}
		})
	}
}

func TestAddReasonsKeepsRejections(t *testing.T) {
	var p searchResultPayload
	p.addReasons("size", []string{"smaller than a real 1080p WEB-DL release is likely to be"})
	if len(p.Rejections) != 1 || len(p.Reasons) != 1 || p.Reasons[0].Kind != "size" || p.Reasons[0].Text != p.Rejections[0] {
		t.Fatalf("rejections %v reasons %+v", p.Rejections, p.Reasons)
	}
}

func TestRejectionReasonKinds(t *testing.T) {
	_, profile := profileChain(true, quality.PresetCinema)
	got := rejectionReasonsFor("Film.2026.1080p.TeleSync.x264-GRP", profile, false, "", "")
	if len(got) == 0 || got[0].Kind != "quality" {
		t.Fatalf("a quality the profile does not allow should come first as quality: %+v", got)
	}
	texts := rejectionsFor("Film.2026.1080p.TeleSync.x264-GRP", profile, false, "", "")
	if len(texts) != len(got) {
		t.Fatalf("the texts and the reasons must agree: %v vs %+v", texts, got)
	}
}

func TestSetFormatScoreBreakdown(t *testing.T) {
	hdr := quality.CustomFormat{ID: 1, Name: "HDR", App: quality.AppRadarr, Specs: []quality.FormatSpec{{Name: "HDR", Kind: quality.SpecReleaseTitle, Value: `\bHDR\b`}}}
	hdr.Compile()
	profile := quality.Presets()[quality.Preset1080p]
	profile.FormatScores = map[int64]int{1: 500}
	profile.MinFormatScore = 100
	profile.CutoffFormatScore = 800
	profile = quality.AttachFormats(profile, map[int64]*quality.CustomFormat{1: &hdr})

	var p searchResultPayload
	setFormatScore(&p, profile, "Heat.1995.1080p.WEB-DL.HDR.x264-GRP")
	if p.FormatScore == nil || *p.FormatScore != 500 || len(p.Formats) != 1 || p.Formats[0].Name != "HDR" || p.Formats[0].Score != 500 {
		t.Fatalf("score %v formats %+v", p.FormatScore, p.Formats)
	}
	if p.MinScore != 100 || p.UpgradeUntil != 800 {
		t.Fatalf("min %d upgrade until %d", p.MinScore, p.UpgradeUntil)
	}
}
