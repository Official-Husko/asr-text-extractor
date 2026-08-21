package asura

import (
	"bytes"
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
