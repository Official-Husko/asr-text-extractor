package asura

import (
	"strings"
	"testing"
)

func TestXMLRoundTrip(t *testing.T) {
	records := []Record{
		{Command: "1493970712", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"},
		{Command: "1522599863", SourceText: "2 of 2<END>", OverrideText: "translated"},
	}

	text, err := encodeXML(records, EncodingUTF8)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeXML(text)
	if err != nil {
		t.Fatalf("decodeXML: %v\ninput:\n%s", err, text)
	}
	if len(got) != len(records) {
		t.Fatalf("got %d records, want %d", len(got), len(records))
	}
	for i := range records {
		if got[i] != records[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], records[i])
		}
	}
}

// TestXMLEmbeddedNUL exercises the hex-fallback path: a raw NUL byte (as found in
// null-padded voice commands) can't appear in XML 1.0 character data at all, not even as a
// character reference, so encodeXML must hex-encode it and decodeXML must reverse that.
func TestXMLEmbeddedNUL(t *testing.T) {
	records := []Record{{Command: "CMD\x00", SourceText: "hi", OverrideText: "hi"}}

	text, err := encodeXML(records, EncodingUTF8)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `hex="true"`) {
		t.Fatalf("expected hex=\"true\" attribute for an unsafe command, got:\n%s", text)
	}
	got, err := decodeXML(text)
	if err != nil {
		t.Fatalf("decodeXML: %v\ninput:\n%s", err, text)
	}
	if got[0].Command != "CMD\x00" {
		t.Fatalf("Command = %q, want %q", got[0].Command, "CMD\x00")
	}
}

func TestXMLPrologEncoding(t *testing.T) {
	text, err := encodeXML(nil, EncodingUTF16LE)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `encoding="UTF-16"`) {
		t.Fatalf("expected a UTF-16 encoding declaration, got:\n%s", text)
	}
}
