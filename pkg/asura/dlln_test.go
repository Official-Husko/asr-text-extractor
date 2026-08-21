package asura

import (
	"bytes"
	"testing"
)

// testOpaque returns a synthetic 25-byte opaque blob for the code1/code2/timestamp1/pad/
// timestamp2/skip4/command2 fields. Its first byte must be non-zero so scanCommand's
// trailing-zero scan doesn't mistake it for extra command padding.
func testOpaque() []byte {
	b := make([]byte, 25)
	b[0] = 0xAA
	return b
}

// buildDLLNEntry builds one Version-4 DLLN entry's raw bytes (tag through text data).
func buildDLLNEntry(command []byte, opaque []byte, text string) []byte {
	var buf bytes.Buffer
	buf.WriteString("DLLN")
	writeU32(&buf, 0) // Filesize: irrelevant to parsing, never used to seek
	writeU32(&buf, 4) // Version
	writeU32(&buf, 0) // Reserved ("Null")
	buf.Write(command)
	buf.Write(opaque)
	data := EncodeText(text)
	writeU32(&buf, uint32(len(data)/2))
	buf.Write(data)
	return buf.Bytes()
}

func TestUnpackVoiceV4(t *testing.T) {
	command := []byte{'C', 'M', 'D', 0}

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write([]byte{0x01, 0x02, 0x03}) // junk bytes the scan must skip over
	buf.Write(buildDLLNEntry(command, testOpaque(), "hello voice line"))

	entries, err := UnpackVoice(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Command != "CMD\x00" {
		t.Errorf("Command = %q", e.Command)
	}
	if e.SourceText != "hello voice line" || e.OverrideText != "hello voice line" {
		t.Errorf("text = %q / %q", e.SourceText, e.OverrideText)
	}
}

func TestUnpackVoiceUnknownVersionResyncs(t *testing.T) {
	command := []byte{'C', 'M', 'D', 0}

	// An entry with an unrecognized version (its text layout isn't understood) followed by
	// a normal Version-4 entry: the scan must resynchronize and still find the second one.
	var unknown bytes.Buffer
	unknown.WriteString("DLLN")
	writeU32(&unknown, 0)
	writeU32(&unknown, 99)
	writeU32(&unknown, 0)
	unknown.Write(command)
	unknown.Write(testOpaque())

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(unknown.Bytes())
	buf.Write(buildDLLNEntry(command, testOpaque(), "second entry"))

	entries, err := UnpackVoice(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].SourceText != "" {
		t.Errorf("unknown-version entry should have empty text, got %q", entries[0].SourceText)
	}
	if entries[1].SourceText != "second entry" {
		t.Errorf("second entry text = %q", entries[1].SourceText)
	}
}

func TestOverrideVoiceV4(t *testing.T) {
	command := []byte{'C', 'M', 'D', 0}

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.Write(buildDLLNEntry(command, testOpaque(), "hello voice line"))
	buf.Write([]byte{0x99, 0x98}) // trailing bytes after the last entry

	overrides := map[string]CSVRecord{
		"CMD\x00": {Command: "CMD\x00", SourceText: "hello voice line", OverrideText: "TRANSLATED"},
	}
	out, err := OverrideVoice(buf.Bytes(), overrides, true)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := UnpackVoice(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].SourceText != "TRANSLATED" {
		t.Fatalf("override did not apply: %+v", entries)
	}
	if !bytes.HasSuffix(out, []byte{0x99, 0x98}) {
		t.Errorf("trailing bytes after the last entry were not preserved")
	}
}

func TestOverrideVoiceRejectsNonV4(t *testing.T) {
	command := []byte{'C', 'M', 'D', 0}

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.WriteString("DLLN")
	writeU32(&buf, 0)
	writeU32(&buf, 5) // Version 5: unsupported for override
	writeU32(&buf, 0)
	buf.Write(command)
	buf.Write(testOpaque())

	if _, err := OverrideVoice(buf.Bytes(), map[string]CSVRecord{}, true); err == nil {
		t.Fatal("expected an error for a non-Version-4 entry")
	}
}
