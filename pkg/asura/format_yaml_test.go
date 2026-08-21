package asura

import "testing"

func TestYAMLRoundTrip(t *testing.T) {
	records := []Record{
		{Command: "1493970712", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"},
		{Command: "1522599863", SourceText: "2 of 2<END>", OverrideText: "translated"},
	}

	text, err := encodeYAML(records)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeYAML(text)
	if err != nil {
		t.Fatalf("decodeYAML: %v\ninput:\n%s", err, text)
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

func TestYAMLEmbeddedNUL(t *testing.T) {
	records := []Record{{Command: "CMD\x00", SourceText: "hi", OverrideText: "hi"}}

	text, err := encodeYAML(records)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeYAML(text)
	if err != nil {
		t.Fatalf("decodeYAML: %v\ninput:\n%s", err, text)
	}
	if got[0].Command != "CMD\x00" {
		t.Fatalf("Command = %q, want %q", got[0].Command, "CMD\x00")
	}
}
