package asura

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
)

// zbbMagic is the 8-byte signature for the "AsuraZbb" compression wrapper used by Asura
// Engine level-package files (.pc, .pc_entdata) — distinct from the plain "Asura   " magic
// every other chunk type in this package uses.
var zbbMagic = [8]byte{'A', 's', 'u', 'r', 'a', 'Z', 'b', 'b'}

// CheckZbbMagic reports whether data begins with the "AsuraZbb" compression-wrapper signature.
func CheckZbbMagic(data []byte) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], zbbMagic[:])
}

// DecompressZbb decompresses an "AsuraZbb"-wrapped file to its full uncompressed content.
//
// Layout: the 8-byte magic, a uint32 (unused here — equal to len(data)-16, i.e. "bytes
// remaining after this field"), a uint32 giving the total decompressed size, then repeating
// [compressedSize uint32][decompressedSize uint32][zlib-compressed bytes] chunks — each
// chunk decompresses to 2MB except the last, which is however many bytes remain. Confirmed
// against a real 255MB Zombie Army 4 level package: 226 chunks, declared total decompressed
// size matches the sum of every chunk's own decompressed size exactly, and walking the chunk
// table lands precisely on the end of the file.
func DecompressZbb(data []byte) ([]byte, error) {
	if !CheckZbbMagic(data) {
		return nil, fmt.Errorf("asura: missing \"AsuraZbb\" magic")
	}
	if len(data) < 16 {
		return nil, fmt.Errorf("asura: truncated AsuraZbb header")
	}
	totalSize := binary.LittleEndian.Uint32(data[12:16])

	out := make([]byte, 0, totalSize)
	pos := 16
	for pos < len(data) {
		if len(data)-pos < 8 {
			return nil, fmt.Errorf("asura: truncated chunk header at offset %d", pos)
		}
		compressedSize := binary.LittleEndian.Uint32(data[pos : pos+4])
		decompressedSize := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		pos += 8
		if pos+int(compressedSize) > len(data) {
			return nil, fmt.Errorf("asura: chunk at offset %d: declared compressed size %d exceeds remaining data", pos-8, compressedSize)
		}

		zr, err := zlib.NewReader(bytes.NewReader(data[pos : pos+int(compressedSize)]))
		if err != nil {
			return nil, fmt.Errorf("asura: chunk at offset %d: %w", pos-8, err)
		}
		chunk := make([]byte, decompressedSize)
		if _, err := io.ReadFull(zr, chunk); err != nil {
			return nil, fmt.Errorf("asura: chunk at offset %d: decompressing: %w", pos-8, err)
		}
		zr.Close()

		out = append(out, chunk...)
		pos += int(compressedSize)
	}

	if uint32(len(out)) != totalSize {
		return nil, fmt.Errorf("asura: decompressed %d bytes, header declared %d", len(out), totalSize)
	}
	return out, nil
}
