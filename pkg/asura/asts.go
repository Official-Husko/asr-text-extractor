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
// ASCII asset path, then zero-padding to a fixed alignment (not "skip every zero byte present"
// — see the doc comment on the padding computation below), then a uint32 byte size and a
// uint32 absolute file offset — the audio bytes themselves are appended back-to-back
// immediately after the manifest, in entry order, ending in a 4-byte zero footer.
//
// The single byte directly after the header (before the first entry's path) is a real,
// confirmed flag, not incidental padding: 0 in every real self-contained sample checked (425
// real Sniper Elite Resistance files, plus the already-verified Sniper Elite 5/Zombie Army 4
// samples), 1 in every real ".ssm"-named companion manifest checked (100 files total across all
// three titles, no other value ever seen). A flag-1 file's own entries never reconcile against
// its own declared size no matter what padding is tried — because it isn't a self-contained
// audio container at all: it's a lightweight reference-only manifest whose real audio lives in a
// sibling "<name>.asr.*.streamsounds" file sharing the exact same entry paths. Confirmed
// directly: real "sounds/ss_alps.ssm.pc.streamsounds" (504 bytes, flag=1, 7 entries, every one
// failing the size/offset self-consistency check) sits next to a real
// "sounds/ss_alps.asr.pc.streamsounds" (27.9MB, flag=0) whose own 7 entries are the identical 7
// asset paths and extract to real, valid WAV data — so no audio is actually inaccessible, a
// flag-1 file just isn't the file to extract it from. ParseASTS surfaces this immediately as a
// specific error rather than letting entry parsing fail deep in with a confusing "audio range
// outside file" message.
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
	if r.pos >= len(r.data) {
		return nil, fmt.Errorf("asura: ASTS chunk truncated before its embedded-audio flag byte")
	}
	if flag := r.data[r.pos]; flag != 0 {
		return nil, fmt.Errorf("asura: ASTS chunk has no embedded audio (reference-only manifest, flag=%d) — look for a sibling \"<name>.asr.*.streamsounds\" file with the same asset paths", flag)
	}
	r.pos++

	f.Entries = make([]SoundEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		start := r.pos
		rel := bytes.IndexByte(r.data[start:], 0)
		if rel < 0 {
			return nil, fmt.Errorf("asura: reading ASTS entry %d: unterminated path", i)
		}
		nul := start + rel
		path := string(r.data[start:nul])
		// Padding after the NUL terminator is NOT "skip every zero byte present" — an earlier
		// implementation did that and broke as soon as a real sample's size/offset field
		// itself started with a zero byte (a genuine ~1-in-256 coincidence per field, common
		// enough to hit in a large real file: a real Sniper Elite 5 sample with 15,509 entries
		// failed at entry 84 this way, decoding size=3556769969/offset=1140892985 — garbage
		// from a real, valid entry whose size's own low byte happened to be 0x00). The real
		// rule, confirmed by checking every one of that same 15,509-entry file's entries (plus
		// 581 in a real Zombie Army 4 sample — the same bug, not a per-game difference) against
		// the file's own declared total size: padding always lands the size field at the
		// smallest position where (pathLen+1+padding) ≡ 1 (mod 4), i.e. always 1 to 4 padding
		// bytes, never 0 and never determined by scanning for the next non-zero byte.
		pathLen := rel
		padding := (4-(pathLen+1)%4)%4 + 1
		r.pos = nul + 1 + padding
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
