package asura

import (
	"bytes"
	"strings"
	"testing"
)

// astsPadding mirrors ParseASTS's own padding formula (see asts.go): the number of zero bytes
// following a path's NUL terminator before the size field starts, confirmed against 15,509 real
// Sniper Elite 5 entries and 581 real Zombie Army 4 entries — always 1 to 4 bytes, never
// determined by scanning for the next non-zero byte.
func astsPadding(pathLen int) int {
	return (4-(pathLen+1)%4)%4 + 1
}

// buildASTS constructs a synthetic ASTS chunk in the shape confirmed against real Zombie Army 4,
// Sniper Elite 5, and Sniper Elite Resistance streamsounds files: header, a mandatory
// embedded-audio flag byte (0, matching every real self-contained sample), a manifest of
// {path, size, offset} entries, the audio blobs themselves back-to-back, and a 4-byte zero
// footer.
func buildASTS(paths []string, blobs [][]byte) []byte {
	var manifest bytes.Buffer
	for _, p := range paths {
		manifest.WriteString(p)
		manifest.WriteByte(0)
		manifest.Write(make([]byte, astsPadding(len(p))))
	}

	var body bytes.Buffer
	body.WriteString("ASTS")
	writeU32(&body, 0) // Filesize placeholder, unused by the parser
	writeU32(&body, 2) // Version
	writeU32(&body, 0) // Reserved
	writeU32(&body, uint32(len(paths)))
	body.WriteByte(0) // embedded-audio flag: 0 = self-contained

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
		body.Write(make([]byte, astsPadding(len(p))))
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
		n += len(p) + 1 + astsPadding(len(p)) + 8 // path + NUL + padding + size(4) + offset(4)
	}
	return n
}

func TestParseASTS(t *testing.T) {
	paths := []string{`Sounds\A.wav`, `Sounds\Nested\B.wav`}
	blobs := [][]byte{
		[]byte("RIFF____WAVEfmt fakeaudio1"),
		[]byte("RIFF____WAVEfmt fakeaudio-two"),
	}
	data := buildASTS(paths, blobs)

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

func TestParseASTSReferenceOnlyManifest(t *testing.T) {
	// A flag byte of 1 (confirmed in every real ".ssm"-named companion manifest checked across
	// Zombie Army 4, Sniper Elite 5, and Sniper Elite Resistance) means this chunk has no
	// embedded audio at all — ParseASTS must reject it with a specific, actionable error rather
	// than fail deep inside entry parsing with a confusing out-of-range message.
	var body bytes.Buffer
	body.WriteString("ASTS")
	writeU32(&body, 0)
	writeU32(&body, 2)
	writeU32(&body, 0)
	writeU32(&body, 1) // count
	body.WriteByte(1)  // embedded-audio flag: 1 = reference-only
	body.WriteString(`Sounds\A.wav`)
	body.WriteByte(0)

	var data bytes.Buffer
	data.Write(Magic[:])
	data.Write(body.Bytes())

	_, err := ParseASTS(data.Bytes())
	if err == nil {
		t.Fatal("expected an error for a reference-only (flag=1) manifest")
	}
	if !strings.Contains(err.Error(), "no embedded audio") {
		t.Fatalf("error = %v, want a message about no embedded audio", err)
	}
}

func TestParseASTSTruncatedBeforeFlag(t *testing.T) {
	var body bytes.Buffer
	body.WriteString("ASTS")
	writeU32(&body, 0)
	writeU32(&body, 2)
	writeU32(&body, 0)
	writeU32(&body, 0) // count, then nothing else — no flag byte

	var data bytes.Buffer
	data.Write(Magic[:])
	data.Write(body.Bytes())

	if _, err := ParseASTS(data.Bytes()); err == nil {
		t.Fatal("expected an error for a chunk truncated before its flag byte")
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
	path := `Sounds\A.wav`
	body.WriteString(path)
	body.WriteByte(0)
	body.Write(make([]byte, astsPadding(len(path))))
	writeU32(&body, 999999) // size: far larger than any data actually present
	writeU32(&body, 0)      // offset

	var data bytes.Buffer
	data.Write(Magic[:])
	data.Write(body.Bytes())

	if _, err := ParseASTS(data.Bytes()); err == nil {
		t.Fatal("expected an error for an out-of-range audio slice")
	}
}
