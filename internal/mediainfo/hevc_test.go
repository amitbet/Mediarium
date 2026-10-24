package mediainfo

import (
	"encoding/hex"
	"testing"
)

// Setup records (hvcC) cut out of small x265 encodes whose files carry no
// colour element, with the SEI array removed. The colour transfer is only in
// the video's own header there.
const (
	hvccPQ  = "0102200000009000000000003ff000fcfdfafa00000f03200001001840010c01ffff02200000030090000003000003003f959809210001002e42010102200000030090000003000003003fa005020169365959a4932bc05a84880482000003000200000300301022000100074401c172b46240"
	hvccHLG = "0102200000009000000000003cf000fcfdfafa00000f03200001001840010c01ffff02200000030090000003000003003c959809210001002d42010102200000030090000003000003003ca00a080f1365959a4932bc05a8489048200000030020000003030122000100074401c172b46240"
	hvccSDR = "0101600000009000000000003cf000fcfdf8f800000f03200001001840010c01ffff01600000030090000003000003003c959809210001002d42010101600000030090000003000003003ca00a080f165959a4932bc05a80808082000003000200000300301022000100074401c172b46240"
	// more reference pictures and B-frames: a longer short-term set list in the SPS
	hvccPQRefs = "0102200000009000000000003cf000fcfdfafa00000f03200001001940010c01ffff02200000030090000003000003003c998a0240210001002e42010102200000030090000003000003003ca00a080f1365998a924caf016a12201208000003000800000300c04022000100074401c172b46240"
)

func TestHEVCTransferFromTheSequenceHeader(t *testing.T) {
	tests := []struct {
		name string
		hvcc string
		want int
		hdr  string
	}{
		{"PQ", hvccPQ, 16, "HDR10"},
		{"HLG", hvccHLG, 18, "HLG"},
		{"BT.709", hvccSDR, 1, "SDR"},
		{"PQ with more reference pictures", hvccPQRefs, 16, "HDR10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := hex.DecodeString(tt.hvcc)
			if got := hevcTransfer(rec); got != tt.want {
				t.Fatalf("hevcTransfer = %d, want %d", got, tt.want)
			}
			// the same record in a Matroska track with no colour element ...
			mkv := read(t, buildMKV("matroska", 10, nil, mkvT{kind: mkTypeVideo, codec: "V_MPEGH/ISO/HEVC", w: 3840, h: 2160, private: rec}), "mkv")
			if mkv.Video.HDR != tt.hdr {
				t.Errorf("mkv: %+v", mkv.Video)
			}
			// ... and in an MP4 without a colr box
			mp4 := read(t, buildMP4("isom", 10, nil, mp4T{handler: "vide", w: 3840, h: 2160, entry: visualEntry("hvc1", 3840, 2160, box("hvcC", rec))}), "mp4")
			if mp4.Video.HDR != tt.hdr {
				t.Errorf("mp4: %+v", mp4.Video)
			}
		})
	}
}

func TestHEVCTransferIgnoresDamage(t *testing.T) {
	rec, _ := hex.DecodeString(hvccPQ)
	for n := 0; n < len(rec); n++ {
		hevcTransfer(rec[:n])
	}
	for i := range rec {
		d := append([]byte(nil), rec...)
		d[i] ^= 0xFF
		hevcTransfer(d)
	}
}
