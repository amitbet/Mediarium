package mediainfo

import (
	"math"
	"strings"
)

// Matroska and WebM: the EBML header (for the document type), the Segment's
// Info (length) and Tracks. Elements are only ever walked inside a buffer
// that was read with a size limit, and the layout is followed by fixed
// levels, never recursively, so a damaged or hostile file can't loop.

const (
	mkEBMLHeader   = 0x1A45DFA3
	mkSegment      = 0x18538067
	mkSeekHead     = 0x114D9B74
	mkSeek         = 0x4DBB
	mkSeekID       = 0x53AB
	mkSeekPosition = 0x53AC
	mkInfo         = 0x1549A966
	mkTracks       = 0x1654AE6B
	mkCluster      = 0x1F43B675

	mkDocType        = 0x4282
	mkTimestampScale = 0x2AD7B1
	mkDuration       = 0x4489

	mkTrackEntry     = 0xAE
	mkTrackType      = 0x83
	mkCodecID        = 0x86
	mkCodecPrivate   = 0x63A2
	mkLanguage       = 0x22B59C
	mkLanguageBCP47  = 0x22B59D
	mkName           = 0x536E
	mkFlagDefault    = 0x88
	mkFlagForced     = 0x55AA
	mkDefaultDur     = 0x23E383
	mkVideo          = 0xE0
	mkAudio          = 0xE1
	mkPixelWidth     = 0xB0
	mkPixelHeight    = 0xBA
	mkChannels       = 0x9F
	mkColour         = 0x55B0
	mkBitsPerChannel = 0x55B2
	mkTransfer       = 0x55BA
	mkBlockAddMap    = 0x41E4
	mkBlockAddType   = 0x41E7
	mkBlockAddData   = 0x41ED

	mkTypeVideo    = 1
	mkTypeAudio    = 2
	mkTypeSubtitle = 17

	mkSmallLimit  = 64 << 10 // Info, SeekHead
	mkTracksLimit = 4 << 20
	mkScanLimit   = 256 // elements walked at the top of the Segment
	mkFrameSlice  = 1 << 20

	mkDVcC = 0x64766343 // "dvcC"
	mkDVvC = 0x64767643 // "dvvC"
)

// ebmlHeader reads an element's id and size from the start of b, and how long
// the header is. A size of all ones is -1 (unknown).
func ebmlHeader(b []byte) (id, size int64, n int, ok bool) {
	if len(b) < 2 {
		return 0, 0, 0, false
	}
	idLen := vintLen(b[0])
	if idLen == 0 || idLen > 4 || len(b) < idLen+1 {
		return 0, 0, 0, false
	}
	for i := 0; i < idLen; i++ {
		id = id<<8 | int64(b[i])
	}
	b = b[idLen:]
	sLen := vintLen(b[0])
	if sLen == 0 || len(b) < sLen {
		return 0, 0, 0, false
	}
	size = int64(b[0]) & (0xFF >> sLen)
	allOnes := size == int64(0xFF>>sLen)
	for i := 1; i < sLen; i++ {
		size = size<<8 | int64(b[i])
		allOnes = allOnes && b[i] == 0xFF
	}
	if allOnes {
		size = -1
	}
	return id, size, idLen + sLen, true
}

// vintLen is the length a variable-length number's first byte announces
// (0 when it isn't valid).
func vintLen(first byte) int {
	for n, mask := 1, byte(0x80); n <= 8; n, mask = n+1, mask>>1 {
		if first&mask != 0 {
			return n
		}
	}
	return 0
}

// ebmlChildren calls fn for each element directly inside buf. A child whose
// size runs past the buffer ends the walk.
func ebmlChildren(buf []byte, fn func(id int64, data []byte)) {
	for len(buf) > 0 {
		id, size, n, ok := ebmlHeader(buf)
		if !ok || size < 0 || size > int64(len(buf)-n) {
			return
		}
		fn(id, buf[n:n+int(size)])
		buf = buf[n+int(size):]
	}
}

func beUint(b []byte) uint64 {
	var v uint64
	for _, c := range b[:min(len(b), 8)] {
		v = v<<8 | uint64(c)
	}
	return v
}

