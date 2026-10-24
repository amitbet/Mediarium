package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rdborg/mediarium/internal/mediainfo"
)

// arrMediaInfo is the "mediaInfo" of a movie or episode file in the Radarr and
// Sonarr style API, filled from what Mediarium read from the file's headers
// (media_info.go). Languages are the three-letter codes those apps show,
// separated by "/".
type arrMediaInfo struct {
	AudioChannels         float64 `json:"audioChannels"`
	AudioCodec            string  `json:"audioCodec"`
	AudioLanguages        string  `json:"audioLanguages"`
	AudioStreamCount      int     `json:"audioStreamCount"`
	VideoBitDepth         int     `json:"videoBitDepth"`
	VideoCodec            string  `json:"videoCodec"`
	VideoDynamicRange     string  `json:"videoDynamicRange"`
	VideoDynamicRangeType string  `json:"videoDynamicRangeType"`
	VideoFps              float64 `json:"videoFps"`
	Resolution            string  `json:"resolution"`
	RunTime               string  `json:"runTime"`
	Subtitles             string  `json:"subtitles"`
}

// arrMediaInfoFor reads the saved details of the file at path; nil when the
// file has not been read.
func (s *Server) arrMediaInfoFor(path string) *arrMediaInfo {
	if path == "" || s.MovieRepo == nil {
		return nil
	}
	row, ok, err := s.MovieRepo.GetMediaInfo(path)
	if err != nil || !ok || row.Failed || row.Info == "" {
		return nil
	}
	var info mediainfo.Info
	if json.Unmarshal([]byte(row.Info), &info) != nil {
		return nil
	}
	return toArrMediaInfo(info)
}

func toArrMediaInfo(info mediainfo.Info) *arrMediaInfo {
	out := &arrMediaInfo{AudioStreamCount: len(info.Audio)}
	if v := info.Video; v != nil {
		out.VideoCodec = arrVideoCodec(v.Codec)
		out.VideoBitDepth = v.BitDepth
		out.VideoFps = v.FPS
		if v.Width > 0 && v.Height > 0 {
			out.Resolution = fmt.Sprintf("%dx%d", v.Width, v.Height)
		}
		switch v.HDR {
		case "HDR10":
			out.VideoDynamicRange, out.VideoDynamicRangeType = "HDR", "HDR10"
		case "HDR10+":
			out.VideoDynamicRange, out.VideoDynamicRangeType = "HDR", "HDR10Plus"
		case "Dolby Vision":
			out.VideoDynamicRange, out.VideoDynamicRangeType = "HDR", "DolbyVision"
		case "HLG":
			out.VideoDynamicRange, out.VideoDynamicRangeType = "HDR", "HLG"
		}
	}
	if len(info.Audio) > 0 {
		main := info.Audio[0]
		for _, a := range info.Audio {
			if a.Default {
				main = a
				break
			}
		}
		out.AudioCodec = arrAudioCodec(main.Codec)
		if main.Channels >= 6 {
			out.AudioChannels = float64(main.Channels-1) + 0.1
		} else {
			out.AudioChannels = float64(main.Channels)
		}
	}
	out.AudioLanguages = arrLanguages(func(add func(string)) {
		for _, a := range info.Audio {
			add(a.Lang)
		}
	})
	out.Subtitles = arrLanguages(func(add func(string)) {
		for _, t := range info.Subtitles {
			add(t.Lang)
		}
	})
	if info.Duration > 0 {
		secs := int(info.Duration + 0.5)
		out.RunTime = fmt.Sprintf("%02d:%02d:%02d", secs/3600, secs/60%60, secs%60)
	}
	return out
}

// arrLanguages joins the different languages given, in order, as "eng/ger".
func arrLanguages(each func(add func(string))) string {
	var out []string
	seen := map[string]bool{}
	each(func(code string) {
		if code != "" && !seen[code] {
			seen[code] = true
			out = append(out, mediainfo.ISO3(code))
		}
	})
	return strings.Join(out, "/")
}

// arrVideoCodec names a codec the way Radarr and Sonarr do.
func arrVideoCodec(c string) string {
	switch c {
	case "H.264":
		return "h264"
	case "HEVC":
		return "h265"
	case "MPEG-2":
		return "MPEG2"
	case "MPEG-4":
		return "MPEG4"
	case "VC-1":
		return "VC1"
	}
	return c
}

// arrAudioCodec names a codec the way Radarr and Sonarr do.
func arrAudioCodec(c string) string {
	switch c {
	case "AC-3":
		return "AC3"
	case "E-AC-3":
		return "EAC3"
	}
	return c
}
