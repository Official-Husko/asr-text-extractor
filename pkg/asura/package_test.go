package asura

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type manifestEntrySpec struct {
	path   string
	offset uint32
	size   uint32
}

// buildManifest constructs a standalone RSFL manifest section (starting at position 0 of the
// returned slice), in the shape confirmed against a real Zombie Army 4 level package: tag,
// size, two unused fields, entry count, then per entry a NUL-terminated path padded so the
// offset field starts at a 4-byte-aligned position, followed by offset/size/a trailing field
// (always 1 in every real sample). Padding is computed against buf.Len() itself rather than
// some passed-in absolute base — valid as long as whatever precedes this section in a larger
// buffer is itself a multiple of 4 bytes, true of every caller here (Magic + a header-only
// FNFO section is 16 bytes).
func buildManifest(entries []manifestEntrySpec) []byte {
	var buf bytes.Buffer
	buf.WriteString("RSFL")
	writeU32(&buf, 0) // size placeholder, patched below
	writeU32(&buf, 0) // f1, unused by the parser
	writeU32(&buf, 0) // f2, unused by the parser
	writeU32(&buf, uint32(len(entries)))
	for _, e := range entries {
		buf.WriteString(e.path)
		buf.WriteByte(0)
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
		writeU32(&buf, e.offset)
		writeU32(&buf, e.size)
		writeU32(&buf, 1)
	}
	out := buf.Bytes()
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)))
	return out
}


func TestParseRSFLManifest(t *testing.T) {
	entries := []manifestEntrySpec{
		{path: `LevelExportTemp0\a.bin`, offset: 0, size: 4},
		{path: `LevelExportTemp0\b.bin`, offset: 4, size: 8},
	}
	manifest := buildManifest(entries)

	got, end, err := parseRSFLManifest(manifest, 0)
	if err != nil {
		t.Fatalf("parseRSFLManifest: %v", err)
	}
	if end != len(manifest) {
		t.Errorf("end = %d, want %d", end, len(manifest))
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].path != entries[0].path || got[0].offset != int(entries[0].offset) || got[0].size != int(entries[0].size) {
		t.Errorf("entry 0 = %+v", got[0])
	}
	if got[1].path != entries[1].path || got[1].offset != int(entries[1].offset) || got[1].size != int(entries[1].size) {
		t.Errorf("entry 1 = %+v", got[1])
	}
}

