package mediainfo

import (
	"encoding/binary"
	"math"
	"strings"
)

// MP4, M4V and MOV: the boxes from moov down to each track's sample
// description. Only small boxes are read (never the sample tables), the walk
// follows the fixed layout rather than recursing, and the number of boxes
// visited and bytes read are capped.

const (
	maxBoxes     = 20_000
	smallBox     = 64 << 10 // a box that holds a setup record
	sttsEntries  = 64
	mp4FrameRead = 1 << 20
)

type mp4Box struct {
	typ        string
	start, end int64 // the contents
}

func be16(b []byte) int { return int(binary.BigEndian.Uint16(b)) }
func be32(b []byte) int { return int(binary.BigEndian.Uint32(b)) }

// mp4Boxes lists the boxes between start and end.
func (r *reader) mp4Boxes(start, end int64, visited *int) []mp4Box {
	var out []mp4Box
	if end > r.size {
		end = r.size
	}
	for pos := start; pos+8 <= end; {
		*visited++
		if *visited > maxBoxes {
			break
		}
		hdr, err := r.at(pos, 16)
		if err != nil || len(hdr) < 8 {
			break
		}
		size := int64(binary.BigEndian.Uint32(hdr[:4]))
		head := int64(8)
		switch size {
		case 1:
			if len(hdr) < 16 {
				return out
			}
			u := binary.BigEndian.Uint64(hdr[8:16])
			if u > math.MaxInt64/2 {
				return out
			}
			size, head = int64(u), 16
		case 0:
			size = end - pos
		}
		if size < head || pos+size > end {
			break
		}
		out = append(out, mp4Box{typ: string(hdr[4:8]), start: pos + head, end: pos + size})
		pos += size
	}
	return out
}

// data reads a box's contents when they are small enough.
func (r *reader) data(b mp4Box, limit int64) []byte {
	if b.end-b.start > limit {
		return nil
	}
	d, err := r.at(b.start, b.end-b.start)
	if err != nil {
		return nil
	}
	return d
}

// boxChildren walks the boxes inside a buffer.
func boxChildren(buf []byte, fn func(typ string, data []byte)) {
	for len(buf) >= 8 {
		size := int64(binary.BigEndian.Uint32(buf[:4]))
		head := int64(8)
		switch size {
		case 1:
			if len(buf) < 16 {
				return
			}
			size = int64(binary.BigEndian.Uint64(buf[8:16]))
			head = 16
		case 0:
			size = int64(len(buf))
		}
		if size < head || size > int64(len(buf)) {
			return
		}
		fn(string(buf[4:8]), buf[head:size])
		buf = buf[size:]
	}
}

func readMP4(r *reader) (Info, error) {
	info := Info{Container: "mp4"}
	visited := 0
	var moov *mp4Box
	for _, b := range r.mp4Boxes(0, r.size, &visited) {
		b := b
		switch b.typ {
		case "ftyp":
			if d := r.data(b, 1024); len(d) >= 4 && string(d[:4]) == "qt  " {
				info.Container = "mov"
			}
		case "moov":
			if moov == nil {
				moov = &b
			}
		}
	}
	if moov == nil {
		return info, ErrUnreadable
	}
	tracks := 0
	var plus bool
	var frameAt int64 = -1
	for _, b := range r.mp4Boxes(moov.start, moov.end, &visited) {
		switch b.typ {
		case "mvhd":
			if d := r.data(b, 512); len(d) >= 20 {
				scale, dur := be32(d[12:]), int64(be32(d[16:]))
				if d[0] == 1 && len(d) >= 32 {
					scale, dur = be32(d[20:]), int64(binary.BigEndian.Uint64(d[24:]))
				}
				if scale > 0 && dur > 0 {
					info.Duration = math.Round(float64(dur)/float64(scale)*10) / 10
				}
			}
		case "trak":
			if tracks >= maxTracks {
				continue
			}
			tracks++
			t := r.mp4Track(b, &visited)
			switch t.handler {
			case "vide":
				if info.Video != nil || t.video.Codec == "" {
					continue
				}
				v := t.video
				if t.width > 0 && v.Width == 0 {
					v.Width, v.Height = t.width, t.height
				}
				info.Video = &v
				plus = v.HDR == "HDR10" && v.Codec == "HEVC"
				frameAt = t.firstChunk
			case "soun":
				if t.audio.Codec != "" {
					info.Audio = append(info.Audio, t.audio)
				}
			case "sbtl", "subt":
				info.Subtitles = append(info.Subtitles, t.subtitle)
			}
		}
	}
	if plus && frameAt > 0 {
		if frames, err := r.at(frameAt, mp4FrameRead); err == nil && hasHDR10Plus(frames) {
			info.Video.HDR = "HDR10+"
		}
	}
	return info, nil
}

