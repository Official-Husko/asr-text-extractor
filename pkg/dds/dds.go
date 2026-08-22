// Package dds decodes DirectDraw Surface (DDS) texture files — a public, well-documented
// format (unlike the rest of this project, which reverse-engineers undocumented Asura game
// formats). It supports the block-compressed formats actually seen in Asura Engine texture
// archives (BC1/BC3/BC4/BC5/BC7, both legacy FourCC and DX10-extended headers) plus simple
// 32-bit uncompressed RGB/RGBA, and decodes only the base (largest) mip level — the level a
// modding workflow actually needs; everything finer is something the target tool regenerates.
package dds

import (
	"encoding/binary"
	"fmt"
	"image"
)

const (
	ddpfFourCC = 0x4
	ddpfRGB    = 0x40
)

// Decode parses a DDS file (starting with its own "DDS " magic — the byte immediately after
// an Asura RSCF texture entry's own framing) and decodes its base mip level to an *image.NRGBA.
//
// This must be image.NRGBA, not image.RGBA: Go's image.RGBA holds alpha-*premultiplied*
// values, but decoded texture data (BC7 endpoints, uncompressed pixels, ...) is straight,
// unpremultiplied color — and for many of these textures alpha isn't even transparency at
// all (e.g. an "_ar" albedo+roughness texture packs roughness into the alpha channel). Using
// image.RGBA here would make image/png's encoder "correctly" un-premultiply on write,
// silently corrupting every pixel whose alpha isn't 255 — invisible if you only ever
// round-trip through Go's own encoder/decoder (which re-premultiplies symmetrically on read),
// but wrong in any standards-compliant PNG reader (confirmed against three independent
// decoders while tracking this down).
func Decode(data []byte) (*image.NRGBA, error) {
	if len(data) < 4 || string(data[:4]) != "DDS " {
		return nil, fmt.Errorf("dds: missing \"DDS \" magic")
	}
	if len(data) < 4+124 {
		return nil, fmt.Errorf("dds: truncated header")
	}
	hdr := data[4 : 4+124]
	height := binary.LittleEndian.Uint32(hdr[8:12])
	width := binary.LittleEndian.Uint32(hdr[12:16])
	pfFlags := binary.LittleEndian.Uint32(hdr[76:80])
	fourCC := hdr[80:84]

	pos := 4 + 124
	var dxgiFormat uint32
	isDX10 := string(fourCC) == "DX10"
	if isDX10 {
		if len(data) < pos+20 {
			return nil, fmt.Errorf("dds: truncated DX10 header extension")
		}
		dxgiFormat = binary.LittleEndian.Uint32(data[pos : pos+4])
		pos += 20
	}

	decode, blockBytes, err := blockDecoderFor(pfFlags, fourCC, isDX10, dxgiFormat)
	if err == nil {
		return decodeBlockCompressed(data[pos:], int(width), int(height), decode, blockBytes)
	}
	if pfFlags&ddpfRGB != 0 {
		return decodeUncompressed(data[pos:], hdr, int(width), int(height))
	}
	return nil, err
}

type blockDecodeFunc func(block []byte) [16][4]uint8

