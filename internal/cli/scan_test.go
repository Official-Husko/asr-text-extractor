package cli

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func TestWriteTreeRendersClassicConnectors(t *testing.T) {
	root := &treeNode{
		name: "root",
		children: []*treeNode{
			{name: "dirA", children: []*treeNode{
				{name: "leaf1"},
				{name: "leaf2"},
			}},
			{name: "fileB"},
		},
	}

	var buf bytes.Buffer
	if err := writeTree(&buf, root); err != nil {
		t.Fatalf("writeTree: %v", err)
	}

	want := "root\n" +
		"├── dirA\n" +
		"│   ├── leaf1\n" +
		"│   └── leaf2\n" +
		"└── fileB\n"
	if got := buf.String(); got != want {
		t.Errorf("writeTree output =\n%s\nwant\n%s", got, want)
	}
}

// buildSymbolNameTrailing constructs the optional secondary symbol-name table HTXTFile.Trailing
// can carry (see parseSymbolNames's doc comment in pkg/asura/htxt.go): a NUL-terminated table
// name padded to a 4-byte boundary, a uint32 byte length, that many bytes of NUL-separated
// names (one per string-table entry, in order) ending in a NUL, then a 4-byte zero footer.
// SymbolNames itself is a parse-only, derived field — Encode() only ever replays Trailing
// verbatim — so a fixture wanting scan to see symbol names has to build this by hand rather
// than just setting HTXTFile.SymbolNames directly.
func buildSymbolNameTrailing(tableName string, names []string) []byte {
	var trailing []byte
	trailing = append(trailing, tableName...)
	trailing = append(trailing, 0)
	for len(trailing)%4 != 0 {
		trailing = append(trailing, 0)
	}

	var body []byte
	for _, n := range names {
		body = append(body, n...)
		body = append(body, 0)
	}

	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(body)))
	trailing = append(trailing, size[:]...)
	trailing = append(trailing, body...)
	trailing = append(trailing, 0, 0, 0, 0) // footer
	return trailing
}

func TestScanFindsHTXTStringsByHashAndSymbolName(t *testing.T) {
	f := &asura.HTXTFile{
		LanguageID: 1,
		Entries: []asura.TextEntry{
			{Hash: 111, Data: asura.EncodeText("hello")},
			{Hash: 222, Data: asura.EncodeText("world")},
		},
		Trailing: buildSymbolNameTrailing("Symbols", []string{"GREETING", ""}),
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Menu.asr_en"), f.Encode(), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("just some text"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	root := &treeNode{name: "dir"}
	sc := newScanner()
	if err := scanInto(sc, root, dir); err != nil {
		t.Fatalf("scanInto: %v", err)
	}
	sc.wait()

	var buf bytes.Buffer
	if err := writeTree(&buf, root); err != nil {
		t.Fatalf("writeTree: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "Menu.asr_en") {
		t.Errorf("missing the HTXT file itself; output:\n%s", out)
	}
	if !strings.Contains(out, "strings (2)") {
		t.Errorf("missing the strings count; output:\n%s", out)
	}
	if !strings.Contains(out, "111 (GREETING)") {
		t.Errorf("missing hash 111 with its symbol name; output:\n%s", out)
	}
	if !strings.Contains(out, "222\n") {
		t.Errorf("missing hash 222 (no symbol name, so no parenthetical); output:\n%s", out)
	}
	if !strings.Contains(out, "readme.txt") {
		t.Errorf("missing the unrecognized plain file; output:\n%s", out)
	}
	// The plain file has no recognized magic, so it must not gain a synthetic entry list.
	if strings.Contains(out, "readme.txt\n│") || strings.Contains(out, "readme.txt\n    ") {
		t.Errorf("unrecognized file unexpectedly got child entries; output:\n%s", out)
	}
}

func TestScanRecursesIntoSubdirectories(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested", "leaf.dat"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	root := &treeNode{name: "dir"}
	sc := newScanner()
	if err := scanInto(sc, root, dir); err != nil {
		t.Fatalf("scanInto: %v", err)
	}
	sc.wait()

	var buf bytes.Buffer
	if err := writeTree(&buf, root); err != nil {
		t.Fatalf("writeTree: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"sub", "nested", "leaf.dat"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in nested output:\n%s", want, out)
		}
	}
}
