package mediainfo

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Helpers that build tiny, valid Matroska and MP4 byte streams for the tests:
// just the headers a real muxer writes, with no pictures or sound in them.

// ---- EBML ----

func eid(id int64) []byte {
	var b []byte
	for v := id; v > 0; v >>= 8 {
		b = append([]byte{byte(v)}, b...)
	}
	return b
}

// esize writes a size in the fewest bytes.
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

func estr(id int64, s string) []byte { return el(id, []byte(s)) }

func efloat(id int64, f float64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, math.Float64bits(f))
	return el(id, b)
}

type mkvT struct {
	kind                  uint64
	codec, lang, name     string
	bcp47                 string
	private               []byte
	noDefault, forced     bool
	defaultDur            uint64
	w, h, depth, transfer int
	channels              int
	dv                    []byte // a Dolby Vision record
}

func (t mkvT) bytes() []byte {
	parts := [][]byte{euint(mkTrackType, t.kind), estr(mkCodecID, t.codec)}
	if t.lang != "" {
		parts = append(parts, estr(mkLanguage, t.lang))
	}
	if t.bcp47 != "" {
		parts = append(parts, estr(mkLanguageBCP47, t.bcp47))
	}
	if t.name != "" {
		parts = append(parts, estr(mkName, t.name))
	}
	if t.noDefault {
		parts = append(parts, euint(mkFlagDefault, 0))
	}
	if t.forced {
		parts = append(parts, euint(mkFlagForced, 1))
	}
	if t.private != nil {
		parts = append(parts, el(mkCodecPrivate, t.private))
	}
	if t.defaultDur > 0 {
		parts = append(parts, euint(mkDefaultDur, t.defaultDur))
	}
	if t.kind == mkTypeVideo {
		v := [][]byte{euint(mkPixelWidth, uint64(t.w)), euint(mkPixelHeight, uint64(t.h))}
		var col [][]byte
		if t.depth > 0 {
			col = append(col, euint(mkBitsPerChannel, uint64(t.depth)))
		}
		if t.transfer > 0 {
			col = append(col, euint(mkTransfer, uint64(t.transfer)))
		}
		if col != nil {
			v = append(v, el(mkColour, col...))
		}
		parts = append(parts, el(mkVideo, v...))
	}
	if t.kind == mkTypeAudio {
		parts = append(parts, el(mkAudio, efloat(0xB5, 48000), euint(mkChannels, uint64(t.channels))))
	}
	if t.dv != nil {
		parts = append(parts, el(mkBlockAddMap, euint(mkBlockAddType, mkDVvC), el(mkBlockAddData, t.dv)))
	}
	return el(mkTrackEntry, parts...)
}

// buildMKV writes a Matroska file: EBML header, then a Segment with a
// SeekHead, Info, Tracks and one Cluster holding frames.
func buildMKV(doc string, seconds float64, frames []byte, tracks ...mkvT) []byte {
	var tr [][]byte
	for _, t := range tracks {
		tr = append(tr, t.bytes())
	}
	header := el(mkEBMLHeader, estr(mkDocType, doc), euint(0x4287, 4))
	info := el(mkInfo, euint(mkTimestampScale, 1_000_000), efloat(mkDuration, seconds*1000))
	tracksEl := el(mkTracks, tr...)
	cluster := el(mkCluster, euint(0xE7, 0), el(0xA3, frames))
	void := el(0xEC, make([]byte, 20))
	seg := el(mkSegment, void, info, tracksEl, cluster)
	return append(header, seg...)
}

// ---- MP4 ----

func box(typ string, parts ...[]byte) []byte {
	body := bytes.Join(parts, nil)
	b := make([]byte, 8, 8+len(body))
	binary.BigEndian.PutUint32(b, uint32(8+len(body)))
	copy(b[4:], typ)
	return append(b, body...)
}

func fullbox(typ string, version byte, parts ...[]byte) []byte {
	return box(typ, append([][]byte{{version, 0, 0, 0}}, parts...)...)
}

func u16(v int) []byte { return []byte{byte(v >> 8), byte(v)} }
func u32(v int) []byte { return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)} }

// mdhdBox is a media header with a time scale, length and a packed language.
func mdhdBox(scale, length int, lang string) []byte {
	packed := 0
	for _, c := range []byte(lang) {
		packed = packed<<5 | int(c-0x60)
	}
	return fullbox("mdhd", 0, u32(0), u32(0), u32(scale), u32(length), u16(packed), u16(0))
}

func hdlrBox(kind string) []byte {
	return fullbox("hdlr", 0, u32(0), []byte(kind), make([]byte, 12), []byte{0})
}

func visualEntry(typ string, w, h int, children ...[]byte) []byte {
	body := bytes.Join([][]byte{
		make([]byte, 6), u16(1), // reserved, data reference
		make([]byte, 16), // pre-defined, reserved
		u16(w), u16(h), u32(0x480000), u32(0x480000), u32(0), u16(1),
		make([]byte, 32), u16(24), u16(0xFFFF),
	}, nil)
	return box(typ, append([][]byte{body}, children...)...)
}

