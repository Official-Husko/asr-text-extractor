package asura

import (
	"fmt"
	"os"
	"strconv"
	"unicode/utf16"
)

// Record is one interchange entry, format-agnostic: for HTXT (text) tables Command is the
// decimal string-table hash; for DLLN (voice) entries it's the ASCII command name. If
// OverrideText equals SourceText, the entry hasn't been translated yet.
type Record struct {
	Command      string
	SourceText   string
	OverrideText string
}

// Code parses Command as the numeric hash key used by the HTXT (text) interchange format.
func (r Record) Code() (uint32, error) {
	v, err := strconv.ParseUint(r.Command, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("asura: invalid command code %q: %w", r.Command, err)
	}
	return uint32(v), nil
}

// Format selects an interchange file's on-disk shape.
type Format string

const (
	FormatTSV  Format = "txt"  // the original tool's tab-separated lines
	FormatCSV  Format = "csv"  // proper comma-separated, RFC 4180 quoting
	FormatJSON Format = "json" // ordered array of {command, source, override} objects
	FormatYAML Format = "yaml" // same shape as JSON
	FormatXML  Format = "xml"  // <records><record><command/><source/><override/></record>...
)

// ParseFormat validates a --format flag value.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case FormatTSV, FormatCSV, FormatJSON, FormatYAML, FormatXML:
		return Format(s), nil
	default:
		return "", fmt.Errorf("asura: unknown format %q (want txt, csv, json, yaml, or xml)", s)
	}
}

// Encoding selects an interchange file's text encoding. EncodingAuto picks each format's
// natural default: UTF-16LE+BOM for FormatTSV (matching the original tool's Excel/Notepad
// workflow), UTF-8 for everything else.
type Encoding string

const (
	EncodingAuto    Encoding = ""
	EncodingUTF8    Encoding = "utf8"
	EncodingUTF16LE Encoding = "utf16le"
)

// ParseEncoding validates an --encoding flag value. An empty string is EncodingAuto.
func ParseEncoding(s string) (Encoding, error) {
	switch Encoding(s) {
	case EncodingAuto, EncodingUTF8, EncodingUTF16LE:
		return Encoding(s), nil
	default:
		return "", fmt.Errorf("asura: unknown encoding %q (want utf8 or utf16le)", s)
	}
}

func resolveEncoding(format Format, enc Encoding) Encoding {
	if enc != EncodingAuto {
		return enc
	}
	if format == FormatTSV {
		return EncodingUTF16LE
	}
	return EncodingUTF8
}

// WriteRecords serializes records as format/enc and writes them to path. EncodingAuto picks
// the format's natural default encoding.
func WriteRecords(path string, records []Record, format Format, enc Encoding) error {
	resolved := resolveEncoding(format, enc)
	text, err := encodeFormat(records, format, resolved)
	if err != nil {
		return err
	}
	return writeText(path, text, resolved)
}

// ReadRecords reads and parses an interchange file written by WriteRecords (or a compatible
// hand-edited file in the same format/encoding).
func ReadRecords(path string, format Format, enc Encoding) ([]Record, error) {
	resolved := resolveEncoding(format, enc)
	text, err := readText(path, resolved)
	if err != nil {
		return nil, err
	}
	return decodeFormat(text, format)
}

func encodeFormat(records []Record, format Format, enc Encoding) (string, error) {
	switch format {
	case FormatTSV:
		return encodeTSV(records)
	case FormatCSV:
		return encodeCSV(records)
	case FormatJSON:
		return encodeJSON(records)
	case FormatYAML:
		return encodeYAML(records)
	case FormatXML:
		return encodeXML(records, enc)
	default:
		return "", fmt.Errorf("asura: unknown format %q", format)
	}
}

func decodeFormat(text string, format Format) ([]Record, error) {
	switch format {
	case FormatTSV:
		return decodeTSV(text)
	case FormatCSV:
		return decodeCSV(text)
	case FormatJSON:
		return decodeJSON(text)
	case FormatYAML:
		return decodeYAML(text)
	case FormatXML:
		return decodeXML(text)
	default:
		return nil, fmt.Errorf("asura: unknown format %q", format)
	}
}

// writeText writes text to path, transcoding to UTF-16LE+BOM when enc requests it.
func writeText(path, text string, enc Encoding) error {
	data := []byte(text)
	if enc == EncodingUTF16LE {
		data = utf16LEBytesWithBOM(text)
	}
	return os.WriteFile(path, data, 0o644)
}

// readText reads path back to a string, decoding a UTF-16LE+BOM file regardless of enc (a
// BOM unambiguously identifies the encoding), and otherwise treating the bytes as UTF-8.
func readText(path string, enc Encoding) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return utf16LEStringFromBOM(data)
	}
	if enc == EncodingUTF16LE {
		return "", fmt.Errorf("asura: %s: expected UTF-16LE (with BOM) but found none", path)
	}
	return string(data), nil
}

func utf16LEBytesWithBOM(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, 2+len(units)*2)
	data[0], data[1] = 0xFF, 0xFE
	copy(data[2:], encodeUTF16LE(units))
	return data
}

func utf16LEStringFromBOM(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", fmt.Errorf("asura: odd-length UTF-16LE data")
	}
	units := decodeUTF16LE(data)
	if len(units) > 0 && units[0] == 0xFEFF {
		units = units[1:]
	}
	return string(utf16.Decode(units)), nil
}
