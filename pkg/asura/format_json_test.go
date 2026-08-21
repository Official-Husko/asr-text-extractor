package asura

import (
	"strings"
	"testing"
)

// TestJSONNoHTMLEscaping guards against json.Marshal's default HTML-escaping of angle
// brackets and ampersands (meant for JSON embedded inside HTML). Round-tripping works either
// way since both forms are valid JSON, but escaping would mangle every <TAG> placeholder for
// anyone hand-editing the file, defeating the point of a human-editable format.
func TestJSONNoHTMLEscaping(t *testing.T) {
	records := []Record{{Command: "1", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"}}

	text, err := encodeJSON(records)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "<END>") {
		t.Errorf("expected the literal tag text in output, got:\n%s", text)
	}
	escapedLessThan := "\\u003c" // the 6-character sequence backslash,u,0,0,3,c
	if strings.Contains(text, escapedLessThan) {
		t.Errorf("output was HTML-escaped, got:\n%s", text)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	records := []Record{
		{Command: "1493970712", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"},
		{Command: "1522599863", SourceText: "2 of 2<END>", OverrideText: "translated"},
	}

	text, err := encodeJSON(records)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeJSON(text)
	if err != nil {
		t.Fatalf("decodeJSON: %v\ninput:\n%s", err, text)
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

func TestJSONEmbeddedNUL(t *testing.T) {
	// Voice commands can carry raw null-byte padding; JSON must round-trip it losslessly.
	records := []Record{{Command: "CMD\x00", SourceText: "hi", OverrideText: "hi"}}

	text, err := encodeJSON(records)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeJSON(text)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Command != "CMD\x00" {
		t.Fatalf("Command = %q, want %q", got[0].Command, "CMD\x00")
	}
}
