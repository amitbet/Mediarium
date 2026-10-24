package parser_test

import (
	"slices"
	"testing"

	"github.com/rdborg/mediarium/internal/parser"
)

func TestParseLanguages(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantLangs  []string
		wantMulti  bool
		wantSubbed bool
	}{
		{"no tag at all", "The.Matrix.1999.1080p.WEB-DL.x264-GROUP", nil, false, false},
		{"Italian and English", "Weapons.2025.iTA-ENG.Bluray.1080p.x264-CYBER", []string{"Italian", "English"}, false, false},
		{"Italian alone", "Weapons.2025.ITA.1080p.BluRay.x264-GRP", []string{"Italian"}, false, false},
		{"Italian spelled out", "Weapons.2025.ITALIAN.1080p.BluRay.x264-GRP", []string{"Italian"}, false, false},
		{"German", "Movie.2020.GERMAN.1080p.BluRay.x264-GRP", []string{"German"}, false, false},
		{"German short tag", "Movie.2020.GER.1080p.BluRay.x264-GRP", []string{"German"}, false, false},
		{"German dual language", "Movie.2020.German.DL.1080p.BluRay.x264-GRP", []string{"German"}, true, false},
		{"French", "Movie.2020.FRENCH.1080p.WEB-DL.x264-GRP", []string{"French"}, false, false},
		{"TrueFrench", "Movie.2020.TRUEFRENCH.1080p.WEB.x264-GRP", []string{"French"}, false, false},
		{"French short tag", "Movie.2020.FR.720p.WEB.x264-GRP", []string{"French"}, false, false},
		{"French dub tags", "Movie.2020.MULTi.VFF.1080p.WEB.x264-GRP", []string{"French"}, true, false},
		{"Spanish", "Movie.2020.SPANISH.1080p.BluRay.x264-GRP", []string{"Spanish"}, false, false},
		{"Spanish short tag", "Movie.2020.ESP.1080p.BluRay.x264-GRP", []string{"Spanish"}, false, false},
		{"Latino", "Movie.2020.LATINO.1080p.WEB-DL.x264-GRP", []string{"Spanish"}, false, false},
		{"Russian", "Movie.2020.RUS.1080p.BluRay.x264-GRP", []string{"Russian"}, false, false},
		{"Croatian spelled out", "Movie.2020.CROATIAN.1080p.BluRay.x264-GRP", []string{"Croatian"}, false, false},
		{"Croatian short tag", "Movie.2020.HRV.1080p.BluRay.x264-GRP", []string{"Croatian"}, false, false},
		{"Serbian spelled out", "Movie.2020.SERBIAN.1080p.BluRay.x264-GRP", []string{"Serbian"}, false, false},
		{"Serbian short tag", "Movie.2020.SRB.1080p.BluRay.x264-GRP", []string{"Serbian"}, false, false},
		{"Bosnian", "Movie.2020.BOSNIAN.1080p.BluRay.x264-GRP", []string{"Bosnian"}, false, false},
		{"Slovenian", "Movie.2020.SLOVENIAN.1080p.BluRay.x264-GRP", []string{"Slovenian"}, false, false},
		{"Bulgarian", "Movie.2020.BULGARIAN.1080p.BluRay.x264-GRP", []string{"Bulgarian"}, false, false},
		{"Bulgarian short tag", "Movie.2020.BUL.ENG.1080p.BluRay.x264-GRP", []string{"Bulgarian", "English"}, false, false},
		{"a language word in the title", "The.Serbian.Film.2010.1080p.BluRay.x264-GRP", nil, false, false},
		{"multi", "Movie.2020.MULTi.1080p.BluRay.x264-GRP", nil, true, false},
		{"dual audio", "Movie.2020.DUAL.1080p.BluRay.x264-GRP", nil, true, false},
		{"multi with the languages listed", "Movie.2020.MULTi.ITA.ENG.1080p.BluRay.x264-GRP", []string{"Italian", "English"}, true, false},
		{"English tag", "Movie.2020.ENG.1080p.BluRay.x264-GRP", []string{"English"}, false, false},
		{"English spelled out", "Movie.2020.ENGLISH.1080p.BluRay.x264-GRP", []string{"English"}, false, false},
		{"tags in lower case", "movie.2020.ita.eng.1080p.bluray.x264-grp", []string{"Italian", "English"}, false, false},
		{"tags with plus and brackets", "Movie (2020) [ITA+ENG] 1080p BluRay x264-GRP", []string{"Italian", "English"}, false, false},
		{"subtitles are not a language", "Movie.2020.VOSTFR.1080p.BluRay.x264-GRP", nil, false, true},
		{"subbed", "Movie.2020.SUBBED.1080p.BluRay.x264-GRP", nil, false, true},
		{"Korean with subs", "Movie.2020.KOREAN.SUBBED.1080p.BluRay.x264-GRP", []string{"Korean"}, false, true},
		{"the same language twice counts once", "Movie.2020.ITA.ITALIAN.1080p.BluRay.x264-GRP", []string{"Italian"}, false, false},
		{"WEB-DL is not dual language", "Movie.2020.GERMAN.WEB-DL.1080p.x264-GRP", []string{"German"}, false, false},
		{"DL on its own is not dual language", "Movie.2020.1080p.WEB-DL.x264-GRP", nil, false, false},
		{"a language in the title is part of the title", "The.Italian.Job.2003.1080p.BluRay.x264-GRP", nil, false, false},
		{"a language word in the title, tagged after it", "The.German.Doctor.2013.ITA.1080p.BluRay.x264-GRP", []string{"Italian"}, false, false},
		{"a film called Dual", "Dual.2022.1080p.WEB-DL.x264-GRP", nil, false, false},
		{"TV episode", "Show.Name.S01E02.ITA.ENG.720p.HDTV.x264-GRP", []string{"Italian", "English"}, false, false},
		{"TV German", "Show.Name.S01E02.GERMAN.720p.HDTV.x264-GRP", []string{"German"}, false, false},
		{"TV season pack multi", "Show.Name.S01.MULTi.1080p.WEB.h264-GRP", nil, true, false},
		{"episode title: French Toast", "Show.S03E04.French.Toast.1080p.WEB-DL.DDP5.1.H.264-GRP", nil, false, false},
		{"episode title: Dutch Oven", "Show.S03E04.The.Dutch.Oven.720p.HDTV.x264-GRP", nil, false, false},
		{"episode title: Polish Hill", "Show.S03E04.Polish.Hill.1080p.WEB-DL.x264-GRP", nil, false, false},
		{"episode title: Greek Week", "Show.S03E04.Greek.Week.1080p.AMZN.WEB-DL.x264-GRP", nil, false, false},
		{"episode title: Dual Loyalties", "Show.S03E04.Dual.Loyalties.1080p.WEB-DL.x264-GRP", nil, false, false},
		{"episode title, then a real tag", "Show.S03E04.French.Toast.GERMAN.1080p.WEB-DL.x264-GRP", []string{"German"}, false, false},
		{"episode title, real tag after the quality", "Show.S03E04.French.Toast.1080p.WEB-DL.GERMAN.x264-GRP", []string{"German"}, false, false},
		{"issue 91: DL right after German", "Phantastische.Tierwesen.und.wo.sie.zu.finden.sind.2016.German.DL.AC3.1080p.BluRay.x265-FuN", []string{"German"}, true, false},
		{"issue 91: DL after the source", "Phantastische.Tierwesen.und.wo.sie.zu.finden.sind.2016.German.BDRip.DL.1080p.x265.AAC-2BA", []string{"German"}, true, false},
		{"German.AC3.DL", "Movie.2020.German.AC3.DL.720p.BluRay.x264-GRP", []string{"German"}, true, false},
		{"DL before the language", "Movie.2020.DL.German.1080p.BluRay.x264-GRP", []string{"German"}, true, false},
		{"German.DTS.DL", "Movie.2020.German.DTS.DL.1080p.BluRay.x264-GRP", []string{"German"}, true, false},
		{"German dubbed is not dual", "Movie.2020.GERMAN.DUBBED.1080p.BluRay.x264-GRP", []string{"German"}, false, false},
		{"German.MULTi", "Movie.2020.German.MULTi.1080p.BluRay.x264-GRP", []string{"German"}, true, false},
		{"English.German.DL", "Movie.2020.English.German.DL.1080p.BluRay.x264-GRP", []string{"English", "German"}, true, false},
		{"German.DL.UHD.BluRay.TrueHD", "Movie.2020.German.DL.2160p.UHD.BluRay.TrueHD.x265-GRP", []string{"German"}, true, false},
		{"German.DL with DDP5.1", "Movie.2020.German.DL.1080p.DDP5.1.x264-GRP", []string{"German"}, true, false},
		{"German WEB-DL only is not dual", "Movie.2020.German.1080p.WEB-DL.x264-GRP", []string{"German"}, false, false},
		{"German WEB.DL only is not dual", "Movie.2020.German.1080p.AMZN.WEB.DL.DDP5.1.x264-GRP", []string{"German"}, false, false},
		{"German.DL.WEB-DL is dual", "Movie.2020.German.DL.1080p.WEB-DL.x264-GRP", []string{"German"}, true, false},
		{"UHD BluRay TrueHD without a language", "Movie.2020.2160p.UHD.BluRay.TrueHD.7.1.x265-GRP", nil, false, false},
		{"DL without a language is left alone", "Movie.2020.DL.1080p.BluRay.x264-GRP", nil, false, false},
		{"TV season pack German dual", "Show.S01.GERMAN.DL.720p.WEB.x264-GRP", []string{"German"}, true, false},
		{"TV episode multi after the title", "Show.S03E04.Pilot.MULTi.1080p.WEB.x264-GRP", nil, true, false},
		{"a word that only contains a tag", "Movie.2020.GERMANY.1080p.BluRay.x264-GRP", nil, false, false},
		{"no year, tag after the quality", "Movie.1080p.GERMAN.BluRay.x264-GRP", []string{"German"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parser.Parse(tc.in)
			if !slices.Equal(got.Languages, tc.wantLangs) {
				t.Errorf("Languages = %v, want %v", got.Languages, tc.wantLangs)
			}
			if got.Multi != tc.wantMulti {
				t.Errorf("Multi = %v, want %v", got.Multi, tc.wantMulti)
			}
			if got.Subbed != tc.wantSubbed {
				t.Errorf("Subbed = %v, want %v", got.Subbed, tc.wantSubbed)
			}
		})
	}
}

// A language tag never changes what the rest of the name parses to.
func TestLanguageTagsLeaveTheRestAlone(t *testing.T) {
	got := parser.Parse("Weapons.2025.iTA-ENG.Bluray.1080p.x264-CYBER")
	want := parser.Release{Title: "Weapons", Year: 2025, Resolution: "1080p", Source: "BluRay", Codec: "x264", Group: "CYBER"}
	assertReleaseEqual(t, want, got)
}

func TestLanguageNames(t *testing.T) {
	names := parser.LanguageNames()
	if names[0] != "English" {
		t.Fatalf("English should come first: %v", names)
	}
	for _, want := range []string{"Italian", "German", "French", "Spanish", "Russian", "Croatian", "Serbian", "Bosnian", "Slovenian", "Bulgarian"} {
		if !slices.Contains(names, want) {
			t.Errorf("%s is missing from %v", want, names)
		}
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("%s is listed twice", n)
		}
		seen[n] = true
	}
}