type mp4TrackInfo struct {
	handler       string
	width, height int // from the track header
	video         Video
	audio         Audio
	subtitle      Subtitle
	firstChunk    int64
	delta         float64 // the usual frame length, in the track's time scale
}

func (r *reader) mp4Track(trak mp4Box, visited *int) mp4TrackInfo {
	var t mp4TrackInfo
	var timescale int
	var lang string
	for _, b := range r.mp4Boxes(trak.start, trak.end, visited) {
		switch b.typ {
		case "tkhd":
			if d := r.data(b, 256); len(d) >= 84 {
				t.width, t.height = be16(d[len(d)-8:]), be16(d[len(d)-4:])
			}
		case "mdia":
			for _, m := range r.mp4Boxes(b.start, b.end, visited) {
				switch m.typ {
				case "hdlr":
					if d := r.data(m, 512); len(d) >= 12 {
						t.handler = string(d[8:12])
					}
				case "mdhd":
					if d := r.data(m, 64); len(d) >= 4 {
						timescale, lang = mdhdFields(d)
					}
				case "minf":
					r.mp4Sample(m, &t, visited)
				}
			}
		}
	}
	if t.handler == "vide" && timescale > 0 {
		if t.delta > 0 {
			t.video.FPS = math.Round(float64(timescale)/t.delta*1000) / 1000
		}
	}
	t.audio.Lang, t.subtitle.Lang = lang, lang
	return t
}

// mdhdFields reads the time scale and the language of a media header.
func mdhdFields(d []byte) (scale int, lang string) {
	off := 20 // version 0: version/flags, two times, scale, length
	scaleAt := 12
	if d[0] == 1 {
		off, scaleAt = 32, 20
	}
	if len(d) >= scaleAt+4 {
		scale = be32(d[scaleAt:])
	}
	if len(d) < off+2 {
		return scale, ""
	}
	v := be16(d[off:])
	if v == 0 || v == 0x7FFF {
		return scale, ""
	}
	return scale, language(string([]byte{byte(v>>10&0x1F) + 0x60, byte(v>>5&0x1F) + 0x60, byte(v&0x1F) + 0x60}))
}

// mp4Sample reads minf > stbl: the sample description (codec and setup), the
// frame timing and where the first chunk is.
func (r *reader) mp4Sample(minf mp4Box, t *mp4TrackInfo, visited *int) {
	for _, stbl := range r.mp4Boxes(minf.start, minf.end, visited) {
		if stbl.typ != "stbl" {
			continue
		}
		for _, b := range r.mp4Boxes(stbl.start, stbl.end, visited) {
			switch b.typ {
			case "stsd":
				if d := r.data(b, smallBox); len(d) >= 16 {
					switch t.handler {
					case "vide":
						t.video = videoEntry(d[8:])
					case "soun":
						t.audio = audioEntry(d[8:])
					case "sbtl", "subt":
						t.subtitle = Subtitle{Codec: subtitleEntry(string(d[12:16]))}
					}
				}
			case "stts":
				if t.handler == "vide" {
					if d, err := r.at(b.start, min(b.end-b.start, 8+8*sttsEntries)); err == nil {
						t.delta = dominantDelta(d)
					}
				}
			case "stco", "co64":
				if t.handler == "vide" {
					if d, err := r.at(b.start, 16); err == nil && len(d) >= 12 {
						if b.typ == "stco" {
							t.firstChunk = int64(be32(d[8:]))
						} else if len(d) >= 16 {
							t.firstChunk = int64(binary.BigEndian.Uint64(d[8:16]))
						}
					}
				}
			}
		}
	}
}

// dominantDelta is the sample duration most samples have, from the start of
// the time-to-sample table; the caller divides the time scale by it.
func dominantDelta(d []byte) float64 {
	if len(d) < 8 {
		return 0
	}
	n := min(be32(d[4:]), (len(d)-8)/8)
	best, bestCount := 0, 0
	for i := 0; i < n; i++ {
		count, delta := be32(d[8+8*i:]), be32(d[12+8*i:])
		if delta > 0 && count > bestCount {
			best, bestCount = delta, count
		}
	}
	return float64(best)
}

