package asura

import "testing"

func TestHTXTRoundTrip(t *testing.T) {
	f := &HTXTFile{
		Filesize:   1234,
		Version:    3,
		Reserved:   0,
		FileHash:   5678,
		LanguageID: 0,
		Entries: []TextEntry{
			{Hash: 111, Data: EncodeText("hello")},
			{Hash: 222, Data: EncodeText("world<END>")},
		},
		Trailing: []byte{0xDE, 0xAD, 0xBE, 0xEF},
	}
	data := f.Encode()

	got, err := ParseHTXT(data)
	if err != nil {
		t.Fatalf("ParseHTXT: %v", err)
	}
	if got.Version != f.Version || got.FileHash != f.FileHash || got.LanguageID != f.LanguageID {
		t.Fatalf("header mismatch: %+v", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Entries))
	}
	if DecodeText(got.Entries[0].Data) != "hello" {
		t.Errorf("entry 0 = %q", DecodeText(got.Entries[0].Data))
	}
	if DecodeText(got.Entries[1].Data) != "world<END>" {
		t.Errorf("entry 1 = %q", DecodeText(got.Entries[1].Data))
	}
	if string(got.Trailing) != string(f.Trailing) {
		t.Errorf("trailing mismatch: %v vs %v", got.Trailing, f.Trailing)
	}

	// Re-encoding an unmodified parse should reproduce the exact original bytes.
	if reEncoded := got.Encode(); string(reEncoded) != string(data) {
		t.Errorf("re-encode not byte-identical: got %d bytes, want %d", len(reEncoded), len(data))
	}
}

func TestHTXTOverride(t *testing.T) {
	f := &HTXTFile{
		Entries: []TextEntry{
			{Hash: 1, Data: EncodeText("one")},
			{Hash: 2, Data: EncodeText("two")},
		},
	}

	// Mismatched source text without force: guard blocks the override.
	f.Override(map[uint32]Record{
		1: {Command: "1", SourceText: "WRONG", OverrideText: "SHOULD_NOT_APPLY"},
	}, false)
	if DecodeText(f.Entries[0].Data) != "one" {
		t.Fatalf("guard failed: entry 0 = %q", DecodeText(f.Entries[0].Data))
	}

	// Matching source text: applies.
	f.Override(map[uint32]Record{
		1: {Command: "1", SourceText: "one", OverrideText: "ONE_TRANSLATED"},
	}, false)
	if DecodeText(f.Entries[0].Data) != "ONE_TRANSLATED" {
		t.Fatalf("entry 0 = %q", DecodeText(f.Entries[0].Data))
	}
	if DecodeText(f.Entries[1].Data) != "two" {
		t.Fatalf("untouched entry changed: %q", DecodeText(f.Entries[1].Data))
	}

	// force=true applies even with mismatched source text.
	f.Override(map[uint32]Record{
		2: {Command: "2", SourceText: "WRONG", OverrideText: "TWO_FORCED"},
	}, true)
	if DecodeText(f.Entries[1].Data) != "TWO_FORCED" {
		t.Fatalf("force override failed: %q", DecodeText(f.Entries[1].Data))
	}
}

func TestParseHTXTBadMagic(t *testing.T) {
	if _, err := ParseHTXT([]byte("not an asura file")); err != ErrBadMagic {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}