func beFloat(b []byte) float64 {
	switch len(b) {
	case 4:
		return float64(math.Float32frombits(uint32(beUint(b))))
	case 8:
		return math.Float64frombits(beUint(b))
	}
	return 0
}

func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func readMatroska(r *reader) (Info, error) {
	info := Info{Container: "matroska"}
	b, err := r.at(0, 16)
	if err != nil {
		return info, err
	}
	_, size, n, ok := ebmlHeader(b)
	if !ok || size < 0 || size > 4096 {
		return info, ErrUnreadable
	}
	if body, err := r.at(int64(n), size); err == nil {
		ebmlChildren(body, func(id int64, v []byte) {
			if id == mkDocType && cString(v) == "webm" {
				info.Container = "webm"
			}
		})
	}
	pos := int64(n) + size
	b, err = r.at(pos, 16)
	if err != nil {
		return info, err
	}
	id, segSize, n, ok := ebmlHeader(b)
	if !ok || id != mkSegment {
		return info, ErrUnreadable
	}
	segStart := pos + int64(n)
	segEnd := r.size
	if segSize >= 0 && segStart+segSize < segEnd {
		segEnd = segStart + segSize
	}

	var infoAt, tracksAt, clusterAt int64 = -1, -1, -1
	var infoBuf, tracksBuf []byte
	pos = segStart
scan:
	for i := 0; i < mkScanLimit && pos+2 <= segEnd; i++ {
		b, err := r.at(pos, 16)
		if err != nil {
			return info, err
		}
		id, size, n, ok := ebmlHeader(b)
		if !ok {
			break
		}
		switch id {
		case mkCluster:
			clusterAt = pos
			break scan // the track list comes before the pictures, or the SeekHead says where it is
		case mkInfo:
			if size >= 0 && size <= mkSmallLimit {
				infoBuf, _ = r.at(pos+int64(n), size)
			}
		case mkTracks:
			if size < 0 || size > mkTracksLimit {
				return info, ErrUnreadable
			}
			tracksBuf, err = r.at(pos+int64(n), size)
			if err != nil {
				return info, err
			}
		case mkSeekHead:
			if size >= 0 && size <= mkSmallLimit {
				if sb, err := r.at(pos+int64(n), size); err == nil {
					ebmlChildren(sb, func(id int64, entry []byte) {
						if id != mkSeek {
							return
						}
						var target int64
						at := int64(-1)
						ebmlChildren(entry, func(id int64, v []byte) {
							switch id {
							case mkSeekID:
								target = int64(beUint(v))
							case mkSeekPosition:
								at = int64(beUint(v))
							}
						})
						if at >= 0 && at < segEnd-segStart {
							switch target {
							case mkInfo:
								infoAt = segStart + at
							case mkTracks:
								tracksAt = segStart + at
							}
						}
					})
				}
			}
		}
		if size < 0 {
			break
		}
		pos += int64(n) + size
	}
	if infoBuf == nil && infoAt >= 0 {
		if b, err := r.at(infoAt, 16); err == nil {
			if id, size, n, ok := ebmlHeader(b); ok && id == mkInfo && size >= 0 && size <= mkSmallLimit {
				infoBuf, _ = r.at(infoAt+int64(n), size)
			}
		}
	}
	if tracksBuf == nil && tracksAt >= 0 {
		if b, err := r.at(tracksAt, 16); err == nil {
			if id, size, n, ok := ebmlHeader(b); ok && id == mkTracks && size >= 0 && size <= mkTracksLimit {
				tracksBuf, err = r.at(tracksAt+int64(n), size)
				if err != nil {
					return info, err
				}
			}
		}
	}

	scale := 1_000_000.0
	var dur float64
	ebmlChildren(infoBuf, func(id int64, v []byte) {
		switch id {
		case mkTimestampScale:
			if s := float64(beUint(v)); s > 0 {
				scale = s
			}
		case mkDuration:
			dur = beFloat(v)
		}
	})
	if dur > 0 && !math.IsInf(dur, 0) && !math.IsNaN(dur) {
		info.Duration = math.Round(dur*scale/1e9*10) / 10
	}

	tracks := 0
	var plus bool
	ebmlChildren(tracksBuf, func(id int64, entry []byte) {
		if id != mkTrackEntry || tracks >= maxTracks {
			return
		}
		tracks++
		t := mkvTrack(entry)
		switch t.kind {
		case mkTypeVideo:
			if info.Video != nil {
				return
			}
			v := t.video()
			info.Video = &v
			plus = v.HDR == "HDR10" && v.Codec == "HEVC" && clusterAt >= 0
		case mkTypeAudio:
			info.Audio = append(info.Audio, t.audio())
		case mkTypeSubtitle:
			info.Subtitles = append(info.Subtitles, t.subtitle())
		}
	})
	if plus {
		if frames, err := r.at(clusterAt, mkFrameSlice); err == nil && hasHDR10Plus(frames) {
			info.Video.HDR = "HDR10+"
		}
	}
	return info, nil
}

