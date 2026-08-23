package asura

import (
	"fmt"
)

// TextureEntry is a single texture asset from an RSCF chunk: the game's own source asset
// path (backslash-separated, e.g. `\graphics\characters\...\rs16_clothes_ar.tga` — the
// extension reflects the original art source, not the extracted data) and its raw bytes,
// sliced directly from the source file. Data always starts with the standard DDS magic
// (`DDS `) and is a complete, valid, standalone .dds file on its own — confirmed against real
// Zombie Army 4 data across every texture in a 763MB sample (correct declared DDS header
// fields, both legacy FourCC and DX10-extended pixel formats).
type TextureEntry struct {
	Path string
	Data []byte
}

// RSCFFile is a parsed RSCF chunk: a flat sequence of texture entries. Unlike HTXT/DLLN/ASTS,
// RSCF has no single chunk-level header — each entry repeats its own "RSCF" tag, one after
// another, until a 4-byte zero footer at the end of the file.
type RSCFFile struct {
	Entries []TextureEntry
}

// RSCF resource-type codes (the 3rd of an entry's 5 header fields, see parseRSCFEntry) —
// cross-checked against independent community Asura-format reference decoders
// (unpack_rebellion.py, tools_ZA4.py — see CLAUDE.md). Type 0 is a large, mixed category: most
// entries are per-object meshes (see mesh.go), but a couple of real samples are unrelated
// bulk data blobs ("inst (dynamic)"/"inst (static)", almost certainly a reference into the
// separate, much larger INST section) that don't decode as a mesh at all — ParseMesh's own
// size-reconciliation check is what tells the two apart, not the type code alone. Type 3 is
// audio (not seen in the sample used to build this feature, not implemented). Type 6 is a bare
// reference to another package with no embedded payload (e.g. a level package's self-reference
// to its own .pc file).
const (
	rscfResourceTypeMesh    = 0
	rscfResourceTypeTexture = 2
)

// ParseRSCF decodes a sequence of RSCF entries from data, which must start with the 8-byte
// Asura magic immediately followed by the first entry's "RSCF" tag.
//
// Each entry is: the "RSCF" tag, a uint32 giving the entry's total byte length (tag through
// the end of its payload — this is what makes walking the whole file reliable without needing
// to understand any entry's payload), 4 more uint32 fields (two of unconfirmed meaning, a
// resource-type code — see rscfResourceTypeTexture — and a flags field of unconfirmed
// meaning), a fifth uint32 giving the payload's exact byte length, a path string in the
// 4-byte-chunk-aligned encoding alignedString implements, and finally the payload itself,
// read directly via the declared length (not searched for by DDS magic — that was an earlier,
// working-but-imprecise approach this project used before cross-checking the field layout
// against independent reference decoders that read the same 5 fields with assertion-verified
// value ranges). Only resource-type-2 (texture) entries whose payload actually starts with
// the DDS magic are decoded as textures; every other entry (including further embedded audio,
// unimplemented so far) is skipped, not treated as an error, since the declared total length
// is enough to stay in sync with the rest of the file regardless.
//
// RSCF sections also show up embedded inside larger level-package files (see package.go);
// parseRSCFEntry below is the shared per-entry decoder both use.
func ParseRSCF(data []byte) (*RSCFFile, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	f := &RSCFFile{}
	pos := 8
	for pos < len(data) {
		if len(data)-pos < 28 {
			if allZero(data[pos:]) {
				break
			}
			return nil, fmt.Errorf("asura: %d trailing bytes are too short for an RSCF entry and aren't a zero footer", len(data)-pos)
		}
		if string(data[pos:pos+4]) != "RSCF" {
			if allZero(data[pos:]) {
				break
			}
			return nil, fmt.Errorf("asura: expected RSCF entry at offset %d, found %q", pos, data[pos:pos+4])
		}
		entry, nextPos, err := parseRSCFEntry(data, pos)
		if err != nil {
			return nil, err
		}
		if tex := entry.asTexture(); tex != nil {
			f.Entries = append(f.Entries, *tex)
		}
		pos = nextPos
	}
	return f, nil
}

// rscfEntry is one raw, type-tagged resource from an RSCF section, before interpreting its
// payload according to its resource-type code (see asTexture, mesh.go's asMesh).
type rscfEntry struct {
	path    string
	resType uint32
	payload []byte
}

// parseRSCFEntry decodes one RSCF entry at data[pos:] (which must already be known to start
// with the "RSCF" tag) and returns the raw entry, the position of the next entry, and any hard
// parse error (a truncated header, unterminated path, or a total/payload size that doesn't fit
// — never "this entry's payload isn't the type its resource-type code claims", which is left
// to the type-specific interpreters to decide).
func parseRSCFEntry(data []byte, pos int) (*rscfEntry, int, error) {
	entryStart := pos
	r := &reader{data: data, pos: pos + 4}
	totalSize := r.u32()
	r.bytes(8) // 2 fields, meaning not confirmed
	resType := r.u32()
	r.bytes(4) // flags, meaning not confirmed
	payloadSize := r.u32()
	if r.err != nil {
		return nil, 0, fmt.Errorf("asura: RSCF entry at offset %d: truncated header", entryStart)
	}

	path, payloadStart, ok := alignedString(data, r.pos)
	if !ok {
		return nil, 0, fmt.Errorf("asura: RSCF entry at offset %d: unterminated path", entryStart)
	}

	nextEntryStart := entryStart + int(totalSize)
	if totalSize == 0 || nextEntryStart <= entryStart || nextEntryStart > len(data) {
		return nil, 0, fmt.Errorf("asura: RSCF entry %q at offset %d: invalid total size %d", path, entryStart, totalSize)
	}

	payloadEnd := payloadStart + int(payloadSize)
	if payloadEnd > nextEntryStart {
		return nil, 0, fmt.Errorf("asura: RSCF entry %q at offset %d: declared payload size %d overruns entry", path, entryStart, payloadSize)
	}

	return &rscfEntry{path: path, resType: resType, payload: data[payloadStart:payloadEnd]}, nextEntryStart, nil
}

// asTexture interprets the entry as a texture: nil unless its resource-type code is
// rscfResourceTypeTexture and its payload actually starts with the DDS magic (the type code
// alone isn't trusted — see the rscfResourceType doc comment).
func (e *rscfEntry) asTexture() *TextureEntry {
	if e.resType != rscfResourceTypeTexture {
		return nil
	}
	if len(e.payload) < 4 || string(e.payload[:4]) != "DDS " {
		return nil
	}
	return &TextureEntry{Path: e.path, Data: e.payload}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
