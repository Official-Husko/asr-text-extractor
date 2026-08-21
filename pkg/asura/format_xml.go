package asura

import (
	"encoding/hex"
	"encoding/xml"
	"fmt"
)

// xmlCommand carries Command as plain text normally, but voice (DLLN) commands can include
// raw null-byte padding preserved from the original file, and XML 1.0 has no way to
// represent U+0000 at all (not even as a character reference). When Command isn't
// representable as XML text, it's hex-encoded instead and flagged with hex="true".
type xmlCommand struct {
	Hex   bool   `xml:"hex,attr,omitempty"`
	Value string `xml:",chardata"`
}

type xmlRecord struct {
	Command  xmlCommand `xml:"command"`
	Source   string     `xml:"source"`
	Override string     `xml:"override"`
}

type xmlDoc struct {
	XMLName xml.Name    `xml:"records"`
	Records []xmlRecord `xml:"record"`
}

func encodeXML(records []Record, enc Encoding) (string, error) {
	doc := xmlDoc{Records: make([]xmlRecord, len(records))}
	for i, r := range records {
		cmd := xmlCommand{Value: r.Command}
		if !isXMLSafeText(r.Command) {
			cmd.Hex = true
			cmd.Value = hex.EncodeToString([]byte(r.Command))
		}
		doc.Records[i] = xmlRecord{Command: cmd, Source: r.SourceText, Override: r.OverrideText}
	}

	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	label := "UTF-8"
	if enc == EncodingUTF16LE {
		label = "UTF-16"
	}
	return fmt.Sprintf("<?xml version=\"1.0\" encoding=\"%s\"?>\n%s\n", label, b), nil
}

func decodeXML(text string) ([]Record, error) {
	var doc xmlDoc
	if err := xml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, fmt.Errorf("asura: invalid XML records: %w", err)
	}

	records := make([]Record, len(doc.Records))
	for i, r := range doc.Records {
		command := r.Command.Value
		if r.Command.Hex {
			b, err := hex.DecodeString(command)
			if err != nil {
				return nil, fmt.Errorf("asura: invalid hex-encoded command %q: %w", command, err)
			}
			command = string(b)
		}
		records[i] = Record{Command: command, SourceText: r.Source, OverrideText: r.Override}
	}
	return records, nil
}

// isXMLSafeText reports whether s can appear as literal XML 1.0 character data. This is
// deliberately conservative — it only needs to catch the real-world case (NUL padding in
// voice commands), not exhaustively validate every restricted character.
func isXMLSafeText(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0xFFFE || r == 0xFFFF {
			return false
		}
	}
	return true
}
