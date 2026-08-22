package dds

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"image/png"
	"testing"
)

// TestDecodeBC7RealBlock pins the exact regression this package was built against: a real
// 16-byte BC7 block (Mode 4, from a Zombie Army 4 texture archive) whose correct decoded
// value was independently confirmed against the public bcdec.h reference decoder
// (github.com/iOrange/bcdec). If this ever breaks, the BC7 decoder itself is wrong.
func TestDecodeBC7RealBlock(t *testing.T) {
	block, err := hex.DecodeString("1065985146b676e949a394c620ddc6ff")
	if err != nil {
		t.Fatal(err)
	}
	got := decodeBC7Block(block)
	want := [4]uint8{35, 41, 35, 122}
	if got[0] != want {
		t.Fatalf("pixel(0,0) = %v, want %v", got[0], want)
	}
}

// TestNRGBANotPremultiplied is a regression test for the bug that motivated this package's
// design: Decode must return *image.NRGBA (straight alpha), not *image.RGBA (premultiplied).
// Using image.RGBA here would make image/png's encoder silently divide color channels by
// alpha on write — invisible when round-tripping through Go's own png package (which
// re-multiplies symmetrically on read) but wrong in every standards-compliant PNG reader.
// This test catches that class of bug by actually encoding to PNG and decoding with the
// stdlib png package, checking the alpha-affected color channels survive exactly.
func TestNRGBANotPremultiplied(t *testing.T) {
	// A 4x4, uncompressed 32-bit RGBA DDS: one solid color with alpha well below 255, which
	// is exactly the condition that exposes premultiplication bugs (at alpha=255, straight
	// and premultiplied values are identical and the bug is invisible).
	data := buildUncompressedDDS(t, 4, 4, [4]uint8{100, 150, 200, 128})

	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if c := img.NRGBAAt(0, 0); c.R != 100 || c.G != 150 || c.B != 200 || c.A != 128 {
		t.Fatalf("Decode result = %v, want {100 150 200 128}", c)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	back, err := png.Decode(&buf)
	if err != nil {
		t.Fatalf("png.Decode: %v", err)
	}
	r, g, b, a := back.At(0, 0).RGBA()
	// image/color.Color.RGBA() always returns premultiplied 16-bit values, so undo that
	// before comparing against the original straight 8-bit input.
	gotR, gotG, gotB, gotA := unpremultiply16(r, g, b, a)
	if gotR != 100 || gotG != 150 || gotB != 200 || gotA != 128 {
		t.Fatalf("round-tripped through PNG = {%d %d %d %d}, want {100 150 200 128}", gotR, gotG, gotB, gotA)
	}
}

func unpremultiply16(r, g, b, a uint32) (uint8, uint8, uint8, uint8) {
	if a == 0 {
		return 0, 0, 0, 0
	}
	un := func(c uint32) uint8 { return uint8((c * 0xffff / a) >> 8) }
	return un(r), un(g), un(b), uint8(a >> 8)
}

func TestDecodeBC1Block(t *testing.T) {
	// Two distinct RGB565 endpoints, c0 > c1 (standard 4-color mode), all 16 texels using
	// index 0 (should equal endpoint 0 exactly).
	var block [8]byte
	binary.LittleEndian.PutUint16(block[0:2], 0xF800) // pure red, c0
	binary.LittleEndian.PutUint16(block[2:4], 0x001F) // pure blue, c1 (c0 > c1 numerically)
	// indices already zero -> every texel uses color 0

	pixels := decodeBC1Block(block[:])
	for i, p := range pixels {
		if p[0] != 255 || p[1] != 0 || p[2] != 0 || p[3] != 255 {
			t.Fatalf("pixel %d = %v, want opaque red", i, p)
		}
	}
}

func TestDecodeBC4Block(t *testing.T) {
	block := []byte{200, 100, 0, 0, 0, 0, 0, 0} // a0=200 > a1=100 -> 6-value interpolation; all indices 0
	pixels := decodeBC4Block(block)
	for i, p := range pixels {
		if p[0] != 200 || p[1] != 200 || p[2] != 200 || p[3] != 255 {
			t.Fatalf("pixel %d = %v, want {200 200 200 255}", i, p)
		}
	}
}

func TestDecodeBC5Block(t *testing.T) {
	block := make([]byte, 16)
	block[0], block[1] = 200, 100 // R channel block
	block[8], block[9] = 50, 10   // G channel block
	pixels := decodeBC5Block(block)
	if pixels[0][0] != 200 || pixels[0][1] != 50 || pixels[0][2] != 0 || pixels[0][3] != 255 {
		t.Fatalf("pixel 0 = %v, want {200 50 0 255}", pixels[0])
	}
}

func TestDecodeBadMagic(t *testing.T) {
	if _, err := Decode([]byte("not a dds file")); err == nil {
		t.Fatal("expected an error for missing DDS magic")
	}
}

func TestDecodeUnsupportedFormat(t *testing.T) {
	data := buildDDSHeader(t, 4, 4, "XXXX", 0)
	if _, err := Decode(data); err == nil {
		t.Fatal("expected an error for an unrecognized FourCC")
	}
}

// buildDDSHeader constructs a minimal 4+124-byte DDS header (plus a 20-byte DX10 extension
// when fourCC == "DX10"), with no pixel data appended.
func buildDDSHeader(t *testing.T, width, height uint32, fourCC string, dxgiFormat uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("DDS ")
	u32 := func(v uint32) { binary.Write(&buf, binary.LittleEndian, v) }
	u32(124)                    // dwSize
	u32(0x00081007)             // dwFlags (CAPS|HEIGHT|WIDTH|PIXELFORMAT)
	u32(height)                 // dwHeight
	u32(width)                  // dwWidth
	u32(0)                      // dwPitchOrLinearSize
	u32(0)                      // dwDepth
	u32(1)                      // dwMipMapCount
	buf.Write(make([]byte, 44)) // dwReserved1[11]

	// DDS_PIXELFORMAT (32 bytes)
	u32(32) // size
	if fourCC == "" {
		u32(0x40) // DDPF_RGB
		buf.Write(make([]byte, 4))
		u32(32)         // RGBBitCount
		u32(0x00ff0000) // R mask (BGRA-style, matches common uncompressed DDS layout)
		u32(0x0000ff00) // G mask
		u32(0x000000ff) // B mask
		u32(0xff000000) // A mask
	} else {
		u32(0x4) // DDPF_FOURCC
		fc := fourCC
		if len(fc) > 4 {
			fc = fc[:4]
		}
		buf.WriteString(fc)
		for len(fc) < 4 {
			buf.WriteByte(0)
			fc += "0"
		}
		u32(0) // RGBBitCount
		u32(0) // R mask
		u32(0) // G mask
		u32(0) // B mask
		u32(0) // A mask
	}
	u32(0x1000) // dwCaps (DDSCAPS_TEXTURE)
	u32(0)      // dwCaps2
	u32(0)      // dwCaps3
	u32(0)      // dwCaps4
	u32(0)      // dwReserved2

	if fourCC == "DX10" {
		u32(dxgiFormat)
		u32(3) // D3D10_RESOURCE_DIMENSION_TEXTURE2D
		u32(0) // miscFlag
		u32(1) // arraySize
		u32(0) // miscFlags2
	}
	return buf.Bytes()
}

// buildUncompressedDDS builds a full DDS file (header + pixel data) for a solid-color
// width x height 32-bit uncompressed RGBA image, using the R/G/B/A masks set up by
// buildDDSHeader for the no-FourCC (DDPF_RGB) case.
func buildUncompressedDDS(t *testing.T, width, height uint32, color [4]uint8) []byte {
	t.Helper()
	data := buildDDSHeader(t, width, height, "", 0)
	px := uint32(color[2]) | uint32(color[1])<<8 | uint32(color[0])<<16 | uint32(color[3])<<24
	for i := uint32(0); i < width*height; i++ {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], px)
		data = append(data, b[:]...)
	}
	return data
}
