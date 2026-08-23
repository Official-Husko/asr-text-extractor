package asura

import (
	"encoding/binary"
	"fmt"
)

// PackageEntry is one file referenced by an Asura level-package's FNFO/RSFL manifest — e.g.
// a .anim, .pfx, .snd, .nav, .cut, or .ent sub-file — addressed by absolute byte offset/size
// within the package's fully-decompressed content.
type PackageEntry struct {
	Path string
	Data []byte
}

// Package is a parsed Asura level-package file (.pc, .pc_entdata — both AsuraZbb-compressed):
// the manifest-referenced sub-files, plus any embedded RSCF texture and mesh entries found
// among the many tagged sections (geometry/spatial data, per-object records, etc. — see
// parsePackageContent) that sit between the manifest and those sub-files. Not every package
// has RSCF sections there (e.g. .pc_entdata doesn't), in which case Textures and Meshes are
// both empty.
type Package struct {
	Entries  []PackageEntry
	Textures []TextureEntry
	Meshes   []Mesh
}

// ParsePackage decompresses an AsuraZbb-wrapped level-package file and parses its FNFO/RSFL
// manifest, plus any embedded RSCF texture archive that follows.
func ParsePackage(raw []byte) (*Package, error) {
	data, err := DecompressZbb(raw)
	if err != nil {
		return nil, err
	}
	return parsePackageContent(data)
}

func parsePackageContent(data []byte) (*Package, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	rsflStart, tag, ok := skipTaggedSection(data, 8)
	if !ok || tag != "FNFO" {
		return nil, fmt.Errorf("asura: expected FNFO manifest header at offset 8")
	}

	manifest, rsflEnd, err := parseRSFLManifest(data, rsflStart)
	if err != nil {
		return nil, err
	}

	// Each entry's declared offset is relative to the end of the RSFL manifest section
	// itself, not the start of the package's decompressed content — confirmed by real
	// entries decoding to a clean small count field followed by the sub-file's own name
	// repeated (e.g. offset -> "\x07\x00\x00\x00\x00\x00\x00\x00Explo_Bo..." exactly
	// matching "Explo_Box_Sm_Chunk_13.anim") only once rsflEnd is added in.
	pkg := &Package{}
	extrasStart := len(data)
	for _, e := range manifest {
		start := rsflEnd + e.offset
		if start < extrasStart {
			extrasStart = start
		}
	}
	for _, e := range manifest {
		start := rsflEnd + e.offset
		end := start + e.size
		if start < 0 || e.size < 0 || end > len(data) || end < start {
			continue // defensively skip a malformed entry rather than slice out of range
		}
		pkg.Entries = append(pkg.Entries, PackageEntry{Path: e.path, Data: data[start:end]})
	}

	// Between the manifest and the manifest-referenced sub-files sits a long run of tagged
	// sections — geometry/spatial data (PBRV, SDPH), a small unidentified one (IRTX), and
	// many section types that turn out to be per-level-object records (CONA entity
	// transforms, SDSM/SDEV spatial data, HSKN/HSKL/HSBB/HSKE/HMPT skeleton/hitbox data,
	// FAAN/TXAN animation refs, and more) — all sharing the same generic tag+size framing.
	// RSCF sections are interleaved throughout this run one at a time (each immediately
	// followed by unrelated sections, not packed contiguously); most are textures or
	// single-group meshes, but some are bare resource references or other resource types this
	// package doesn't decode (confirmed against a real sample: 3071 RSCF sections — 2502
	// textures, matching an independent whole-file search for "DDS " exactly, 550 single-group
	// meshes, 1 bare package reference, and 2 unrelated "inst" resources that don't decode as
	// anything here). So rather than trying to locate "the start of the RSCF archive", every
	// section is walked generically, and each one tagged RSCF is decoded as a possible texture
	// or mesh entry inline.
	pos := rsflEnd
	for pos < extrasStart {
		if len(data)-pos >= 4 && string(data[pos:pos+4]) == "RSCF" {
			entry, next, err := parseRSCFEntry(data, pos)
			if err != nil {
				break
			}
			if tex := entry.asTexture(); tex != nil {
				pkg.Textures = append(pkg.Textures, *tex)
			} else if m := entry.asMesh(); m != nil {
				pkg.Meshes = append(pkg.Meshes, *m)
			}
			pos = next
			continue
		}
		next, _, ok := skipTaggedSection(data, pos)
		if !ok {
			break
		}
		pos = next
	}

	return pkg, nil
}

