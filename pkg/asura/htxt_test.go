package asura

import (
	"bytes"
	"testing"
)

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

// buildSymbolTable constructs the trailing bytes for the optional secondary symbol-name
// table in the shape confirmed against a real Zombie Army 4 menu.asr_en: a NUL-terminated
// ASCII table name padded to the next 4-byte boundary, a uint32 body length, that many bytes
// of NUL-separated names, and a 4-byte zero footer.
func buildSymbolTable(tableName string, names []string) []byte {
	var body bytes.Buffer
	for _, n := range names {
		body.WriteString(n)
		body.WriteByte(0)
	}

	var buf bytes.Buffer
	buf.WriteString(tableName)
	buf.WriteByte(0)
	headerLen := len(tableName) + 1
	for pad := (4 - headerLen%4) % 4; pad > 0; pad-- {
		buf.WriteByte(0)
	}
	writeU32(&buf, uint32(body.Len()))
	buf.Write(body.Bytes())
	buf.Write(make([]byte, 4)) // footer
	return buf.Bytes()
}

func TestParseSymbolNames(t *testing.T) {
	f := &HTXTFile{
		Entries: []TextEntry{
			{Hash: 1493970712, Data: EncodeText("1 of 2<END>")},
			{Hash: 1522599863, Data: EncodeText("2 of 2<END>")},
		},
		Trailing: buildSymbolTable("MENU", []string{"1_OF_2", "2_OF_2"}),
	}
	original := f.Encode()

	got, err := ParseHTXT(original)
	if err != nil {
		t.Fatalf("ParseHTXT: %v", err)
	}
	if got.SymbolTableName != "MENU" {
		t.Errorf("SymbolTableName = %q, want %q", got.SymbolTableName, "MENU")
	}
	wantNames := []string{"1_OF_2", "2_OF_2"}
	if len(got.SymbolNames) != len(wantNames) {
		t.Fatalf("SymbolNames = %v, want %v", got.SymbolNames, wantNames)
	}
	for i := range wantNames {
		if got.SymbolNames[i] != wantNames[i] {
			t.Errorf("SymbolNames[%d] = %q, want %q", i, got.SymbolNames[i], wantNames[i])
		}
	}

	// Symbol-table parsing must never affect re-encoding: Trailing is always the source of
	// truth, replayed verbatim regardless of whether it was understood.
	if reEncoded := got.Encode(); string(reEncoded) != string(original) {
		t.Errorf("re-encode not byte-identical after symbol-table parsing")
	}
}

func TestParseSymbolNamesMismatchedCountIsIgnored(t *testing.T) {
	f := &HTXTFile{
		Entries: []TextEntry{
			{Hash: 1, Data: EncodeText("one")},
			{Hash: 2, Data: EncodeText("two")},
		},
		// Only one name for two entries: the shape doesn't match, so parsing should quietly
		// decline rather than guess or error.
		Trailing: buildSymbolTable("MENU", []string{"only_one"}),
	}
	original := f.Encode()

	got, err := ParseHTXT(original)
	if err != nil {
		t.Fatalf("ParseHTXT: %v", err)
	}
	if got.SymbolNames != nil {
		t.Errorf("expected no SymbolNames on a count mismatch, got %v", got.SymbolNames)
	}
	if reEncoded := got.Encode(); string(reEncoded) != string(original) {
		t.Errorf("re-encode not byte-identical")
	}
}

func TestParseSymbolNamesNoTrailingData(t *testing.T) {
	f := &HTXTFile{
		Entries: []TextEntry{{Hash: 1, Data: EncodeText("one")}},
		// No Trailing at all: plenty of real files won't have this optional table.
	}
	got, err := ParseHTXT(f.Encode())
	if err != nil {
		t.Fatalf("ParseHTXT: %v", err)
	}
	if got.SymbolNames != nil || got.SymbolTableName != "" {
		t.Errorf("expected no symbol table, got name=%q names=%v", got.SymbolTableName, got.SymbolNames)
	}
}
