package api_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
	"github.com/rdborg/mediarium/internal/library"
	"github.com/rdborg/mediarium/internal/settings"
)

// A tiny Matroska file: the headers a muxer writes, with the given tracks.
func tinyMKV(tracks ...[]byte) []byte {
	size := func(n int) []byte {
		for l := 1; l <= 8; l++ {
			if int64(n) < int64(1)<<(7*l)-1 {
				b := make([]byte, l)
				v := int64(n) | int64(1)<<(7*l)
				for i := l - 1; i >= 0; i-- {
					b[i] = byte(v)
					v >>= 8
				}
				return b
			}
		}
		return nil
	}
	el := func(id []byte, parts ...[]byte) []byte {
		body := bytes.Join(parts, nil)
		return append(append(append([]byte(nil), id...), size(len(body))...), body...)
	}
	header := el([]byte{0x1A, 0x45, 0xDF, 0xA3}, el([]byte{0x42, 0x82}, []byte("matroska")))
	tracksEl := el([]byte{0x16, 0x54, 0xAE, 0x6B}, tracks...)
	return append(header, el([]byte{0x18, 0x53, 0x80, 0x67}, tracksEl, el([]byte{0x1F, 0x43, 0xB6, 0x75}, []byte("frames")))...)
}

func mkvTrack(kind byte, codec, lang string, extra ...[]byte) []byte {
	var body bytes.Buffer
	put := func(b []byte) { body.Write(b) }
	put([]byte{0x83, 0x81, kind})
	put(append([]byte{0x86, 0x80 | byte(len(codec))}, codec...))
	put(append([]byte{0x22, 0xB5, 0x9C, 0x80 | byte(len(lang))}, lang...))
	for _, e := range extra {
		put(e)
	}
	n := body.Len()
	out := []byte{0xAE, 0x40 | byte(n>>8), byte(n)}
	return append(out, body.Bytes()...)
}

func u16be(v int) []byte { b := make([]byte, 2); binary.BigEndian.PutUint16(b, uint16(v)); return b }

func videoElement(w, h, transfer int) []byte {
	inner := append([]byte{0xB0, 0x82}, u16be(w)...)
	inner = append(inner, append([]byte{0xBA, 0x82}, u16be(h)...)...)
	if transfer > 0 {
		inner = append(inner, 0x55, 0xB0, 0x84, 0x55, 0xBA, 0x81, byte(transfer))
	}
	return append([]byte{0xE0, 0x80 | byte(len(inner))}, inner...)
}

func audioElement(channels int) []byte {
	return []byte{0xE1, 0x83, 0x9F, 0x81, byte(channels)}
}

