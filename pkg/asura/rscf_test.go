package asura

import (
	"bytes"
	"testing"
)

// buildRSCFEntry constructs one RSCF entry in the shape confirmed against independent
// community Asura-format reference decoders (unpack_rebellion.py, tools_ZA4.py — see
// CLAUDE.md): tag, a total-size field, 2 fields of unconfirmed meaning, a resource-type code,
// a flags field of unconfirmed meaning, a payload-size field, a path in the 4-byte-chunk-
// aligned string encoding alignedString implements, and finally exactly payloadSize bytes of
// payload.
func buildRSCFEntry(path string, resType uint32, payload []byte) []byte {
	var tail bytes.Buffer
	tail.WriteString(path)
	tail.WriteByte(0)
	for tail.Len()%4 != 0 {
		tail.WriteByte(0)
	}
	tail.Write(payload)

	totalSize := 4 + 4 + 20 + tail.Len() // tag + size field + 5 fields + tail

	var out bytes.Buffer
	out.WriteString("RSCF")
	writeU32(&out, uint32(totalSize))
	writeU32(&out, 0)                    // unk1
	writeU32(&out, 0)                    // unk2
	writeU32(&out, resType)
	writeU32(&out, 0)                    // flags
	writeU32(&out, uint32(len(payload)))
	out.Write(tail.Bytes())
	return out.Bytes()
}

func fakeDDS(n int) []byte {
	b := append([]byte("DDS "), bytes.Repeat([]byte{0xAB}, n)...)
	return b
}

func TestParseRSCF(t *testing.T) {
	e0 := buildRSCFEntry(`\graphics\a.tga`, rscfResourceTypeTexture, fakeDDS(40))
	e1 := buildRSCFEntry(`\graphics\nested\b.tga`, rscfResourceTypeTexture, fakeDDS(64))

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(e0)
	buf.Write(e1)
	buf.Write(make([]byte, 4)) // footer

	f, err := ParseRSCF(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseRSCF: %v", err)
	}
	if len(f.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(f.Entries))
	}
	if f.Entries[0].Path != `\graphics\a.tga` {
		t.Errorf("entry 0 Path = %q", f.Entries[0].Path)
	}
	if string(f.Entries[0].Data[:4]) != "DDS " {
		t.Errorf("entry 0 Data doesn't start with DDS magic: %q", f.Entries[0].Data[:4])
	}
	if len(f.Entries[0].Data) != len(fakeDDS(40)) {
		t.Errorf("entry 0 Data length = %d, want %d", len(f.Entries[0].Data), len(fakeDDS(40)))
	}
	if f.Entries[1].Path != `\graphics\nested\b.tga` {
		t.Errorf("entry 1 Path = %q", f.Entries[1].Path)
	}
	if !bytes.Equal(f.Entries[1].Data, fakeDDS(64)) {
		t.Errorf("entry 1 Data mismatch")
	}
}

func TestParseRSCFBadMagic(t *testing.T) {
	if _, err := ParseRSCF([]byte("not an asura file")); err != ErrBadMagic {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

// TestParseRSCFSkipsNonTextureEntry regression-tests a real case: some RSCF entries are bare
// references to another resource (resource-type 0 or 6) with a declared payload size of 0 —
// e.g. a level package's self-reference to its own .pc file, found immediately after the
// manifest in a real Zombie Army 4 sample. These must be skipped without being counted as a
// texture, and parsing must still resync correctly on the entry that follows.
func TestParseRSCFSkipsNonTextureEntry(t *testing.T) {
	ref := buildRSCFEntry(`Envs\Foo.pc`, 6, nil)
	tex := buildRSCFEntry(`\graphics\a.tga`, rscfResourceTypeTexture, fakeDDS(20))

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(ref)
	buf.Write(tex)
	buf.Write(make([]byte, 4))

	f, err := ParseRSCF(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseRSCF: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("got %d entries, want 1 (the reference entry should be skipped, not counted)", len(f.Entries))
	}
	if f.Entries[0].Path != `\graphics\a.tga` {
		t.Errorf("surviving entry Path = %q", f.Entries[0].Path)
	}
}

// TestParseRSCFSkipsTextureTypeWithBadPayload covers the defensive fallback: an entry whose
// resource-type field claims texture (2) but whose declared payload doesn't actually start
// with the DDS magic shouldn't be trusted, since the field layout is understood only via
// cross-referencing other tools, not a first-party spec.
func TestParseRSCFSkipsTextureTypeWithBadPayload(t *testing.T) {
	bad := buildRSCFEntry(`\graphics\unknown.bin`, rscfResourceTypeTexture, bytes.Repeat([]byte{0xCD}, 16))
	good := buildRSCFEntry(`\graphics\a.tga`, rscfResourceTypeTexture, fakeDDS(20))

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(bad)
	buf.Write(good)
	buf.Write(make([]byte, 4))

	f, err := ParseRSCF(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseRSCF: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("got %d entries, want 1 (the bad-payload entry should be skipped, not aborted)", len(f.Entries))
	}
	if f.Entries[0].Path != `\graphics\a.tga` {
		t.Errorf("surviving entry Path = %q", f.Entries[0].Path)
	}
}

func TestParseRSCFInvalidTotalSize(t *testing.T) {
	var out bytes.Buffer
	out.WriteString("RSCF")
	writeU32(&out, 0) // invalid: zero total size
	out.Write(make([]byte, 20))
	out.WriteString("x\x00")

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(out.Bytes())

	if _, err := ParseRSCF(buf.Bytes()); err == nil {
		t.Fatal("expected an error for a zero total-size field")
	}
}

func TestParseRSCFPayloadSizeOverrunsEntry(t *testing.T) {
	path := `\graphics\a.tga`
	var tail bytes.Buffer
	tail.WriteString(path)
	tail.WriteByte(0)
	for tail.Len()%4 != 0 {
		tail.WriteByte(0)
	}
	tail.Write(fakeDDS(8))

	var out bytes.Buffer
	out.WriteString("RSCF")
	writeU32(&out, uint32(4+4+20+tail.Len()))
	writeU32(&out, 0)
	writeU32(&out, 0)
	writeU32(&out, rscfResourceTypeTexture)
	writeU32(&out, 0)
	writeU32(&out, 9999) // declared payload size far larger than the entry itself
	out.Write(tail.Bytes())

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(out.Bytes())

	if _, err := ParseRSCF(buf.Bytes()); err == nil {
		t.Fatal("expected an error when the declared payload size overruns the entry")
	}
}