// blockDecoderFor picks the right 4x4-block decoder and its block byte size, covering both
// the legacy FourCC-tagged formats and the newer DX10-extended dxgiFormat values. BC2 and
// BC6H aren't implemented (not seen in any sample archive so far) and report a clear error
// naming the unsupported format rather than silently producing garbage.
func blockDecoderFor(pfFlags uint32, fourCC []byte, isDX10 bool, dxgiFormat uint32) (blockDecodeFunc, int, error) {
	if isDX10 {
		switch dxgiFormat {
		case 70, 71, 72: // BC1_TYPELESS, BC1_UNORM, BC1_UNORM_SRGB
			return decodeBC1Block, 8, nil
		case 76, 77, 78: // BC3_TYPELESS, BC3_UNORM, BC3_UNORM_SRGB
			return decodeBC3Block, 16, nil
		case 79, 80, 81: // BC4_TYPELESS, BC4_UNORM, BC4_SNORM
			return decodeBC4Block, 8, nil
		case 82, 83, 84: // BC5_TYPELESS, BC5_UNORM, BC5_SNORM
			return decodeBC5Block, 16, nil
		case 97, 98, 99, 100: // BC7_TYPELESS, BC7_UNORM, BC7_UNORM_SRGB (100 seen in practice)
			return decodeBC7Block, 16, nil
		default:
			return nil, 0, fmt.Errorf("dds: unsupported DXGI format %d", dxgiFormat)
		}
	}
	if pfFlags&ddpfFourCC != 0 {
		switch string(fourCC) {
		case "DXT1":
			return decodeBC1Block, 8, nil
		case "DXT4", "DXT5":
			return decodeBC3Block, 16, nil
		case "ATI1", "BC4U", "BC4S":
			return decodeBC4Block, 8, nil
		case "ATI2", "BC5U", "BC5S":
			return decodeBC5Block, 16, nil
		default:
			return nil, 0, fmt.Errorf("dds: unsupported FourCC %q", fourCC)
		}
	}
	return nil, 0, fmt.Errorf("dds: no recognized compressed pixel format")
}

func decodeBlockCompressed(data []byte, width, height int, decode blockDecodeFunc, blockBytes int) (*image.NRGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("dds: invalid dimensions %dx%d", width, height)
	}
	const blockDim = 4
	blocksWide := (width + blockDim - 1) / blockDim
	blocksHigh := (height + blockDim - 1) / blockDim
	need := blocksWide * blocksHigh * blockBytes
	if len(data) < need {
		return nil, fmt.Errorf("dds: truncated pixel data: need %d bytes, have %d", need, len(data))
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	pos := 0
	for by := 0; by < blocksHigh; by++ {
		for bx := 0; bx < blocksWide; bx++ {
			pixels := decode(data[pos : pos+blockBytes])
			pos += blockBytes
			for ty := 0; ty < blockDim; ty++ {
				py := by*blockDim + ty
				if py >= height {
					continue
				}
				for tx := 0; tx < blockDim; tx++ {
					px := bx*blockDim + tx
					if px >= width {
						continue
					}
					c := pixels[ty*4+tx]
					o := img.PixOffset(px, py)
					img.Pix[o+0], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c[0], c[1], c[2], c[3]
				}
			}
		}
	}
	return img, nil
}

// decodeUncompressed handles the common case of plain 32-bit-per-pixel RGB/RGBA data
// (DDPF_RGB, no FourCC), using the header's own component bit masks so it doesn't matter
// whether the channel order is RGBA, BGRA, or something else.
func decodeUncompressed(data []byte, hdr []byte, width, height int) (*image.NRGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("dds: invalid dimensions %dx%d", width, height)
	}
	rgbBitCount := binary.LittleEndian.Uint32(hdr[84:88])
	if rgbBitCount != 32 {
		return nil, fmt.Errorf("dds: unsupported uncompressed bit depth %d (only 32-bit is supported)", rgbBitCount)
	}
	rMask := binary.LittleEndian.Uint32(hdr[88:92])
	gMask := binary.LittleEndian.Uint32(hdr[92:96])
	bMask := binary.LittleEndian.Uint32(hdr[96:100])
	aMask := binary.LittleEndian.Uint32(hdr[100:104])

	need := width * height * 4
	if len(data) < need {
		return nil, fmt.Errorf("dds: truncated pixel data: need %d bytes, have %d", need, len(data))
	}

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	pos := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			px := binary.LittleEndian.Uint32(data[pos : pos+4])
			pos += 4
			o := img.PixOffset(x, y)
			img.Pix[o+0] = extractChannel(px, rMask)
			img.Pix[o+1] = extractChannel(px, gMask)
			img.Pix[o+2] = extractChannel(px, bMask)
			if aMask != 0 {
				img.Pix[o+3] = extractChannel(px, aMask)
			} else {
				img.Pix[o+3] = 255
			}
		}
	}
	return img, nil
}

func extractChannel(px, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := 0
	for mask&1 == 0 {
		mask >>= 1
		shift++
	}
	return uint8((px >> uint(shift)) & mask)
}
