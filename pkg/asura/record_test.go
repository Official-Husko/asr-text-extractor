package asura

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordCode(t *testing.T) {
	rec := Record{Command: "42"}
	code, err := rec.Code()
	if err != nil || code != 42 {
		t.Fatalf("Code() = %d, %v", code, err)
	}
	if _, err := (Record{Command: "not-a-number"}).Code(); err == nil {
		t.Fatal("expected error for non-numeric command")
	}
}

func TestParseFormat(t *testing.T) {
	for _, s := range []string{"txt", "csv", "json", "yaml", "xml"} {
		if _, err := ParseFormat(s); err != nil {
			t.Errorf("ParseFormat(%q): %v", s, err)
		}
	}
	if _, err := ParseFormat("exe"); err == nil {
		t.Error("expected error for unknown format")
	}
}

func TestResolveEncoding(t *testing.T) {
	if got := resolveEncoding(FormatTSV, EncodingAuto); got != EncodingUTF16LE {
		t.Errorf("txt default = %s, want utf16le", got)
	}
	for _, f := range []Format{FormatCSV, FormatJSON, FormatYAML, FormatXML} {
		if got := resolveEncoding(f, EncodingAuto); got != EncodingUTF8 {
			t.Errorf("%s default = %s, want utf8", f, got)
		}
	}
	if got := resolveEncoding(FormatJSON, EncodingUTF16LE); got != EncodingUTF16LE {
		t.Errorf("explicit encoding not honored: got %s", got)
	}
}

func TestWriteReadRecordsEachFormat(t *testing.T) {
	records := []Record{
		{Command: "1493970712", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"},
		{Command: "1522599863", SourceText: "2 of 2<END>", OverrideText: "translated"},
	}
	dir := t.TempDir()

	for _, format := range []Format{FormatTSV, FormatCSV, FormatJSON, FormatYAML, FormatXML} {
		t.Run(string(format), func(t *testing.T) {
			path := filepath.Join(dir, "test."+string(format))
			if err := WriteRecords(path, records, format, EncodingAuto); err != nil {
				t.Fatalf("WriteRecords: %v", err)
			}
			got, err := ReadRecords(path, format, EncodingAuto)
			if err != nil {
				t.Fatalf("ReadRecords: %v", err)
			}
			if len(got) != len(records) {
				t.Fatalf("got %d records, want %d", len(got), len(records))
			}
			for i := range records {
				if got[i] != records[i] {
					t.Errorf("record %d = %+v, want %+v", i, got[i], records[i])
				}
			}
		})
	}
}

func TestWriteRecordsEncodingOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	records := []Record{{Command: "1", SourceText: "hi", OverrideText: "hi"}}

	// JSON defaults to UTF-8; force UTF-16LE and confirm the file actually has a BOM and
	// still round-trips (ReadRecords auto-detects the BOM regardless of the format's
	// default, so EncodingAuto is enough on read).
	if err := WriteRecords(path, records, FormatJSON, EncodingUTF16LE); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xFE {
		t.Fatalf("expected a UTF-16LE BOM, got % x", data[:2])
	}
	got, err := ReadRecords(path, FormatJSON, EncodingAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != records[0] {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}
