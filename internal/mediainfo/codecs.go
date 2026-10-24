package mediainfo

import (
	"bytes"
	"strings"
)

// The small parsers for what a codec keeps in its setup record (avcC, hvcC,
// av1C, the Dolby Vision record, AC-3's dac3/dec3). They only read what is
// there and return zero values for anything cut short.

// avcBitDepth is the luma bit depth of an H.264 setup record (avcC).
func avcBitDepth(c []byte) int {
	if len(c) < 7 {
		return 0
	}
	profile := c[1]
	switch profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
	default:
		return 8 // the profiles without a stated depth are 8-bit
	}
	p := 5
	for pass := 0; pass < 2; pass++ { // the SPS list, then the PPS list
		if p >= len(c) {
			break
		}
		n := int(c[p])
		if pass == 0 {
			n &= 0x1F
		}
		p++
		for i := 0; i < n; i++ {
			if p+2 > len(c) {
				return fallbackAVCDepth(profile)
			}
			p += 2 + int(c[p])<<8 + int(c[p+1])
		}
	}
	if p+2 <= len(c) { // chroma format, then the luma bit depth minus 8
		return int(c[p+1]&7) + 8
	}
	return fallbackAVCDepth(profile)
}

func fallbackAVCDepth(profile byte) int {
	if profile == 110 {
		return 10
	}
	return 0
}

// hevcBitDepth is the luma bit depth of an HEVC setup record (hvcC).
func hevcBitDepth(c []byte) int {
	if len(c) < 19 {
		return 0
	}
	return int(c[17]&7) + 8
}

// av1BitDepth is the bit depth of an AV1 setup record (av1C).
func av1BitDepth(c []byte) int {
	if len(c) < 4 {
		return 0
	}
	switch {
	case c[2]&0x20 != 0:
		return 12
	case c[2]&0x40 != 0:
		return 10
	}
	return 8
}

// dolbyVisionProfile reads a Dolby Vision configuration record (dvcC or
// dvvC); 0 when it is too short to be one.
func dolbyVisionProfile(rec []byte) int {
	if len(rec) < 4 {
		return 0
	}
	return int(rec[2] >> 1)
}

// hdrFromTransfer names the dynamic range from the colour transfer
// characteristic (ISO 23091-2): 16 is PQ (HDR10), 18 is HLG.
func hdrFromTransfer(transfer int) string {
	switch transfer {
	case 16:
		return "HDR10"
	case 18:
		return "HLG"
	case 0, 2: // unspecified
		return ""
	}
	return "SDR"
}

// hdr10PlusMark is how HDR10+ dynamic metadata starts inside a frame: an
// ITU-T T.35 message from Samsung (country 0xB5, provider 0x003C, oriented
// code 1, application 4).
var hdr10PlusMark = []byte{0xB5, 0x00, 0x3C, 0x00, 0x01, 0x04}

func hasHDR10Plus(frames []byte) bool { return bytes.Contains(frames, hdr10PlusMark) }

// ac3Channels reads the channel count from an AC-3 setup record (dac3).
func ac3Channels(c []byte) int {
	if len(c) < 3 {
		return 0
	}
	v := int(c[0])<<16 | int(c[1])<<8 | int(c[2])
	return acmodChannels[(v>>11)&7] + (v>>10)&1
}

var acmodChannels = [8]int{2, 1, 2, 3, 3, 4, 4, 5}

// bits reads a byte slice a bit at a time.
type bits struct {
	b   []byte
	pos int
}

func (r *bits) read(n int) int {
	v := 0
	for ; n > 0; n-- {
		if r.pos>>3 >= len(r.b) {
			r.pos++
			v <<= 1
			continue
		}
		v = v<<1 | int(r.b[r.pos>>3]>>(7-r.pos&7))&1
		r.pos++
	}
	return v
}

func (r *bits) left() int { return len(r.b)*8 - r.pos }

