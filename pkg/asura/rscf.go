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

// AudioEntry is a single embedded WAV asset from an RSCF chunk's resource-type-3 entries — the
// game's own source asset path (e.g. `sounds\hud\duty_roster_collected\..._01.wav`) and its raw
// bytes, sliced directly from the source file. Data always starts with the standard RIFF/WAVE
// magic and is a complete, valid, standalone .wav file on its own — confirmed against a real
// Zombie Army 4 `.pc.sounds` sample (`Chars/mp.pc.pc.sounds`): a plain (uncompressed)
// `"Asura   "`-signed file whose very first section is RSCF, sharing the exact same per-entry
// tag+size+type framing `parseRSCFEntry` already decodes for textures/meshes — this is a
// standalone sibling of a `.pc`'s embedded RSCF section, not a new container format.
type AudioEntry struct {
	Path string
	Data []byte
}

// RSCFFile is a parsed RSCF chunk: a flat sequence of texture and audio entries. Unlike
// HTXT/DLLN/ASTS, RSCF has no single chunk-level header — each entry repeats its own "RSCF"
// tag, one after another, until a 4-byte zero footer at the end of the file.
type RSCFFile struct {
	Entries      []TextureEntry
	AudioEntries []AudioEntry
}

// RSCF resource-type codes (the 3rd of an entry's 5 header fields, see parseRSCFEntry) —
// cross-checked against independent reference implementations of the format. Type 0 is a large,
// mixed category: most entries are per-object meshes (see mesh.go), but real samples also
// contain two other, unrelated shapes that don't decode as a mesh at all — ParseMesh's own
// size-reconciliation check is what tells all three apart, not the type code alone:
//   - All-zero 16-byte stub payloads — placement/marker/proxy objects (GUI prompt anchors,
//     physics compound proxies, "null_object" gizmo points) that share the mesh resource type
//     but carry no geometry whatsoever. Common in Sniper Elite 5 (45 of 1,613 resource-type-0
//     entries in one real DLC package); not seen in Zombie Army 4 samples, but the same
//     "payload too short for even the fixed header" check that catches these there would catch
//     an equivalent ZA4 stub too, if one exists.
//   - Unrelated bulk data blobs named "inst (dynamic)"/"inst (static)" (seen in both Zombie
//     Army 4 and Sniper Elite 5 samples) and, in Sniper Elite 5 specifically, one further named
//     "Env" — almost certainly references into a separate, much larger section (`INST`) this
//     project doesn't parse, not mesh data in a format ParseMesh doesn't yet understand: their
//     header fields don't remotely reconcile with their actual payload size (one real "inst
//     (static)" entry's header predicts a payload over 5x larger than what's actually there).
//
// Type 3 is audio — see AudioEntry and asAudio. Type 6 is a bare reference to another package
// with no embedded payload (e.g. a level package's self-reference to its own .pc file).
const (
	rscfResourceTypeMesh    = 0
	rscfResourceTypeTexture = 2
	rscfResourceTypeAudio   = 3
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
		} else if aud := entry.asAudio(); aud != nil {
			f.AudioEntries = append(f.AudioEntries, *aud)
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

// asAudio interprets the entry as embedded audio: nil unless its resource-type code is
// rscfResourceTypeAudio and its payload actually starts with the RIFF magic (the type code
// alone isn't trusted, mirroring asTexture's own DDS-magic check).
func (e *rscfEntry) asAudio() *AudioEntry {
	if e.resType != rscfResourceTypeAudio {
		return nil
	}
	if len(e.payload) < 4 || string(e.payload[:4]) != "RIFF" {
		return nil
	}
	return &AudioEntry{Path: e.path, Data: e.payload}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}
