package mediainfo

// Some files (an x265 encode muxed as it came out) say nothing about their
// HDR in the container; the colour transfer is then only in the video's own
// header, the VUI of the HEVC sequence parameter set kept in the setup record.
// hevcTransfer reads just enough of that set to reach it. Anything unexpected
// gives 0 ("not stated"); every loop is bounded by a small constant.

// bitReader reads bits and Exp-Golomb numbers, undoing the emulation
// prevention bytes (00 00 03) of a NAL unit. A read past the end sets bad.
type bitReader struct {
	b   []byte
	pos int
	bad bool
}

func newNALReader(nal []byte) *bitReader {
	out := make([]byte, 0, len(nal))
	zeros := 0
	for _, c := range nal {
		if zeros >= 2 && c == 3 {
			zeros = 0
			continue
		}
		if c == 0 {
			zeros++
		} else {
			zeros = 0
		}
		out = append(out, c)
	}
	return &bitReader{b: out}
}

func (r *bitReader) bit() int {
	if r.pos>>3 >= len(r.b) {
		r.bad = true
		return 0
	}
	v := int(r.b[r.pos>>3]>>(7-r.pos&7)) & 1
	r.pos++
	return v
}

func (r *bitReader) u(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		v = v<<1 | r.bit()
	}
	return v
}

func (r *bitReader) ue() int {
	zeros := 0
	for r.bit() == 0 && !r.bad {
		if zeros++; zeros > 24 {
			r.bad = true
			return 0
		}
	}
	return 1<<zeros - 1 + r.u(zeros)
}

func (r *bitReader) se() int {
	v := r.ue()
	if v&1 == 1 {
		return (v + 1) / 2
	}
	return -v / 2
}

// hevcTransfer finds the transfer characteristics in an hvcC record.
func hevcTransfer(hvcC []byte) int {
	if len(hvcC) < 24 {
		return 0
	}
	p := 23
	for arrays := int(hvcC[22]); arrays > 0 && p+3 <= len(hvcC); arrays-- {
		typ := hvcC[p] & 0x3F
		count := int(hvcC[p+1])<<8 | int(hvcC[p+2])
		p += 3
		for ; count > 0 && p+2 <= len(hvcC); count-- {
			n := int(hvcC[p])<<8 | int(hvcC[p+1])
			p += 2
			if p+n > len(hvcC) {
				return 0
			}
			if typ == 33 && n > 2 { // a sequence parameter set
				return spsTransfer(hvcC[p+2 : p+n])
			}
			p += n
		}
	}
	return 0
}

func spsTransfer(nal []byte) int {
	r := newNALReader(nal)
	r.u(4) // VPS id
	maxSub := r.u(3)
	r.u(1)
	// profile_tier_level
	r.u(96)
	profile := make([]bool, 8)
	level := make([]bool, 8)
	for i := 0; i < maxSub; i++ {
		profile[i], level[i] = r.bit() == 1, r.bit() == 1
	}
	if maxSub > 0 {
		for i := maxSub; i < 8; i++ {
			r.u(2)
		}
	}
	for i := 0; i < maxSub; i++ {
		if profile[i] {
			r.u(88)
		}
		if level[i] {
			r.u(8)
		}
	}
	r.ue() // SPS id
	if r.ue() == 3 {
		r.u(1)
	}
	r.ue() // width
	r.ue() // height
	if r.bit() == 1 {
		for i := 0; i < 4; i++ {
			r.ue()
		}
	}
	r.ue() // luma depth
	r.ue() // chroma depth
	pocBits := r.ue() + 4
	first := maxSub
	if r.bit() == 1 {
		first = 0
	}
	for i := first; i <= maxSub; i++ {
		r.ue()
		r.ue()
		r.ue()
	}
	for i := 0; i < 6; i++ { // block size limits
		r.ue()
	}
	scalingOn := r.bit() == 1
	if scalingOn && r.bit() == 1 { // scaling lists, and their data
		for size := 0; size < 4 && !r.bad; size++ {
			step := 1
			if size == 3 {
				step = 3
			}
			for m := 0; m < 6; m += step {
				if r.bit() == 0 {
					r.ue()
					continue
				}
				coefs := min(64, 1<<(4+size<<1))
				if size > 1 {
					r.se()
				}
				for i := 0; i < coefs; i++ {
					r.se()
				}
			}
		}
	}
	r.u(2)            // amp, sample adaptive offset
	if r.bit() == 1 { // pcm
		r.u(8)
		r.ue()
		r.ue()
		r.u(1)
	}
	sets := r.ue()
	if sets > 64 || r.bad {
		return 0
	}
	deltaPocs := make([]int, sets)
	for i := 0; i < sets && !r.bad; i++ {
		inter := i != 0 && r.bit() == 1
		if inter {
			r.u(1)
			r.ue()
			n := 0
			for j := 0; j <= deltaPocs[i-1]; j++ {
				used := r.bit() == 1
				if used || r.bit() == 1 {
					n++
				}
			}
			deltaPocs[i] = n
			continue
		}
		neg, pos := r.ue(), r.ue()
		if neg > 16 || pos > 16 {
			return 0
		}
		for j := 0; j < neg+pos; j++ {
			r.ue()
			r.u(1)
		}
		deltaPocs[i] = neg + pos
	}
	if r.bit() == 1 { // long-term pictures
		n := r.ue()
		if n > 32 {
			return 0
		}
		for i := 0; i < n; i++ {
			r.u(pocBits)
			r.u(1)
		}
	}
	r.u(2) // temporal MVP, strong intra smoothing
	if r.bit() == 0 || r.bad {
		return 0 // no VUI
	}
	if r.bit() == 1 { // aspect ratio
		if r.u(8) == 255 {
			r.u(32)
		}
	}
	if r.bit() == 1 { // overscan
		r.u(1)
	}
	if r.bit() == 0 { // video signal type
		return 0
	}
	r.u(4) // format, full range
	if r.bit() == 0 {
		return 0
	}
	r.u(8) // primaries
	t := r.u(8)
	if r.bad {
		return 0
	}
	return t
}