// ec3Info reads an E-AC-3 setup record (dec3): the channel count of the main
// programme, and whether it carries Dolby Atmos (the JOC extension flag).
func ec3Info(c []byte) (channels int, atmos bool) {
	if len(c) < 5 {
		return 0, false
	}
	r := &bits{b: c}
	r.read(13) // data rate
	subs := r.read(3) + 1
	for i := 0; i < subs && r.left() >= 24; i++ {
		r.read(2) // fscod
		r.read(5) // bsid
		r.read(2) // reserved, asvc
		r.read(3) // bsmod
		acmod := r.read(3)
		lfe := r.read(1)
		r.read(3)
		dep := r.read(4)
		extra := 0
		if dep > 0 {
			loc := r.read(9)
			for bit, n := range [9]int{2, 2, 1, 1, 2, 2, 2, 1, 1} {
				if loc>>(8-bit)&1 == 1 {
					extra += n
				}
			}
		} else {
			r.read(1)
		}
		if i == 0 {
			channels = acmodChannels[acmod] + lfe + extra
		}
	}
	if r.left() >= 16 {
		r.read(7)
		atmos = r.read(1) == 1
	}
	return channels, atmos
}

// audioFromName refines a DTS or Dolby track from what its name says, since
// the container only knows "DTS" and "TrueHD".
func audioFromName(codec, name string) (string, bool) {
	n := strings.ToLower(name)
	atmos := strings.Contains(n, "atmos")
	if codec == "DTS" {
		switch {
		case strings.Contains(n, "dts:x") || strings.Contains(n, "dts-x"):
			codec = "DTS:X"
		case strings.Contains(n, "-hd ma") || strings.Contains(n, "hd master") || strings.Contains(n, "dts-hd ma") || strings.Contains(n, "dts-hdma") || strings.Contains(n, "dts hd ma"):
			codec = "DTS-HD MA"
		case strings.Contains(n, "-hd hra") || strings.Contains(n, "high res") || strings.Contains(n, "dts-hdhra"):
			codec = "DTS-HD HRA"
		case strings.Contains(n, "dts-hd"):
			codec = "DTS-HD"
		}
	}
	return codec, atmos
}

// mpeg4Audio names the audio codec of an MPEG-4 object type indication.
func mpeg4Audio(obj byte) string {
	switch obj {
	case 0x40, 0x66, 0x67, 0x68:
		return "AAC"
	case 0x69, 0x6B:
		return "MP3"
	case 0x6A:
		return "MP2"
	case 0xA5:
		return "AC-3"
	case 0xA6:
		return "E-AC-3"
	case 0xA9:
		return "DTS"
	case 0xAA:
		return "DTS-HD HRA"
	case 0xAB:
		return "DTS-HD MA"
	case 0xAC:
		return "DTS-HD"
	case 0xDD:
		return "Vorbis"
	}
	return ""
}

// esdsInfo reads an MP4 elementary stream descriptor: the object type and the
// channel count the AAC setup bytes give.
func esdsInfo(d []byte) (obj byte, channels int) {
	if len(d) < 4 {
		return 0, 0
	}
	d = d[4:] // version and flags
	p := 0
	next := func() (tag byte, size int, ok bool) {
		if p >= len(d) {
			return 0, 0, false
		}
		tag = d[p]
		p++
		for i := 0; i < 4; i++ {
			if p >= len(d) {
				return 0, 0, false
			}
			size = size<<7 | int(d[p]&0x7F)
			p++
			if d[p-1]&0x80 == 0 {
				break
			}
		}
		return tag, size, true
	}
	for steps := 0; steps < 8; steps++ {
		tag, size, ok := next()
		if !ok {
			return
		}
		switch tag {
		case 0x03: // ES descriptor: id, flags, optional fields
			if p+3 > len(d) {
				return
			}
			fl := d[p+2]
			p += 3
			if fl&0x80 != 0 {
				p += 2
			}
			if fl&0x40 != 0 && p < len(d) {
				p += 1 + int(d[p])
			}
			if fl&0x20 != 0 {
				p += 2
			}
		case 0x04: // decoder config: object type, stream type, buffer size, bitrates
			if p+13 > len(d) {
				return
			}
			obj = d[p]
			p += 13
		case 0x05: // the AAC setup bytes
			if p+size > len(d) || size < 2 {
				return
			}
			asc := d[p : p+size]
			if obj == 0x40 || obj == 0x66 || obj == 0x67 || obj == 0x68 {
				r := &bits{b: asc}
				if aot := r.read(5); aot != 31 {
					if r.read(4) == 15 {
						r.read(24)
					}
					if cfg := r.read(4); cfg < 8 {
						channels = [8]int{0, 1, 2, 3, 4, 5, 6, 8}[cfg]
					}
				}
			}
			return
		default:
			p += size
		}
	}
	return
}
