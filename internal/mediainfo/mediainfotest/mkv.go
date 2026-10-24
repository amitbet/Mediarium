// Package mediainfotest builds tiny, valid Matroska files for tests in other
// packages: just the headers a real muxer writes, saying how long the video
// is, with no picture or sound in them.
package mediainfotest

import (
	"bytes"
	"encoding/binary"
	"math"
)

func eid(id int64) []byte {
	var b []byte
	for v := id; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	return b
}

func esize(n int) []byte {
	for l := 1; l <= 8; l++ {
		if int64(n) < (int64(1)<<(7*l))-1 {
			b := make([]byte, l)
			v := int64(n) | int64(1)<<(7*l)
			for i := l - 1; i >= 0; i-- {
				b[i] = byte(v)
				v >>= 8
			}
			return b
		}
	}
	panic("too big")
}

func el(id int64, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	return append(append(eid(id), esize(len(body))...), body...)
}

func euint(id int64, v uint64) []byte {
	var b []byte
	for ; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	if len(b) == 0 {
		b = []byte{0}
	}
	return el(id, b)
}

func efloat(id int64, f float64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(f))
	return el(id, b)
}

// MKV returns a Matroska file that says it is seconds long and holds one
// 1920x1080 H.264 video track.
func MKV(seconds float64) []byte {
	video := el(0xAE, euint(0x83, 1), el(0x86, []byte("V_MPEG4/ISO/AVC")),
		el(0xE0, euint(0xB0, 1920), euint(0xBA, 1080)))
	header := el(0x1A45DFA3, el(0x4282, []byte("matroska")), euint(0x4287, 4))
	info := el(0x1549A966, euint(0x2AD7B1, 1_000_000), efloat(0x4489, seconds*1000))
	tracks := el(0x1654AE6B, video)
	cluster := el(0x1F43B675, euint(0xE7, 0), el(0xA3, []byte("frames")))
	void := el(0xEC, make([]byte, 20))
	seg := el(0x18538067, void, info, tracks, cluster)
	return append(header, seg...)
}