func TestFileDetailsAreReadAndShown(t *testing.T) {
	restore := api.SetMediaInfoSettle(0)
	defer restore()
	e := newMCPEnv(t)
	e.enable(t)
	s := e.server
	movies := filepath.Join(t.TempDir(), "movies")
	if err := s.Settings.Set(settings.KeyMoviesPath, movies, false); err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"Night Harbour (2021)/Night.Harbour.2021.2160p.mkv": tinyMKV(
			mkvTrack(1, "V_MPEGH/ISO/HEVC", "eng", videoElement(3840, 2160, 16)),
			mkvTrack(2, "A_TRUEHD", "eng", audioElement(8)),
			mkvTrack(2, "A_AC3", "ger", audioElement(6)),
			mkvTrack(17, "S_HDMV/PGS", "dut")),
		"Tide Pool (2019)/Tide.Pool.2019.1080p.mkv": tinyMKV(
			mkvTrack(1, "V_MPEG4/ISO/AVC", "eng", videoElement(1920, 1080, 0)),
			mkvTrack(2, "A_AAC", "eng", audioElement(2))),
		"Old Film (1950)/Old.Film.1950.avi": []byte("this is an avi file that is not read for its streams"),
	}
	ids := map[string]library.Movie{}
	for i, name := range []string{"Night Harbour (2021)/Night.Harbour.2021.2160p.mkv", "Tide Pool (2019)/Tide.Pool.2019.1080p.mkv", "Old Film (1950)/Old.Film.1950.avi"} {
		path := filepath.Join(movies, filepath.FromSlash(name))
		writeBody(t, path, string(files[name]))
		m, err := s.MovieRepo.Add(library.Movie{TMDBID: 800 + i, Title: []string{"Night Harbour", "Tide Pool", "Old Film"}[i], Year: 2000 + i, Monitored: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.MovieRepo.SetStatus(m.ID, library.StatusDownloaded, "", path); err != nil {
			t.Fatal(err)
		}
		ids[name] = m
	}
	// A subtitle file beside a video counts like a track inside it.
	writeBody(t, filepath.Join(movies, "Tide Pool (2019)", "Tide.Pool.2019.1080p.de.srt"), "1 Hallo")
	night := ids["Night Harbour (2021)/Night.Harbour.2021.2160p.mkv"]
	tide := ids["Tide Pool (2019)/Tide.Pool.2019.1080p.mkv"]

	// Before the job runs, nothing is known and nothing is invented.
	status := getJSON[map[string]int](t, e.admin, e.base+"/api/library/file-details")
	if status["done"] != 0 || status["total"] != 3 {
		t.Fatalf("before: %v", status)
	}
	before := getJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d", e.base, night.ID))
	if before["fileInfo"] != nil {
		t.Fatalf("details before reading: %v", before["fileInfo"])
	}

	s.RunMediaInfoJob(context.Background())

	status = getJSON[map[string]int](t, e.admin, e.base+"/api/library/file-details")
	if status["done"] != 3 || status["total"] != 3 {
		t.Fatalf("after: %v", status)
	}

	// The movie page: the full details of the file.
	got := getJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d", e.base, night.ID))
	info, _ := got["fileInfo"].(map[string]any)
	video, _ := info["video"].(map[string]any)
	if info["container"] != "matroska" || info["resolution"] != "2160p" || video["codec"] != "HEVC" || video["hdr"] != "HDR10" {
		t.Fatalf("fileInfo = %v", info)
	}
	audio, _ := info["audio"].([]any)
	if len(audio) != 2 || audio[0].(map[string]any)["codec"] != "TrueHD" || audio[1].(map[string]any)["lang"] != "de" {
		t.Fatalf("audio = %v", audio)
	}

	// The Library list: short lists for the filters.
	list := getJSON[[]map[string]any](t, e.admin, e.base+"/api/movies?brief=1")
	media := map[float64]map[string]any{}
	for _, m := range list {
		if md, ok := m["media"].(map[string]any); ok {
			media[m["id"].(float64)] = md
		}
	}
	if len(media) != 3 { // the avi has a row too (container only), with no lists
		t.Logf("media: %v", media)
	}
	nm := media[float64(night.ID)]
	if fmt.Sprint(nm["audioLangs"]) != "[en de]" || fmt.Sprint(nm["audioCodecs"]) != "[TrueHD AC-3]" || fmt.Sprint(nm["subLangs"]) != "[nl]" || fmt.Sprint(nm["resolutions"]) != "[2160p]" {
		t.Errorf("night's media = %v", nm)
	}
	if tm := media[float64(tide.ID)]; fmt.Sprint(tm["hdr"]) != "[SDR]" || fmt.Sprint(tm["videoCodecs"]) != "[H.264]" || fmt.Sprint(tm["subLangs"]) != "[de]" {
		t.Errorf("tide's media = %v", tm)
	}

	// The Radarr style API.
	key := postJSON[map[string]any](t, e.admin, e.base+"/api/auth/api-keys", map[string]any{"name": "nzb360"}, http.StatusCreated)["key"].(string)
	files3 := list2(arrCall(t, "GET", fmt.Sprintf("%s/radarr/api/v3/moviefile?movieId=%d", e.base, night.ID), key, nil, http.StatusOK))
	mi := obj(obj(files3[0])["mediaInfo"])
	if mi["videoCodec"] != "h265" || mi["resolution"] != "3840x2160" || mi["audioCodec"] != "TrueHD" || mi["audioLanguages"] != "eng/ger" ||
		mi["videoDynamicRangeType"] != "HDR10" || mi["subtitles"] != "dut" || mi["audioChannels"] != 7.1 {
		t.Errorf("mediaInfo = %v", mi)
	}

	// The MCP search.
	count := func(args map[string]any) float64 {
		t.Helper()
		_, text, p := e.call(t, e.roKey, "library_search", args)
		if p == nil {
			t.Fatalf("answer: %s", text)
		}
		return p["total"].(float64)
	}
	tests := []struct {
		args map[string]any
		want float64
	}{
		{map[string]any{"audio_language": "German"}, 1},
		{map[string]any{"audio_language": "de"}, 1},
		{map[string]any{"audio_language": "english"}, 2},
		{map[string]any{"audio_language": "fr"}, 0},
		{map[string]any{"video_codec": "hevc"}, 1},
		{map[string]any{"video_codec": "x264"}, 1},
		{map[string]any{"video_codec": "h264", "audio_language": "de"}, 0},
		{map[string]any{"video_codec": "av1"}, 0},
		{map[string]any{"missing_subtitle_language": "Dutch"}, 2}, // the night has it inside; the tide and the avi do not
		{map[string]any{"missing_subtitle_language": "de"}, 2},    // the tide has a file beside it
		{map[string]any{"missing_subtitle_language": "fr"}, 3},    // none has French
		{map[string]any{"missing_subtitle_language": "nl", "audio_language": "english"}, 1},
	}
	for _, tt := range tests {
		if got := count(tt.args); got != tt.want {
			t.Errorf("%v: total %v, want %v", tt.args, got, tt.want)
		}
	}

	// A file that changed is read again by a rescan; one that did not is left alone.
	nightPath := filepath.Join(movies, "Night Harbour (2021)", "Night.Harbour.2021.2160p.mkv")
	before2, _, _ := s.MovieRepo.GetMediaInfo(nightPath)
	s.RunMediaInfoJob(context.Background())
	after2, _, _ := s.MovieRepo.GetMediaInfo(nightPath)
	if before2.ScannedAt != after2.ScannedAt || before2.MTime != after2.MTime {
		t.Errorf("an unchanged file was read again")
	}
	if err := os.WriteFile(nightPath, tinyMKV(mkvTrack(1, "V_AV1", "eng", videoElement(1920, 1080, 0)), mkvTrack(2, "A_OPUS", "jpn", audioElement(2))), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	_ = os.Chtimes(nightPath, later, later)
	postJSON[map[string]any](t, e.admin, fmt.Sprintf("%s/api/movies/%d/rescan", e.base, night.ID), nil, http.StatusOK)
	deadline := time.Now().Add(5 * time.Second)
	for {
		row, _, _ := s.MovieRepo.GetMediaInfo(nightPath)
		if row.VCodec == "AV1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the changed file was not read again: %+v", row)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A file that is gone from the library is forgotten.
	if err := s.MovieRepo.Delete(tide.ID); err != nil {
		t.Fatal(err)
	}
	s.RunMediaInfoJob(context.Background())
	if _, ok, _ := s.MovieRepo.GetMediaInfo(filepath.Join(movies, "Tide Pool (2019)", "Tide.Pool.2019.1080p.mkv")); ok {
		t.Error("the details of a removed movie's file should be forgotten")
	}
}

func list2(v any) []any { l, _ := v.([]any); return l }
