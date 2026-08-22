package dds

import "encoding/binary"

// Per-mode field widths, straight from the BC7 spec (bc7Partitions2/3 tables live in
// bc7_tables.go). bc7RGBABits/bc7AlphaBits are the raw stored precision per color/alpha
// component (before any P-bit is folded in); bc7HasPBits has bit i set when mode i carries a
// P-bit (mode 1's is shared between two endpoints, everything else's is one per endpoint —
// see the mode==1 special case below).
var (
	bc7RGBABits  = [8]int{4, 6, 5, 7, 5, 7, 7, 5}
	bc7AlphaBits = [8]int{0, 0, 0, 0, 6, 8, 7, 5}
	bc7HasPBits  = uint8(0b11001011)

	bc7Weight2 = [4]int{0, 21, 43, 64}
	bc7Weight3 = [8]int{0, 9, 18, 27, 37, 46, 55, 64}
	bc7Weight4 = [16]int{0, 4, 9, 13, 17, 21, 26, 30, 34, 38, 43, 47, 51, 55, 60, 64}
)

// bc7BitReader pulls bits LSB-first from a 128-bit BC7 block, matching the format's own
// bitstream convention (mode bits are the very first, least-significant bits of the block).
type bc7BitReader struct {
	lo, hi uint64
}

func newBC7BitReader(block []byte) *bc7BitReader {
	return &bc7BitReader{
		lo: binary.LittleEndian.Uint64(block[0:8]),
		hi: binary.LittleEndian.Uint64(block[8:16]),
	}
}

func (r *bc7BitReader) readBits(n int) int {
	if n == 0 {
		return 0
	}
	mask := uint64(1)<<uint(n) - 1
	bits := r.lo & mask
	r.lo = (r.lo >> uint(n)) | ((r.hi & mask) << uint(64-n))
	r.hi >>= uint(n)
	return int(bits)
}

func (r *bc7BitReader) readBit() int {
	return r.readBits(1)
}

func bc7Interpolate(a, b int, weights []int, index int) uint8 {
	return uint8((a*(64-weights[index]) + b*weights[index] + 32) >> 6)
}

func bc7WeightsFor(bits int) []int {
	switch bits {
	case 2:
		return bc7Weight2[:]
	case 3:
		return bc7Weight3[:]
	default:
		return bc7Weight4[:]
	}
}

