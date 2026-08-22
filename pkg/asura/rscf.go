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

// rscfResourceTypeTexture is the RSCF resource-type code (the 3rd of an entry's 5 header
// fields, see parseRSCFEntry) for a texture entry. Other observed values — cross-checked
// against independent community Asura-format reference decoders (unpack_rebellion.py,
// tools_ZA4.py — see CLAUDE.md) — are 0 (a bare reference to another resource, no embedded
// payload: e.g. a level package's self-reference to its own .pc file, seen with a declared
// payload size of 0), 3 (audio), and 6 (a reference to a .pc package, also no embedded
// payload in samples seen so far). Only textures are decoded today; audio entries exist in
// real data but extracting them hasn't been tried yet.
const rscfResourceTypeTexture = 2

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
		if entry != nil {
			f.Entries = append(f.Entries, *entry)
		}
		pos = nextPos
	}
	return f, nil
}

// parseRSCFEntry decodes one RSCF entry at data[pos:] (which must already be known to start
// with the "RSCF" tag) and returns the decoded texture (nil if this entry isn't a texture, or
// its declared payload doesn't actually start with the DDS magic), the position of the next
// entry, and any hard parse error.
func parseRSCFEntry(data []byte, pos int) (*TextureEntry, int, error) {
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

	if resType != rscfResourceTypeTexture {
		return nil, nextEntryStart, nil
	}
	payload := data[payloadStart:payloadEnd]
	if len(payload) < 4 || string(payload[:4]) != "DDS " {
		// The type field says texture but the payload doesn't look like one — don't trust
		// it, still advance by the declared total size so the caller stays in sync.
		return nil, nextEntryStart, nil
	}
	return &TextureEntry{Path: path, Data: payload}, nextEntryStart, nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
