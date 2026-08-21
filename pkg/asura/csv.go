package asura

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
)

// CSVRecord is one parsed line of the tab-separated interchange format:
//
//	<command>\t<sourceText>[\t<overrideText>]
//
// If overrideText is omitted, it defaults to sourceText (an unpacked, not-yet-translated
// line can be fed straight back in as its own override table).
type CSVRecord struct {
	Command      string
	SourceText   string
	OverrideText string
}

// ParseCSVRecord parses a single interchange line.
func ParseCSVRecord(line string) (CSVRecord, error) {
	parts := strings.Split(line, "\t")
	if len(parts) < 2 {
		return CSVRecord{}, fmt.Errorf("asura: malformed interchange line: %q", line)
	}
	rec := CSVRecord{Command: parts[0], SourceText: parts[1], OverrideText: parts[1]}
	if len(parts) >= 3 {
		rec.OverrideText = parts[2]
	}
	return rec, nil
}

// Line renders the record back to its tab-separated interchange form.
func (r CSVRecord) Line() string {
	return r.Command + "\t" + r.SourceText + "\t" + r.OverrideText
}

// Code parses Command as the numeric hash key used by the HTXT (text) interchange format.
func (r CSVRecord) Code() (uint32, error) {
	v, err := strconv.ParseUint(r.Command, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("asura: invalid command code %q: %w", r.Command, err)
	}
	return uint32(v), nil
}

// ReadUTF16LELines reads a UTF-16LE text file (an optional leading BOM is skipped) and
// splits it into lines on \r\n or \n, matching the original tool's
// StreamReader(path, Encoding.Unicode) + ReadLine() behavior.
func ReadUTF16LELines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data)%2 != 0 {
		return nil, fmt.Errorf("asura: %s: odd-length UTF-16LE file", path)
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
	}
	if len(units) > 0 && units[0] == 0xFEFF {
		units = units[1:]
	}
	text := strings.ReplaceAll(string(utf16.Decode(units)), "\r\n", "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// WriteUTF16LELines writes lines to path as UTF-16LE text with a leading BOM, CRLF-terminated,
// matching the original tool's File.WriteAllLines(path, lines, Encoding.Unicode) behavior so
// output stays compatible with the same Excel/Notepad translation workflows.
func WriteUTF16LELines(path string, lines []string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()

	w := bufio.NewWriter(f)
	writeUTF16Unit(w, 0xFEFF)
	for _, line := range lines {
		writeUTF16String(w, line)
		writeUTF16String(w, "\r\n")
	}
	return w.Flush()
}

func writeUTF16String(w *bufio.Writer, s string) {
	for _, u := range utf16.Encode([]rune(s)) {
		writeUTF16Unit(w, u)
	}
}

func writeUTF16Unit(w *bufio.Writer, u uint16) {
	w.WriteByte(byte(u))
	w.WriteByte(byte(u >> 8))
}
