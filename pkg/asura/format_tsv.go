package asura

import (
	"fmt"
	"strings"
)

// encodeTSV renders records as the original tool's interchange format:
//
//	<command>\t<sourceText>\t<overrideText>
//
// CRLF-terminated, matching File.WriteAllLines' default line ending.
func encodeTSV(records []Record) (string, error) {
	var b strings.Builder
	for _, r := range records {
		b.WriteString(r.Command)
		b.WriteByte('\t')
		b.WriteString(r.SourceText)
		b.WriteByte('\t')
		b.WriteString(r.OverrideText)
		b.WriteString("\r\n")
	}
	return b.String(), nil
}

// decodeTSV parses lines of "<command>\t<sourceText>[\t<overrideText>]". If overrideText is
// omitted, it defaults to sourceText (an unpacked, not-yet-translated line can be fed
// straight back in as its own override table).
func decodeTSV(text string) ([]Record, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	records := make([]Record, len(lines))
	for i, line := range lines {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("asura: malformed interchange line: %q", line)
		}
		rec := Record{Command: parts[0], SourceText: parts[1], OverrideText: parts[1]}
		if len(parts) == 3 {
			rec.OverrideText = parts[2]
		}
		records[i] = rec
	}
	return records, nil
}