type mkTrackInfo struct {
	kind                 uint64
	codec, name          string
	lang, bcp47          string
	private              []byte
	isDefault, forced    bool
	defaultDur           uint64
	width, height, depth int
	channels             int
	transfer             int
	dvProfile            int
}

func mkvTrack(entry []byte) mkTrackInfo {
	t := mkTrackInfo{lang: "eng", isDefault: true}
	ebmlChildren(entry, func(id int64, v []byte) {
		switch id {
		case mkTrackType:
			t.kind = beUint(v)
		case mkCodecID:
			t.codec = cString(v)
		case mkCodecPrivate:
			t.private = v
		case mkLanguage:
			t.lang = cString(v)
		case mkLanguageBCP47:
			t.bcp47 = cString(v)
		case mkName:
			t.name = cString(v)
		case mkFlagDefault:
			t.isDefault = beUint(v) != 0
		case mkFlagForced:
			t.forced = beUint(v) == 1
		case mkDefaultDur:
			t.defaultDur = beUint(v)
		case mkVideo:
			ebmlChildren(v, func(id int64, v []byte) {
				switch id {
				case mkPixelWidth:
					t.width = int(min(beUint(v), 1<<20))
				case mkPixelHeight:
					t.height = int(min(beUint(v), 1<<20))
				case mkColour:
					ebmlChildren(v, func(id int64, v []byte) {
						switch id {
						case mkBitsPerChannel:
							t.depth = int(min(beUint(v), 32))
						case mkTransfer:
							t.transfer = int(min(beUint(v), 255))
						}
					})
				}
			})
		case mkAudio:
			ebmlChildren(v, func(id int64, v []byte) {
				if id == mkChannels {
					t.channels = int(min(beUint(v), 255))
				}
			})
		case mkBlockAddMap:
			var kind uint64
			var data []byte
			ebmlChildren(v, func(id int64, v []byte) {
				switch id {
				case mkBlockAddType:
					kind = beUint(v)
				case mkBlockAddData:
					data = v
				}
			})
			if kind == mkDVcC || kind == mkDVvC {
				if p := dolbyVisionProfile(data); p > 0 {
					t.dvProfile = p
				}
			}
		}
	})
	return t
}

func (t mkTrackInfo) langCode() string { return language(t.bcp47, t.lang) }

func (t mkTrackInfo) video() Video {
	v := Video{Codec: mkvVideoCodec(t.codec, t.private), Width: t.width, Height: t.height, BitDepth: t.depth, DVProfile: t.dvProfile}
	if t.defaultDur > 0 {
		v.FPS = math.Round(1e9/float64(t.defaultDur)*1000) / 1000
	}
	if v.BitDepth == 0 {
		switch v.Codec {
		case "H.264":
			v.BitDepth = avcBitDepth(t.private)
		case "HEVC":
			v.BitDepth = hevcBitDepth(t.private)
		case "AV1":
			v.BitDepth = av1BitDepth(t.private)
		}
	}
	switch {
	case v.DVProfile > 0:
		v.HDR = "Dolby Vision"
	default:
		transfer := t.transfer
		if transfer == 0 && v.Codec == "HEVC" {
			transfer = hevcTransfer(t.private) // a file that doesn't say, but the video's own header does
		}
		v.HDR = hdrFromTransfer(transfer)
		if v.HDR == "" {
			v.HDR = "SDR"
		}
	}
	return v
}

