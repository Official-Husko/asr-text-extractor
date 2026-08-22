package dds

import "encoding/binary"

// unpack565 splits a packed RGB565 value into its raw (unexpanded) 5/6/5-bit components.
func unpack565(c uint16) (r5, g6, b5 uint32) {
	return uint32(c>>11) & 0x1F, uint32(c>>5) & 0x3F, uint32(c) & 0x1F
}

// expand565 widens raw 5/6/5-bit components to 8-bit using the same fixed-point formula
// hardware BC1 decoders use, so the result matches bit-for-bit rather than approximating.
func expand565(r5, g6, b5 uint32) (r, g, b uint8) {
	return uint8((r5*527 + 23) >> 6), uint8((g6*259 + 33) >> 6), uint8((b5*527 + 23) >> 6)
}

// decodeBC1ColorBlock decodes an 8-byte BC1-style color block (also used, with
// forceOpaque=true, for BC3's color half) into its 4-entry RGBA palette.
func decodeBC1ColorBlock(block []byte, forceOpaque bool) [4][4]uint8 {
	c0 := binary.LittleEndian.Uint16(block[0:2])
	c1 := binary.LittleEndian.Uint16(block[2:4])
	r0, g0, b0 := unpack565(c0)
	r1, g1, b1 := unpack565(c1)

	var colors [4][4]uint8
	cr0, cg0, cb0 := expand565(r0, g0, b0)
	cr1, cg1, cb1 := expand565(r1, g1, b1)
	colors[0] = [4]uint8{cr0, cg0, cb0, 255}
	colors[1] = [4]uint8{cr1, cg1, cb1, 255}

	if c0 > c1 || forceOpaque {
		r := (2*r0+r1)*351 + 61
		g := (2*g0+g1)*2763 + 1039
		b := (2*b0+b1)*351 + 61
		colors[2] = [4]uint8{uint8(r >> 7), uint8(g >> 11), uint8(b >> 7), 255}

		r = (r0+2*r1)*351 + 61
		g = (g0+2*g1)*2763 + 1039
		b = (b0+2*b1)*351 + 61
		colors[3] = [4]uint8{uint8(r >> 7), uint8(g >> 11), uint8(b >> 7), 255}
	} else {
		r := (r0+r1)*1053 + 125
		g := (g0+g1)*4145 + 1019
		b := (b0+b1)*1053 + 125
		colors[2] = [4]uint8{uint8(r >> 8), uint8(g >> 11), uint8(b >> 8), 255}
		colors[3] = [4]uint8{0, 0, 0, 0}
	}
	return colors
}

// decodeBC1Block decodes a full 8-byte BC1 block into 16 RGBA8 texels, row-major (index
// y*4+x).
func decodeBC1Block(block []byte) (pixels [16][4]uint8) {
	colors := decodeBC1ColorBlock(block, false)
	idx := binary.LittleEndian.Uint32(block[4:8])
	for i := range pixels {
		pixels[i] = colors[idx&0x3]
		idx >>= 2
	}
	return
}

// decodeSmoothAlphaBlock decodes an 8-byte BC3/BC4/BC5-style interpolated single-channel
// block (2 reference values + 16x3-bit indices) into 16 channel values, row-major.
func decodeSmoothAlphaBlock(block []byte) (out [16]uint8) {
	raw := binary.LittleEndian.Uint64(block[:8])
	a0 := uint8(raw)
	a1 := uint8(raw >> 8)

	var alpha [8]uint8
	alpha[0], alpha[1] = a0, a1
	if a0 > a1 {
		alpha[2] = uint8((6*uint16(a0) + uint16(a1)) / 7)
		alpha[3] = uint8((5*uint16(a0) + 2*uint16(a1)) / 7)
		alpha[4] = uint8((4*uint16(a0) + 3*uint16(a1)) / 7)
		alpha[5] = uint8((3*uint16(a0) + 4*uint16(a1)) / 7)
		alpha[6] = uint8((2*uint16(a0) + 5*uint16(a1)) / 7)
		alpha[7] = uint8((uint16(a0) + 6*uint16(a1)) / 7)
	} else {
		alpha[2] = uint8((4*uint16(a0) + uint16(a1)) / 5)
		alpha[3] = uint8((3*uint16(a0) + 2*uint16(a1)) / 5)
		alpha[4] = uint8((2*uint16(a0) + 3*uint16(a1)) / 5)
		alpha[5] = uint8((uint16(a0) + 4*uint16(a1)) / 5)
		alpha[6] = 0
		alpha[7] = 255
	}

	indices := raw >> 16
	for i := range out {
		out[i] = alpha[indices&0x7]
		indices >>= 3
	}
	return
}

// decodeBC3Block decodes a full 16-byte BC3 block: 8 bytes of interpolated alpha, then 8
// bytes of BC1-style (always 4-color) RGB.
func decodeBC3Block(block []byte) (pixels [16][4]uint8) {
	alpha := decodeSmoothAlphaBlock(block[:8])
	colors := decodeBC1ColorBlock(block[8:16], true)
	idx := binary.LittleEndian.Uint32(block[12:16])
	for i := range pixels {
		c := colors[idx&0x3]
		pixels[i] = [4]uint8{c[0], c[1], c[2], alpha[i]}
		idx >>= 2
	}
	return
}

// decodeBC4Block decodes an 8-byte BC4 block into a single-channel 4x4 image, replicated
// into R/G/B with alpha opaque (BC4 is a single-channel format; callers needing raw single
// channel data should use decodeSmoothAlphaBlock directly).
func decodeBC4Block(block []byte) (pixels [16][4]uint8) {
	values := decodeSmoothAlphaBlock(block)
	for i, v := range values {
		pixels[i] = [4]uint8{v, v, v, 255}
	}
	return
}

// decodeBC5Block decodes a 16-byte BC5 block (two independent BC4-style channels, packed
// into R and G) into 4x4 RGBA texels with B=0 and A=255 — the raw two-channel form BC5 data
// actually has, not a reconstructed normal (reconstructing Z = sqrt(1-x^2-y^2) would assume
// every BC5 texture is a tangent-space normal map, which isn't guaranteed).
func decodeBC5Block(block []byte) (pixels [16][4]uint8) {
	r := decodeSmoothAlphaBlock(block[0:8])
	g := decodeSmoothAlphaBlock(block[8:16])
	for i := range pixels {
		pixels[i] = [4]uint8{r[i], g[i], 0, 255}
	}
	return
}