// decodeBC7Block decodes a full 16-byte BC7 block into 16 RGBA8 texels, row-major (index
// y*4+x). Ported from the public BC7 specification (Microsoft Learn's "BC7 Format" and "BC7
// Format Mode Reference" pages) and cross-checked bit-for-bit against the independent,
// MIT-licensed reference decoder at github.com/iOrange/bcdec.
func decodeBC7Block(block []byte) (pixels [16][4]uint8) {
	r := newBC7BitReader(block)

	mode := 8
	for m := 0; m < 8; m++ {
		if r.readBit() == 1 {
			mode = m
			break
		}
	}
	if mode >= 8 {
		return // reserved mode 8: decodes to transparent black
	}

	partition := 0
	numPartitions := 1
	rotation := 0
	indexSelectionBit := 0

	if mode == 0 || mode == 1 || mode == 2 || mode == 3 || mode == 7 {
		if mode == 0 || mode == 2 {
			numPartitions = 3
		} else {
			numPartitions = 2
		}
		bits := 6
		if mode == 0 {
			bits = 4
		}
		partition = r.readBits(bits)
	}
	numEndpoints := numPartitions * 2

	if mode == 4 || mode == 5 {
		rotation = r.readBits(2)
		if mode == 4 {
			indexSelectionBit = r.readBit()
		}
	}

	var endpoints [6][4]int
	rgbBits := bc7RGBABits[mode]
	for c := 0; c < 3; c++ {
		for e := 0; e < numEndpoints; e++ {
			endpoints[e][c] = r.readBits(rgbBits)
		}
	}
	alphaBits := bc7AlphaBits[mode]
	if alphaBits > 0 {
		for e := 0; e < numEndpoints; e++ {
			endpoints[e][3] = r.readBits(alphaBits)
		}
	}

	hasPBit := bc7HasPBits&(1<<uint(mode)) != 0
	if mode == 0 || mode == 1 || mode == 3 || mode == 6 || mode == 7 {
		for e := 0; e < numEndpoints; e++ {
			for c := 0; c < 4; c++ {
				endpoints[e][c] <<= 1
			}
		}
		if mode == 1 {
			// Shared P-bit: one bit each for subset 0 and subset 1, applied to both of
			// that subset's endpoints.
			p0 := r.readBit()
			p1 := r.readBit()
			for c := 0; c < 3; c++ {
				endpoints[0][c] |= p0
				endpoints[1][c] |= p0
				endpoints[2][c] |= p1
				endpoints[3][c] |= p1
			}
		} else if hasPBit {
			for e := 0; e < numEndpoints; e++ {
				p := r.readBit()
				for c := 0; c < 4; c++ {
					endpoints[e][c] |= p
				}
			}
		}
	}

	pBitExtra := 0
	if hasPBit {
		pBitExtra = 1
	}
	colorPrec := rgbBits + pBitExtra
	alphaPrec := alphaBits + pBitExtra
	for e := 0; e < numEndpoints; e++ {
		for c := 0; c < 3; c++ {
			v := endpoints[e][c] << uint(8-colorPrec)
			endpoints[e][c] = v | (v >> uint(colorPrec))
		}
		if alphaBits > 0 {
			v := endpoints[e][3] << uint(8-alphaPrec)
			endpoints[e][3] = v | (v >> uint(alphaPrec))
		}
	}
	if alphaBits == 0 {
		for e := 0; e < numEndpoints; e++ {
			endpoints[e][3] = 255
		}
	}

	indexBits := 2
	switch mode {
	case 0, 1:
		indexBits = 3
	case 6:
		indexBits = 4
	}
	indexBits2 := 0
	switch mode {
	case 4:
		indexBits2 = 3
	case 5:
		indexBits2 = 2
	}
	primaryWeights := bc7WeightsFor(indexBits)
	var secondaryWeights []int
	if indexBits2 > 0 {
		secondaryWeights = bc7WeightsFor(indexBits2)
	}

	var subsetOf [4][4]int
	var colorIdx [4][4]int
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			var ps int
			if numPartitions == 1 {
				if x == 0 && y == 0 {
					ps = 128
				}
			} else if numPartitions == 2 {
				ps = int(bc7Partitions2[partition][y][x])
			} else {
				ps = int(bc7Partitions3[partition][y][x])
			}
			subsetOf[y][x] = ps

			bits := indexBits
			if ps&0x80 != 0 {
				bits-- // the fix-up index is stored with one fewer bit (implicit MSB 0)
			}
			colorIdx[y][x] = r.readBits(bits)
		}
	}

	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			subset := subsetOf[y][x] & 0x03
			e0, e1 := endpoints[subset*2], endpoints[subset*2+1]
			index := colorIdx[y][x]

			var rr, gg, bb, aa uint8
			if indexBits2 == 0 {
				rr = bc7Interpolate(e0[0], e1[0], primaryWeights, index)
				gg = bc7Interpolate(e0[1], e1[1], primaryWeights, index)
				bb = bc7Interpolate(e0[2], e1[2], primaryWeights, index)
				aa = bc7Interpolate(e0[3], e1[3], primaryWeights, index)
			} else {
				bits2 := indexBits2
				if x == 0 && y == 0 {
					bits2--
				}
				index2 := r.readBits(bits2)
				if indexSelectionBit == 0 {
					rr = bc7Interpolate(e0[0], e1[0], primaryWeights, index)
					gg = bc7Interpolate(e0[1], e1[1], primaryWeights, index)
					bb = bc7Interpolate(e0[2], e1[2], primaryWeights, index)
					aa = bc7Interpolate(e0[3], e1[3], secondaryWeights, index2)
				} else {
					rr = bc7Interpolate(e0[0], e1[0], secondaryWeights, index2)
					gg = bc7Interpolate(e0[1], e1[1], secondaryWeights, index2)
					bb = bc7Interpolate(e0[2], e1[2], secondaryWeights, index2)
					aa = bc7Interpolate(e0[3], e1[3], primaryWeights, index)
				}
			}

			switch rotation {
			case 1:
				rr, aa = aa, rr
			case 2:
				gg, aa = aa, gg
			case 3:
				bb, aa = aa, bb
			}
			pixels[y*4+x] = [4]uint8{rr, gg, bb, aa}
		}
	}
	return
}
