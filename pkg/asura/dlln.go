package asura

import (
	"bytes"
	"fmt"
)

// Source and override language ids used by convention in Version 5 DLLN entries.
const (
	voiceSourceLangID   = 0
	voiceOverrideLangID = 7
)

// UnpackVoice scans data (which must start with the Asura magic) for DLLN chunks and decodes
// each one's command name plus its source/override display text into a Record. Version 5
// entries carry both source and override text (looked up by the language ids above); Version
// 4 entries carry one string used as both; any other version yields a Record with empty text
// (the format for that version isn't understood, but the entry is still surfaced since its
// command name and framing are known).
//
// The scan is deliberately a byte-by-byte search for the next literal "DLLN" tag rather than
// a walk over a chunk directory: that's how the original reverse-engineered tool works. Voice
// files interleave DLLN entries with other, not-yet-understood binary data, and entries whose
// version this tool doesn't know how to parse (anything but 4 or 5) are skipped with empty
// text rather than aborting — the scan just resumes searching for the next "DLLN" tag, which
// is what keeps it resynchronized regardless of any given entry's internal layout.
func UnpackVoice(data []byte) ([]Record, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	r := &reader{data: data, pos: 8}

	var entries []Record
	for findNextDLLN(r) {
		_ = r.u32() // Filesize: informational only, never used to seek
		version := r.u32()
		_ = r.u32() // Reserved ("Null" in the original)
		command := scanCommand(r)
		r.bytes(25) // opaque code1/code2/timestamp1/pad/timestamp2/skip4/command2 blob

		entry := Record{Command: string(command)}
		switch version {
		case 5:
			num := r.u32()
			for i := uint32(0); i < num && r.err == nil; i++ {
				lang := r.u32()
				length := r.u32()
				text := r.bytes(int(length) * 2)
				switch lang {
				case voiceSourceLangID:
					entry.SourceText = DecodeText(text)
				case voiceOverrideLangID:
					entry.OverrideText = DecodeText(text)
				}
			}
		case 4:
			length := r.u32()
			text := r.bytes(int(length) * 2)
			if r.err == nil {
				entry.SourceText = DecodeText(text)
				entry.OverrideText = entry.SourceText
			}
		}
		if r.err != nil {
			return nil, fmt.Errorf("asura: truncated DLLN entry for command %q: %w", entry.Command, r.err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// OverrideVoice rewrites every DLLN entry in data using overrides (keyed by the entry's
// Command, including its original null padding), copying all other bytes through unchanged.
// Mirroring the original tool, every DLLN entry in data must be Version 4 — hitting any other
// version aborts the whole operation, since only Version 4's single-string layout is
// understood well enough to safely rewrite.
func OverrideVoice(data []byte, overrides map[string]Record, force bool) ([]byte, error) {
	if !CheckMagic(data) {
		return nil, ErrBadMagic
	}
	var out bytes.Buffer
	out.Write(data[:8])

	r := &reader{data: data, pos: 8}
	for {
		start := r.pos
		if !findNextDLLN(r) {
			out.Write(data[start:])
			break
		}
		out.Write(data[start : r.pos-4]) // bytes skipped by the scan, copied through verbatim
		out.WriteString("DLLN")

		_ = r.u32() // Filesize: recomputed below, never trusted on read
		version := r.u32()
		if r.err == nil && version != 4 {
			return nil, fmt.Errorf("asura: voice override only supports Version 4 DLLN entries, found Version %d", version)
		}
		reserved := r.u32()
		command := scanCommand(r)
		opaque := r.bytes(25)
		length := r.u32()
		textData := r.bytes(int(length) * 2)
		if r.err != nil {
			return nil, fmt.Errorf("asura: truncated DLLN entry: %w", r.err)
		}

		text := DecodeText(textData)
		if rec, ok := overrides[string(command)]; ok {
			if force || rec.SourceText == text {
				textData = EncodeText(rec.OverrideText)
			}
		}

		// Reproduces the original tool's exact Filesize formula for this chunk (4 constant
		// bytes + Version + Reserved + command + opaque blob + length prefix + text data).
		// It looks like it double-counts Version/Reserved, but it's what the real game's
		// parser was reverse-engineered against — do not "simplify" it.
		size := 4 + 4 + 4 + len(command) + len(opaque) + 4 + len(textData)
		writeU32(&out, uint32(size))
		writeU32(&out, version)
		writeU32(&out, reserved)
		out.Write(command)
		out.Write(opaque)
		writeU32(&out, uint32(len(textData)/2))
		out.Write(textData)
	}
	return out.Bytes(), nil
}

// findNextDLLN advances r past the next literal "DLLN" tag in its remaining data, leaving
// r positioned just after the tag, and reports whether one was found. It is a 1-byte-stride
// substring search: voice files have no chunk directory, so the only reliable way to find
// the next entry is to look for its tag byte-by-byte.
func findNextDLLN(r *reader) bool {
	for r.remaining() >= 4 {
		if bytes.Equal(r.data[r.pos:r.pos+4], []byte("DLLN")) {
			r.pos += 4
			return true
		}
		r.pos++
	}
	r.pos = len(r.data)
	return false
}

// scanCommand replicates the original tool's alignment-padded null-terminated ASCII command
// scan: read 4 bytes at a time until a group's last byte is 0, then keep consuming any further
// single trailing zero bytes, then rewind and re-read that whole span as the command. The
// returned bytes include the ASCII text and all of its null padding.
func scanCommand(r *reader) []byte {
	start := r.pos
	count := 0
	for {
		b := r.bytes(4)
		if r.err != nil {
			return nil
		}
		count += 4
		if b[3] == 0 {
			break
		}
	}
	for {
		b := r.byte()
		if r.err != nil {
			return nil
		}
		if b != 0 {
			break
		}
		count++
	}
	r.pos = start
	return r.bytes(count)
}