// videoEntry reads a visual sample entry (the bytes after the stsd header: an
// entry's size, type and body).
func videoEntry(e []byte) Video {
	var v Video
	if len(e) < 8+78 {
		return v
	}
	typ := string(e[4:8])
	body := e[8:]
	v.Width, v.Height = be16(body[24:]), be16(body[26:])
	dv := typ == "dvh1" || typ == "dvhe" || typ == "dva1" || typ == "dvav" || typ == "dav1"
	transfer := 0
	obj := byte(0)
	var hvcC []byte
	boxChildren(body[78:], func(box string, d []byte) {
		switch box {
		case "avcC":
			v.BitDepth = avcBitDepth(d)
		case "hvcC":
			v.BitDepth = hevcBitDepth(d)
			hvcC = d
		case "av1C":
			v.BitDepth = av1BitDepth(d)
		case "vpcC":
			if len(d) >= 7 {
				v.BitDepth = int(d[6] >> 4)
			}
		case "dvcC", "dvvC":
			dv = true
			v.DVProfile = dolbyVisionProfile(d)
		case "colr":
			if len(d) >= 10 && (string(d[:4]) == "nclx" || string(d[:4]) == "nclc") {
				transfer = be16(d[6:])
			}
		case "esds":
			obj, _ = esdsInfo(d)
		}
	})
	switch typ {
	case "avc1", "avc3", "dva1", "dvav":
		v.Codec = "H.264"
	case "hvc1", "hev1", "dvh1", "dvhe":
		v.Codec = "HEVC"
	case "av01", "dav1":
		v.Codec = "AV1"
	case "vp09":
		v.Codec = "VP9"
	case "vp08":
		v.Codec = "VP8"
	case "mp4v":
		v.Codec = "MPEG-4"
		switch {
		case obj >= 0x60 && obj <= 0x65:
			v.Codec = "MPEG-2"
		case obj == 0x6A:
			v.Codec = "MPEG-1"
		}
	case "apch", "apcn", "apcs", "apco", "ap4h", "ap4x":
		v.Codec = "ProRes"
	case "jpeg", "png ", "mjpa", "mjpb": // cover art, not a picture track
		return Video{}
	default:
		v.Codec = strings.TrimSpace(typ)
	}
	if transfer == 0 && hvcC != nil {
		transfer = hevcTransfer(hvcC)
	}
	if dv {
		v.HDR = "Dolby Vision"
	} else if v.HDR = hdrFromTransfer(transfer); v.HDR == "" {
		v.HDR = "SDR"
	}
	return v
}

// audioEntry reads an audio sample entry.
func audioEntry(e []byte) Audio {
	var a Audio
	if len(e) < 8+28 {
		return a
	}
	typ := string(e[4:8])
	body := e[8:]
	version := be16(body[8:])
	a.Channels = be16(body[16:])
	start := 28
	switch version {
	case 1:
		start = 44
	case 2:
		start = 64
		a.Channels = 0
	}
	if start > len(body) {
		start = len(body)
	}
	obj := byte(0)
	var walk func(box string, d []byte)
	walk = func(box string, d []byte) {
		switch box {
		case "wave": // QuickTime keeps the setup one level down
			boxChildren(d, walk)
		case "esds":
			var ch int
			obj, ch = esdsInfo(d)
			if a.Channels == 0 {
				a.Channels = ch
			}
		case "dac3":
			if ch := ac3Channels(d); ch > 0 {
				a.Channels = ch
			}
		case "dec3":
			ch, atmos := ec3Info(d)
			if ch > 0 {
				a.Channels = ch
			}
			a.Atmos = atmos
		case "dOps":
			if len(d) >= 2 && d[1] > 0 {
				a.Channels = int(d[1])
			}
		}
	}
	boxChildren(body[start:], walk)
	switch typ {
	case "mp4a":
		a.Codec = mpeg4Audio(obj)
		if a.Codec == "" {
			a.Codec = "AAC"
		}
	case "ac-3":
		a.Codec = "AC-3"
	case "ec-3":
		a.Codec = "E-AC-3"
	case "Opus":
		a.Codec = "Opus"
	case "fLaC":
		a.Codec = "FLAC"
	case "alac":
		a.Codec = "ALAC"
	case "dtsc":
		a.Codec = "DTS"
	case "dtse":
		a.Codec = "DTS-HD HRA"
	case "dtsh":
		a.Codec = "DTS-HD"
	case "dtsl":
		a.Codec = "DTS-HD MA"
	case "dtsx":
		a.Codec = "DTS:X"
	case "mlpa":
		a.Codec = "TrueHD"
	case ".mp3", "mp3 ":
		a.Codec = "MP3"
	case "sowt", "twos", "lpcm", "in24", "fl32", "fl64", "raw ":
		a.Codec = "PCM"
	default:
		a.Codec = strings.TrimSpace(typ)
	}
	return a
}

func subtitleEntry(typ string) string {
	switch typ {
	case "tx3g":
		return "mov_text"
	case "wvtt":
		return "WebVTT"
	case "stpp":
		return "TTML"
	}
	return strings.TrimSpace(typ)
}
