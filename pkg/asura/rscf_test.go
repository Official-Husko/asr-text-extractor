package asura

import (
	"bytes"
	"testing"
)

// buildRSCFEntry constructs one RSCF entry in the shape confirmed against a real 763MB
// Zombie Army 4 texture archive: tag, a total-size field, 5 more fields (meaning not fully
// understood, so the test just exercises that they're skipped correctly), a NUL-terminated
// path, some zero padding, and a "DDS "-prefixed payload.
func buildRSCFEntry(path string, ddsPayload []byte) []byte {
	var tail bytes.Buffer
	tail.WriteString(path)
	tail.WriteByte(0)
	tail.Write([]byte{0, 0, 0}) // padding; the parser tolerates any length via a DDS-magic scan
	tail.Write(ddsPayload)

	totalSize := 4 + 4 + 20 + tail.Len() // tag + size field + 5 fields + tail

	var out bytes.Buffer
	out.WriteString("RSCF")
	writeU32(&out, uint32(totalSize))
	writeU32(&out, 0) // fieldB
	writeU32(&out, 2) // fieldC
	writeU32(&out, 2) // fieldD
	writeU32(&out, 0) // fieldE
	writeU32(&out, uint32(len(ddsPayload)))
	out.Write(tail.Bytes())
	return out.Bytes()
}

func fakeDDS(n int) []byte {
	b := append([]byte("DDS "), bytes.Repeat([]byte{0xAB}, n)...)
	return b
}

func TestParseRSCF(t *testing.T) {
	e0 := buildRSCFEntry(`\graphics\a.tga`, fakeDDS(40))
	e1 := buildRSCFEntry(`\graphics\nested\b.tga`, fakeDDS(64))

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

func TestParseRSCFSkipsEntryWithoutDDSMagic(t *testing.T) {
	// An entry whose payload doesn't contain "DDS " should be skipped, not abort the walk,
	// as long as its declared total size is trustworthy enough to find the next entry.
	var tail bytes.Buffer
	tail.WriteString(`\graphics\unknown.bin`)
	tail.WriteByte(0)
	tail.Write([]byte{0, 0, 0})
	tail.Write(bytes.Repeat([]byte{0xCD}, 16)) // no "DDS " anywhere in here
	totalSize := 4 + 4 + 20 + tail.Len()
	var bad bytes.Buffer
	bad.WriteString("RSCF")
	writeU32(&bad, uint32(totalSize))
	writeU32(&bad, 0)
	writeU32(&bad, 2)
	writeU32(&bad, 2)
	writeU32(&bad, 0)
	writeU32(&bad, 0)
	bad.Write(tail.Bytes())

	good := buildRSCFEntry(`\graphics\a.tga`, fakeDDS(20))

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(bad.Bytes())
	buf.Write(good)
	buf.Write(make([]byte, 4))

	f, err := ParseRSCF(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseRSCF: %v", err)
	}
	if len(f.Entries) != 1 {
		t.Fatalf("got %d entries, want 1 (the unrecognized entry should be skipped, not aborted)", len(f.Entries))
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
