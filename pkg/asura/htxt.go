package asura

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TextEntry is a single hash-keyed string from an HTXT chunk's string table.
type TextEntry struct {
	Hash uint32
	Data []byte // raw UTF-16LE bytes; decode with DecodeText, produce with EncodeText
}

// HTXTFile is a parsed HTXT chunk: header fields, its ordered string table, and any bytes
// that followed the chunk in the source file (preserved verbatim on re-encode).
type HTXTFile struct {
	Filesize   uint32
	Version    uint32
	Reserved   uint32 // the original tool's "Null" field
	FileHash   uint32
	LanguageID uint32
	Entries    []TextEntry
	Trailing   []byte

	// SymbolTableName and SymbolNames are parsed from Trailing on a best-effort basis (see
	// parseSymbolNames) — an optional secondary table some HTXT files append after their
	// string table, giving each entry's internal/build-time identifier (e.g. hash
	// 1493970712 -> "1_OF_2"). SymbolNames is parallel to Entries (same index, same order)
	// when non-nil; nil if Trailing didn't parse as this shape. This is informational only:
	// it never affects Encode, which always replays Trailing verbatim regardless of whether
	// parsing succeeded, so a misunderstood or absent symbol table can never corrupt output.
	SymbolTableName string
	SymbolNames     []string
}

// parseSymbolNames attempts to interpret trailing as the optional secondary symbol-name
// table: a NUL-terminated ASCII table name (zero-padded so the following fields starts on a
// 4-byte boundary), a uint32 byte length, that many bytes of NUL-separated ASCII names (one
// per string-table entry, in the same order), and a 4-byte zero footer. Returns ok=false if
// trailing doesn't look like this shape at all (e.g. the parsed string count doesn't match
// wantCount) — callers must treat that as "no symbol table", not an error.
func parseSymbolNames(trailing []byte, wantCount int) (tableName string, names []string, ok bool) {
	nameEnd := bytes.IndexByte(trailing, 0)
	if nameEnd < 0 {
		return "", nil, false
	}
	headerLen := nameEnd + 1
	headerLen += (4 - headerLen%4) % 4 // pad to the next 4-byte boundary
	if headerLen+4 > len(trailing) {
		return "", nil, false
	}
	size := int(binary.LittleEndian.Uint32(trailing[headerLen:]))
	bodyStart := headerLen + 4
	if size < 0 || bodyStart+size > len(trailing) {
		return "", nil, false
	}
	body := trailing[bodyStart : bodyStart+size]
	if len(body) == 0 || body[len(body)-1] != 0 {
		return "", nil, false
	}
	parts := bytes.Split(body[:len(body)-1], []byte{0})
	if len(parts) != wantCount {
		return "", nil, false
	}
	names = make([]string, len(parts))
	for i, p := range parts {
		names[i] = string(p)
	}
	return string(trailing[:nameEnd]), names, true
}

// ParseHTXT decodes a single HTXT chunk from data, which must start with the 8-byte Asura
// magic immediately followed by the "HTXT" chunk (menu/UI text files hold exactly one).
func ParseHTXT(data []byte) (*HTXTFile, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	r := &reader{data: data, pos: 8}
	tag := r.bytes(4)
	if r.err != nil {
		return nil, r.err
	}
	if string(tag) != "HTXT" {
		return nil, fmt.Errorf("asura: expected HTXT chunk, found %q", tag)
	}

	f := &HTXTFile{
		Filesize: r.u32(),
		Version:  r.u32(),
		Reserved: r.u32(),
	}
	num := r.u32()
	f.FileHash = r.u32()
	_ = r.u32() // TextStringSize: recomputed from Entries on Encode, never trusted on read
	f.LanguageID = r.u32()
	if r.err != nil {
		return nil, r.err
	}

	f.Entries = make([]TextEntry, 0, num)
	for i := uint32(0); i < num; i++ {
		hash := r.u32()
		length := r.u32()
		data := r.bytes(int(length) * 2)
		if r.err != nil {
			return nil, fmt.Errorf("asura: reading HTXT entry %d: %w", i, r.err)
		}
		f.Entries = append(f.Entries, TextEntry{Hash: hash, Data: append([]byte(nil), data...)})
	}
	f.Trailing = append([]byte(nil), r.data[r.pos:]...)
	if name, names, ok := parseSymbolNames(f.Trailing, len(f.Entries)); ok {
		f.SymbolTableName = name
		f.SymbolNames = names
	}
	return f, nil
}

// Override rewrites entries whose hash matches a key in overrides. An entry is only
// rewritten if its current decoded text equals the CSV's recorded source text, unless force
// is set — this guards against silently applying a stale or mismatched translation table.
func (f *HTXTFile) Override(overrides map[uint32]Record, force bool) {
	for i, e := range f.Entries {
		rec, ok := overrides[e.Hash]
		if !ok {
			continue
		}
		if !force && DecodeText(e.Data) != rec.SourceText {
			continue
		}
		f.Entries[i].Data = EncodeText(rec.OverrideText)
	}
}

// Encode serializes the HTXT chunk back to bytes. Filesize, Version, Reserved, FileHash and
// LanguageID are passed through unchanged; the string-table size and count are recomputed
// from Entries (matching the original tool, which never trusted the on-disk values for
// those two fields when writing).
func (f *HTXTFile) Encode() []byte {
	var textSize int
	for _, e := range f.Entries {
		textSize += len(e.Data)
	}

	var buf bytes.Buffer
	buf.Write(Magic[:])
	buf.WriteString("HTXT")
	writeU32(&buf, f.Filesize)
	writeU32(&buf, f.Version)
	writeU32(&buf, f.Reserved)
	writeU32(&buf, uint32(len(f.Entries)))
	writeU32(&buf, f.FileHash)
	writeU32(&buf, uint32(textSize))
	writeU32(&buf, f.LanguageID)
	for _, e := range f.Entries {
		writeU32(&buf, e.Hash)
		writeU32(&buf, uint32(len(e.Data)/2))
		buf.Write(e.Data)
	}
	buf.Write(f.Trailing)
	return buf.Bytes()
}