type manifestEntry struct {
	path   string
	offset int
	size   int
}

// parseRSFLManifest parses an RSFL manifest section starting at data[start:] (start points
// at the "RSFL" tag itself), returning its entries and the position right after the section.
// Each entry is a path in the 4-byte-chunk-aligned string encoding alignedString implements
// (not "skip every zero byte present after the NUL terminator" — a handful of real entries
// have an offset whose own low byte is coincidentally zero, which a greedy zero-skip would
// misinterpret as one more byte of padding and misalign every field after it), then a uint32
// offset, a uint32 size, and one more uint32 field. That last field is always exactly 1 across
// all 282 entries in a real sample once alignment is computed this way (a strong
// self-consistency check — the greedy-skip approach got it right for 280 of those 282, with
// the 2 failures exactly matching the entries with a zero-byte-first offset), and matches
// independent community reference decoders (unpack_rebellion.py, tools_ZA4.py — see
// CLAUDE.md), which parse this same field as always-1 too. The offset is relative to the end
// of this manifest section (i.e. to the second return value) — not the start of the package's
// decompressed content — confirmed by real entries decoding cleanly only once that base is
// added: e.g. one real .anim entry decodes to a small count field followed by the sub-file's
// own name repeated verbatim ("\x07\x00\x00\x00\x00\x00\x00\x00Explo_Bo..." exactly matching
// "Explo_Box_Sm_Chunk_13.anim") only with the manifest-end offset included; every entry's
// offset+size still lands exactly on the next entry's own offset either way, which doesn't by
// itself distinguish the two — don't rely on that check alone if this ever needs re-deriving.
func parseRSFLManifest(data []byte, start int) ([]manifestEntry, int, error) {
	end, tag, ok := skipTaggedSection(data, start)
	if !ok || tag != "RSFL" {
		return nil, 0, fmt.Errorf("asura: expected RSFL manifest at offset %d", start)
	}
	if end-start < 20 {
		return nil, 0, fmt.Errorf("asura: RSFL manifest at offset %d is too short", start)
	}
	num := binary.LittleEndian.Uint32(data[start+16 : start+20])
	pos := start + 20

	entries := make([]manifestEntry, 0, num)
	for i := uint32(0); i < num; i++ {
		path, p, ok := alignedString(data, pos)
		if !ok {
			return nil, 0, fmt.Errorf("asura: RSFL manifest entry %d: unterminated or truncated path", i)
		}
		if len(data)-p < 12 {
			return nil, 0, fmt.Errorf("asura: RSFL manifest entry %d (%q): truncated", i, path)
		}
		offset := binary.LittleEndian.Uint32(data[p : p+4])
		size := binary.LittleEndian.Uint32(data[p+4 : p+8])
		pos = p + 12
		entries = append(entries, manifestEntry{path: path, offset: int(offset), size: int(size)})
	}
	return entries, end, nil
}

// skipTaggedSection returns the position right after a generic Asura "tag+size" section (a
// 4-byte all-uppercase-ASCII tag, then a uint32 giving the section's total length measured
// from the tag's own start) without needing to understand what's inside — the same framing
// FNFO and RSFL use, and (as far as skipping over them goes) SDPH/PBRV too.
func skipTaggedSection(data []byte, pos int) (nextPos int, tag string, ok bool) {
	if len(data)-pos < 8 {
		return pos, "", false
	}
	tagBytes := data[pos : pos+4]
	for _, c := range tagBytes {
		if c < 'A' || c > 'Z' {
			return pos, "", false
		}
	}
	size := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
	next := pos + int(size)
	if next <= pos || next > len(data) {
		return pos, "", false
	}
	return next, string(tagBytes), true
}
