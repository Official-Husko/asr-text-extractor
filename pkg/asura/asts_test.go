package asura

import (
	"bytes"
	"testing"
)

// buildASTS constructs a synthetic ASTS chunk in the shape confirmed against a real Zombie
// Army 4 streamsounds file: header, a manifest of {path, size, offset} entries, the audio
// blobs themselves back-to-back, and a 4-byte zero footer.
func buildASTS(paths []string, blobs [][]byte, leadingStrayZero bool) []byte {
	var manifest bytes.Buffer
	for _, p := range paths {
		manifest.WriteString(p)
		manifest.WriteByte(0)
		// pad with a couple of extra zero bytes, matching the observed (if not fully
		// understood) per-entry padding — the parser tolerates any run length.
		manifest.Write([]byte{0, 0})
	}

	var body bytes.Buffer
	body.WriteString("ASTS")
	writeU32(&body, 0) // Filesize placeholder, unused by the parser
	writeU32(&body, 2) // Version
	writeU32(&body, 0) // Reserved
	writeU32(&body, uint32(len(paths)))
	if leadingStrayZero {
		body.WriteByte(0)
	}

	// Audio offsets are relative to the start of the whole file (Magic + this body), so
	// compute them once the manifest layout (and therefore the audio start) is fixed.
	headerLen := body.Len() + manifestEntryLen(paths)
	audioStart := 8 + headerLen // 8 for the Magic that prefixes body
	offsets := make([]int, len(blobs))
	pos := audioStart
	for i, b := range blobs {
		offsets[i] = pos
		pos += len(b)
	}

	for i, p := range paths {
		body.WriteString(p)
		body.WriteByte(0)
		body.Write([]byte{0, 0})
		writeU32(&body, uint32(len(blobs[i])))
		writeU32(&body, uint32(offsets[i]))
	}
	for _, b := range blobs {
		body.Write(b)
	}
	body.Write(make([]byte, 4)) // footer

	var out bytes.Buffer
	out.Write(Magic[:])
	out.Write(body.Bytes())
	return out.Bytes()
}

func manifestEntryLen(paths []string) int {
	n := 0
	for _, p := range paths {
		n += len(p) + 1 + 2 + 8 // path + NUL + 2 pad bytes + size(4) + offset(4)
	}
	return n
}

func TestParseASTS(t *testing.T) {
	paths := []string{`Sounds\A.wav`, `Sounds\Nested\B.wav`}
	blobs := [][]byte{
		[]byte("RIFF____WAVEfmt fakeaudio1"),
		[]byte("RIFF____WAVEfmt fakeaudio-two"),
	}
	data := buildASTS(paths, blobs, true)

	f, err := ParseASTS(data)
	if err != nil {
		t.Fatalf("ParseASTS: %v", err)
	}
	if f.Version != 2 {
		t.Errorf("Version = %d, want 2", f.Version)
	}
	if len(f.Entries) != len(paths) {
		t.Fatalf("got %d entries, want %d", len(f.Entries), len(paths))
	}
	for i, e := range f.Entries {
		if e.Path != paths[i] {
			t.Errorf("entry %d Path = %q, want %q", i, e.Path, paths[i])
		}
		if !bytes.Equal(e.Data, blobs[i]) {
			t.Errorf("entry %d Data = %q, want %q", i, e.Data, blobs[i])
		}
	}
}

func TestParseASTSNoLeadingStrayZero(t *testing.T) {
	// The parser must not depend on the stray zero byte always being present.
	paths := []string{`Sounds\A.wav`}
	blobs := [][]byte{[]byte("RIFF____WAVEfmt fake")}
	data := buildASTS(paths, blobs, false)

	f, err := ParseASTS(data)
	if err != nil {
		t.Fatalf("ParseASTS: %v", err)
	}
	if len(f.Entries) != 1 || f.Entries[0].Path != paths[0] || !bytes.Equal(f.Entries[0].Data, blobs[0]) {
		t.Fatalf("unexpected result: %+v", f.Entries)
	}
}

func TestParseASTSBadMagic(t *testing.T) {
	if _, err := ParseASTS([]byte("not an asura file")); err != ErrBadMagic {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

func TestParseASTSOutOfRangeOffset(t *testing.T) {
	// A manifest entry declaring far more audio data than the file actually has.
	var body bytes.Buffer
	body.WriteString("ASTS")
	writeU32(&body, 0)
	writeU32(&body, 2)
	writeU32(&body, 0)
	writeU32(&body, 1) // one entry
	body.WriteByte(0)
	body.WriteString(`Sounds\A.wav`)
	body.WriteByte(0)
	body.Write([]byte{0, 0})
	writeU32(&body, 999999) // size: far larger than any data actually present
	writeU32(&body, 0)      // offset

	var data bytes.Buffer
	data.Write(Magic[:])
	data.Write(body.Bytes())

	if _, err := ParseASTS(data.Bytes()); err == nil {
		t.Fatal("expected an error for an out-of-range audio slice")
	}
}