func (t mkTrackInfo) audio() Audio {
	codec := mkvAudioCodec(t.codec)
	codec, atmos := audioFromName(codec, t.name)
	return Audio{Codec: codec, Channels: t.channels, Lang: t.langCode(), Title: t.name, Default: t.isDefault, Forced: t.forced, Atmos: atmos}
}

func (t mkTrackInfo) subtitle() Subtitle {
	forced := t.forced
	if !forced {
		n := strings.ToLower(t.name)
		forced = strings.Contains(n, "forced") || strings.Contains(n, "foreign parts")
	}
	return Subtitle{Codec: mkvSubtitleCodec(t.codec), Lang: t.langCode(), Title: t.name, Default: t.isDefault, Forced: forced}
}

func mkvVideoCodec(id string, private []byte) string {
	switch {
	case id == "V_MPEG4/ISO/AVC":
		return "H.264"
	case id == "V_MPEGH/ISO/HEVC":
		return "HEVC"
	case id == "V_AV1":
		return "AV1"
	case id == "V_VP9":
		return "VP9"
	case id == "V_VP8":
		return "VP8"
	case id == "V_MPEG2":
		return "MPEG-2"
	case id == "V_MPEG1":
		return "MPEG-1"
	case strings.HasPrefix(id, "V_MPEG4/ISO/"):
		return "MPEG-4"
	case id == "V_MS/VFW/FOURCC" && len(private) >= 20:
		return fourccVideo(string(private[16:20]))
	case id == "V_THEORA":
		return "Theora"
	}
	return strings.TrimPrefix(id, "V_")
}

func fourccVideo(cc string) string {
	switch strings.ToUpper(cc) {
	case "WVC1", "WMV3", "WMV2", "WMV1":
		return "VC-1"
	case "XVID", "DIVX", "DX50", "FMP4", "MP4V", "3IV2", "DIV3", "MP43":
		return "MPEG-4"
	case "H264", "AVC1", "X264":
		return "H.264"
	case "MPG2", "MPEG":
		return "MPEG-2"
	}
	return strings.TrimSpace(cc)
}

func mkvAudioCodec(id string) string {
	switch {
	case strings.HasPrefix(id, "A_AAC"):
		return "AAC"
	case id == "A_EAC3":
		return "E-AC-3"
	case strings.HasPrefix(id, "A_AC3"):
		return "AC-3"
	case id == "A_DTS/LOSSLESS":
		return "DTS-HD MA"
	case id == "A_DTS/EXPRESS":
		return "DTS-HD HRA"
	case strings.HasPrefix(id, "A_DTS"):
		return "DTS"
	case id == "A_TRUEHD":
		return "TrueHD"
	case id == "A_MLP":
		return "MLP"
	case id == "A_FLAC":
		return "FLAC"
	case id == "A_OPUS":
		return "Opus"
	case id == "A_VORBIS":
		return "Vorbis"
	case id == "A_MPEG/L3":
		return "MP3"
	case id == "A_MPEG/L2":
		return "MP2"
	case id == "A_MPEG/L1":
		return "MP1"
	case strings.HasPrefix(id, "A_PCM"):
		return "PCM"
	case id == "A_ALAC":
		return "ALAC"
	}
	return strings.TrimPrefix(id, "A_")
}

func mkvSubtitleCodec(id string) string {
	switch id {
	case "S_TEXT/UTF8", "S_TEXT/ASCII":
		return "SRT"
	case "S_TEXT/ASS", "S_TEXT/SSA", "S_ASS", "S_SSA":
		return "ASS"
	case "S_HDMV/PGS":
		return "PGS"
	case "S_HDMV/TEXTST":
		return "TextST"
	case "S_VOBSUB":
		return "VobSub"
	case "S_DVBSUB":
		return "DVB"
	case "S_TEXT/WEBVTT":
		return "WebVTT"
	}
	return strings.TrimPrefix(id, "S_")
}
