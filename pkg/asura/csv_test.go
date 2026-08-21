package asura

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCSVRecord(t *testing.T) {
	rec, err := ParseCSVRecord("123\tsource text")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Command != "123" || rec.SourceText != "source text" || rec.OverrideText != "source text" {
		t.Fatalf("2-column parse mismatch: %+v", rec)
	}

	rec, err = ParseCSVRecord("123\tsource text\toverride text")
	if err != nil {
		t.Fatal(err)
	}
	if rec.OverrideText != "override text" {
		t.Fatalf("3-column parse mismatch: %+v", rec)
	}

	if _, err := ParseCSVRecord("nocolumns"); err == nil {
		t.Fatal("expected error for malformed line")
	}
}

func TestCSVRecordCode(t *testing.T) {
	rec := CSVRecord{Command: "42"}
	code, err := rec.Code()
	if err != nil || code != 42 {
		t.Fatalf("Code() = %d, %v", code, err)
	}
	if _, err := (CSVRecord{Command: "not-a-number"}).Code(); err == nil {
		t.Fatal("expected error for non-numeric command")
	}
}

func TestUTF16LERoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	lines := []string{
		"123\thello",
		"456\tworld<END>",
		"789\t<HIGHLIGHT_SET_START>colored<HIGHLIGHT_SET_END>",
	}

	if err := WriteUTF16LELines(path, lines); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xFE {
		t.Fatalf("missing UTF-16LE BOM, got % x", data[:2])
	}

	got, err := ReadUTF16LELines(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(lines) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(lines), got)
	}
	for i := range lines {
		if got[i] != lines[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], lines[i])
		}
	}
}
