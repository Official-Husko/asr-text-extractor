package asura

import (
	"bytes"
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

// ParseRSCF decodes a sequence of RSCF texture entries from data, which must start with the
// 8-byte Asura magic immediately followed by the first entry's "RSCF" tag.
//
// Each entry is: the "RSCF" tag, a uint32 giving the entry's total byte length (tag through
// the end of its DDS data — this is what makes walking the whole file reliable without
// needing to understand the DDS pixel format at all), 20 more bytes of fields whose exact
// meaning isn't confirmed (one of them varies per entry and was hypothesized to flag the
// asset's original source format, but that didn't hold up against real data, so they're left
// unparsed rather than guessed at), a NUL-terminated ASCII path, zero-padding, and finally
// the embedded DDS file itself, which is located by searching for the "DDS " magic within the
// entry's declared span rather than computed from any Asura-specific field.
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
// with the "RSCF" tag) and returns the decoded texture (nil if this entry's payload wasn't a
// recognizable DDS), the position of the next entry, and any hard parse error.
func parseRSCFEntry(data []byte, pos int) (*TextureEntry, int, error) {
	entryStart := pos
	r := &reader{data: data, pos: pos + 4}
	totalSize := r.u32()
	r.bytes(20) // 5 more fields, meaning not confirmed; not needed to locate this entry's end
	pathStart := r.pos
	nul := bytes.IndexByte(data[pathStart:], 0)
	if nul < 0 || r.err != nil {
		return nil, 0, fmt.Errorf("asura: RSCF entry at offset %d: unterminated path", entryStart)
	}
	path := string(data[pathStart : pathStart+nul])

	nextEntryStart := entryStart + int(totalSize)
	if totalSize == 0 || nextEntryStart <= entryStart || nextEntryStart > len(data) {
		return nil, 0, fmt.Errorf("asura: RSCF entry %q at offset %d: invalid total size %d", path, entryStart, totalSize)
	}
	if ddsRel := bytes.Index(data[pathStart+nul:nextEntryStart], []byte("DDS ")); ddsRel >= 0 {
		ddsStart := pathStart + nul + ddsRel
		return &TextureEntry{Path: path, Data: data[ddsStart:nextEntryStart]}, nextEntryStart, nil
	}
	// "DDS " wasn't found in this entry's span — payload not understood, but still advance
	// by the declared total size so the caller stays in sync with the rest of the file.
	return nil, nextEntryStart, nil
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
