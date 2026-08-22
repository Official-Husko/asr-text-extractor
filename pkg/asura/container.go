// Package asura implements the Asura Engine's container/chunk file format used by
// Rebellion's Asura-engine titles (Sniper Elite 4 and others). It currently understands
// the HTXT (menu/UI text) and DLLN (voice line) chunk types; future asset types
// (textures, models, sounds) should live alongside these as new files in this package,
// reusing the container primitives defined here.
package asura

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// Magic is the 8-byte "Asura   " (3 trailing spaces) signature every Asura container starts with.
var Magic = [8]byte{'A', 's', 'u', 'r', 'a', ' ', ' ', ' '}

// ErrBadMagic is returned when data does not start with the Asura container signature.
var ErrBadMagic = errors.New("asura: missing \"Asura\" container signature")

// CheckMagic reports whether data begins with the Asura container signature.
func CheckMagic(data []byte) bool {
	return len(data) >= 8 && bytes.Equal(data[:8], Magic[:])
}

// reader is a minimal sticky-error, little-endian cursor over an in-memory byte slice.
// It mirrors the sequential-read style of the original BinaryReader-based C# tool: once
// r.err is set, every subsequent read is a no-op returning the zero value, so callers can
// perform a run of reads and check r.err once at a natural checkpoint.
type reader struct {
	data []byte
	pos  int
	err  error
}

func (r *reader) remaining() int {
	return len(r.data) - r.pos
}

func (r *reader) bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.remaining() {
		r.err = errUnexpectedEOF
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *reader) byte() byte {
	b := r.bytes(1)
	if r.err != nil {
		return 0
	}
	return b[0]
}

func (r *reader) u32() uint32 {
	b := r.bytes(4)
	if r.err != nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

var errUnexpectedEOF = errors.New("asura: unexpected end of file")

func writeU32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// alignedString reads a NUL-terminated ASCII string starting at data[pos:], consuming input
// in fixed 4-byte chunks (counted from pos itself, not from any absolute file position) until
// a chunk contains the terminator — the padded string encoding RSCF and RSFL manifest entries
// both use for their path fields. Matches the "readAlignedString" helper found in independent
// community Asura-format reference decoders (unpack_rebellion.py, tools_ZA4.py), which this
// project's own understanding of RSFL's path padding was cross-checked against.
func alignedString(data []byte, pos int) (s string, next int, ok bool) {
	start := pos
	for {
		if pos+4 > len(data) {
			return "", 0, false
		}
		chunk := data[pos : pos+4]
		pos += 4
		if idx := bytes.IndexByte(chunk, 0); idx >= 0 {
			return string(data[start : pos-4+idx]), pos, true
		}
	}
}