func audioEntryBox(typ string, channels int, children ...[]byte) []byte {
	body := bytes.Join([][]byte{
		make([]byte, 6), u16(1),
		u16(0), u16(0), u32(0), // sound version, revision, vendor
		u16(channels), u16(16), u16(0), u16(0), u32(48000 << 16),
	}, nil)
	return box(typ, append([][]byte{body}, children...)...)
}

func stsdBox(entry []byte) []byte { return fullbox("stsd", 0, u32(1), entry) }

type mp4T struct {
	handler string
	lang    string
	entry   []byte
	scale   int
	delta   int // frame length, for the frame rate
	frames  []byte
	w, h    int
}

func (t mp4T) trak(chunkAt int) []byte {
	stbl := []byte{}
	stbl = append(stbl, stsdBox(t.entry)...)
	if t.delta > 0 {
		stbl = append(stbl, fullbox("stts", 0, u32(2), u32(1), u32(t.delta*2), u32(5000), u32(t.delta))...)
	}
	stbl = append(stbl, fullbox("stco", 0, u32(1), u32(chunkAt))...)
	tkhd := fullbox("tkhd", 0, make([]byte, 72), u32(t.w<<16), u32(t.h<<16))
	scale := t.scale
	if scale == 0 {
		scale = 1000
	}
	return box("trak", tkhd, box("mdia", mdhdBox(scale, 60*scale, t.lang), hdlrBox(t.handler), box("minf", box("stbl", stbl))))
}

// buildMP4 writes ftyp, an mdat holding frames, then moov (the layout of a
// file made without "faststart").
func buildMP4(brand string, seconds int, frames []byte, tracks ...mp4T) []byte {
	ftyp := box("ftyp", []byte(brand), u32(0), []byte("isom"))
	mdat := box("mdat", frames)
	chunkAt := len(ftyp) + 8
	var traks [][]byte
	for _, t := range tracks {
		traks = append(traks, t.trak(chunkAt))
	}
	mvhd := fullbox("mvhd", 0, u32(0), u32(0), u32(1000), u32(seconds*1000), make([]byte, 80))
	moov := box("moov", append([][]byte{mvhd}, traks...)...)
	return bytes.Join([][]byte{ftyp, mdat, moov}, nil)
}

// A few setup records.

func avcCRecord(profile byte, depth int) []byte {
	sps := []byte{0x67, profile, 0, 40}
	rec := []byte{1, profile, 0, 40, 0xFF, 0xE1, 0, byte(len(sps))}
	rec = append(rec, sps...)
	rec = append(rec, 1, 0, 2, 0x68, 0xEE) // one PPS
	if profile >= 100 {
		rec = append(rec, 0xFC|1, 0xF8|byte(depth-8), 0xF8|byte(depth-8), 0)
	}
	return rec
}

func hvcCRecord(depth int) []byte {
	rec := make([]byte, 23)
	rec[0] = 1
	rec[17] = 0xF8 | byte(depth-8)
	rec[18] = 0xF8 | byte(depth-8)
	return rec
}

func esdsAAC(channels int) []byte {
	cfg := []byte{0x12, byte(channels << 3)} // AAC-LC, 44.1 kHz, then the channel configuration
	dec := append([]byte{0x04, byte(13 + 2 + len(cfg)), 0x40, 0x15}, make([]byte, 11)...)
	dec = append(append(dec, 0x05, byte(len(cfg))), cfg...)
	return fullbox("esds", 0, []byte{0x03, byte(3 + len(dec) + 3), 0, 1, 0}, dec, []byte{0x06, 1, 2})
}

func dvRecord(profile int) []byte {
	return []byte{1, 0, byte(profile << 1), 0x01, 0x10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
}

func dac3Record(acmod, lfe int) []byte {
	v := acmod<<11 | lfe<<10
	return []byte{byte(v >> 16), byte(v >> 8), byte(v)}
}

// dec3Record is an E-AC-3 setup record with one independent substream, and
// the Atmos (JOC) extension when jocLevel is not 0.
func dec3Record(acmod, lfe, jocLevel int) []byte {
	w := &bitWriter{}
	w.put(768, 13)
	w.put(0, 3) // one substream
	w.put(0, 2)
	w.put(16, 5)
	w.put(0, 2)
	w.put(0, 3)
	w.put(acmod, 3)
	w.put(lfe, 1)
	w.put(0, 3)
	w.put(0, 4)
	w.put(0, 1)
	if jocLevel > 0 {
		w.put(0, 7)
		w.put(1, 1)
		w.put(jocLevel, 8)
	}
	return w.bytes()
}

type bitWriter struct {
	b   []byte
	pos int
}

func (w *bitWriter) put(v, n int) {
	for i := n - 1; i >= 0; i-- {
		if w.pos%8 == 0 {
			w.b = append(w.b, 0)
		}
		if v>>i&1 == 1 {
			w.b[len(w.b)-1] |= 0x80 >> (w.pos % 8)
		}
		w.pos++
	}
}

func (w *bitWriter) bytes() []byte { return w.b }