// TestParseRSFLManifestOffsetWithZeroLeadByte regression-tests a real bug: an earlier
// implementation padded by skipping every zero byte after a path's NUL terminator, rather
// than padding to a fixed 4-byte alignment. That misparsed any entry whose offset field's own
// low byte happened to be zero — mistaking it for one more byte of padding and shifting every
// field after it (offset, size, the trailing field, and the whole next entry's path) by one
// byte. Found against a real Zombie Army 4 level package, where it silently corrupted 2 of
// 282 manifest entries.
func TestParseRSFLManifestOffsetWithZeroLeadByte(t *testing.T) {
	entries := []manifestEntrySpec{
		{path: `a.bin`, offset: 0x00000100, size: 4}, // low byte is 0x00
		{path: `b.bin`, offset: 8, size: 4},
	}
	manifest := buildManifest(entries)

	got, _, err := parseRSFLManifest(manifest, 0)
	if err != nil {
		t.Fatalf("parseRSFLManifest: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].offset != 0x100 {
		t.Errorf("entry 0 offset = %#x, want 0x100", got[0].offset)
	}
	if got[1].path != "b.bin" {
		t.Errorf("entry 1 path = %q, want %q (a naive zero-skip would misalign and corrupt this)", got[1].path, "b.bin")
	}
	if got[1].offset != 8 {
		t.Errorf("entry 1 offset = %#x, want 8", got[1].offset)
	}
}

func TestParsePackageBadMagic(t *testing.T) {
	if _, err := parsePackageContent([]byte("not an asura file")); err != ErrBadMagic {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

func TestParsePackageEntries(t *testing.T) {
	entryAData := []byte("AAAA")
	entryBData := []byte("BBBBBBBB")
	entries := []manifestEntrySpec{
		{path: `LevelExportTemp0\a.bin`, offset: 0, size: uint32(len(entryAData))},
		{path: `LevelExportTemp0\b.bin`, offset: uint32(len(entryAData)), size: uint32(len(entryBData))},
	}
	manifest := buildManifest(entries)

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.WriteString("FNFO")
	writeU32(&buf, 8) // header-only FNFO: tag + size, nothing else
	buf.Write(manifest)
	buf.Write(entryAData)
	buf.Write(entryBData)

	pkg, err := parsePackageContent(buf.Bytes())
	if err != nil {
		t.Fatalf("parsePackageContent: %v", err)
	}
	if len(pkg.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(pkg.Entries))
	}
	if pkg.Entries[0].Path != `LevelExportTemp0\a.bin` || !bytes.Equal(pkg.Entries[0].Data, entryAData) {
		t.Errorf("entry 0 = %q %q", pkg.Entries[0].Path, pkg.Entries[0].Data)
	}
	if pkg.Entries[1].Path != `LevelExportTemp0\b.bin` || !bytes.Equal(pkg.Entries[1].Data, entryBData) {
		t.Errorf("entry 1 = %q %q", pkg.Entries[1].Path, pkg.Entries[1].Data)
	}
	if len(pkg.Textures) != 0 {
		t.Errorf("got %d textures, want 0 (no RSCF sections in this fixture)", len(pkg.Textures))
	}
}

// TestParsePackageTextures confirms the interleaved-sections walk found against a real
// sample: RSCF texture entries sit one at a time among many unrelated tagged sections
// (geometry data, per-object records, ...) rather than packed together in their own
// contiguous block, and some RSCF-tagged entries are bare references with no embedded
// texture at all and must be skipped without being counted.
func TestParsePackageTextures(t *testing.T) {
	dds := append([]byte("DDS "), bytes.Repeat([]byte{0xAB}, 16)...)
	subfileData := []byte("SUBFILE!")

	var extras bytes.Buffer
	extras.WriteString("PBRV") // an unrelated tagged section that must be skipped over
	writeU32(&extras, 16)      // covers itself (8 bytes) + 8 bytes of payload
	extras.Write([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	extras.Write(buildRSCFEntry(`Envs\Foo.pc`, 6, nil)) // bare reference, no texture
	extras.Write(buildRSCFEntry(`graphics\a.dds`, rscfResourceTypeTexture, dds))

	entries := []manifestEntrySpec{
		{path: `sub.bin`, offset: uint32(extras.Len()), size: uint32(len(subfileData))},
	}
	manifest := buildManifest(entries)

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.WriteString("FNFO")
	writeU32(&buf, 8)
	buf.Write(manifest)
	buf.Write(extras.Bytes())
	buf.Write(subfileData)

	pkg, err := parsePackageContent(buf.Bytes())
	if err != nil {
		t.Fatalf("parsePackageContent: %v", err)
	}
	if len(pkg.Entries) != 1 || !bytes.Equal(pkg.Entries[0].Data, subfileData) {
		t.Fatalf("Entries = %+v", pkg.Entries)
	}
	if len(pkg.Textures) != 1 {
		t.Fatalf("got %d textures, want 1", len(pkg.Textures))
	}
	if pkg.Textures[0].Path != `graphics\a.dds` {
		t.Errorf("texture Path = %q", pkg.Textures[0].Path)
	}
	if !bytes.Equal(pkg.Textures[0].Data, dds) {
		t.Errorf("texture Data mismatch")
	}
}

func TestParsePackage(t *testing.T) {
	entries := []manifestEntrySpec{
		{path: `a.bin`, offset: 0, size: 4},
	}
	manifest := buildManifest(entries)

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.WriteString("FNFO")
	writeU32(&buf, 8)
	buf.Write(manifest)
	buf.WriteString("DATA")

	raw := buildZbb(t, buf.Bytes())

	pkg, err := ParsePackage(raw)
	if err != nil {
		t.Fatalf("ParsePackage: %v", err)
	}
	if len(pkg.Entries) != 1 || string(pkg.Entries[0].Data) != "DATA" {
		t.Fatalf("Entries = %+v", pkg.Entries)
	}
}
