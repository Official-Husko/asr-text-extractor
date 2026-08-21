package asura

import (
	"bytes"
	"fmt"
)

// SoundEntry is a single embedded audio asset referenced by an ASTS manifest: the game's own
// asset path (backslash-separated, e.g. `Sounds\Cutscenes\Flix\...\Intro_SFX.wav`) and its
// raw bytes, sliced directly from the source file — a complete, valid RIFF/WAVE file on its
// own (confirmed against real Zombie Army 4 data: correct RIFF size, fmt/data/smpl subchunks).
type SoundEntry struct {
	Path string
	Data []byte
}

// ASTSFile is a parsed ASTS chunk: header fields and its manifest of embedded sound assets.
type ASTSFile struct {
	Filesize uint32
	Version  uint32
	Reserved uint32
	Entries  []SoundEntry
}

// ParseASTS decodes a single ASTS chunk from data, which must start with the 8-byte Asura
// magic immediately followed by the "ASTS" chunk. Each manifest entry is a NUL-terminated
// ASCII asset path, then zero-padding up to the next non-zero byte, then a uint32 byte size
// and a uint32 absolute file offset — the audio bytes themselves are appended back-to-back
// immediately after the manifest, in entry order, ending in a 4-byte zero footer.
func ParseASTS(data []byte) (*ASTSFile, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	r := &reader{data: data, pos: 8}
	tag := r.bytes(4)
	if r.err != nil {
		return nil, r.err
	}
	if string(tag) != "ASTS" {
		return nil, fmt.Errorf("asura: expected ASTS chunk, found %q", tag)
	}

	f := &ASTSFile{
		Filesize: r.u32(),
		Version:  r.u32(),
		Reserved: r.u32(),
	}
	count := r.u32()
	if r.err != nil {
		return nil, r.err
	}
	// A single stray zero byte has been observed directly after the header, before the
	// first entry's path — harmless to skip if present, and every other entry boundary
	// already tolerates leading zero bytes via the zero-run skip below.
	if r.pos < len(r.data) && r.data[r.pos] == 0 {
		r.pos++
	}

	f.Entries = make([]SoundEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		start := r.pos
		rel := bytes.IndexByte(r.data[start:], 0)
		if rel < 0 {
			return nil, fmt.Errorf("asura: reading ASTS entry %d: unterminated path", i)
		}
		nul := start + rel
		path := string(r.data[start:nul])
		r.pos = nul
		for r.pos < len(r.data) && r.data[r.pos] == 0 {
			r.pos++
		}
		size := r.u32()
		offset := r.u32()
		if r.err != nil {
			return nil, fmt.Errorf("asura: reading ASTS entry %d (%q): %w", i, path, r.err)
		}
		if int(offset)+int(size) > len(data) || int(offset) < 0 || int(size) < 0 {
			return nil, fmt.Errorf("asura: ASTS entry %d (%q): audio range [%d:%d] outside file", i, path, offset, offset+size)
		}
		f.Entries = append(f.Entries, SoundEntry{Path: path, Data: data[offset : offset+size]})
	}
	return f, nil
}
