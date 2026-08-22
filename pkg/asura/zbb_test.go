package asura

import (
	"bytes"
	"compress/zlib"
	"testing"
)

// buildZbb constructs a synthetic "AsuraZbb"-wrapped file from one or more plaintext chunks,
// in the shape confirmed against a real 255MB Zombie Army 4 level package: magic, an unused
// field, the total decompressed size, then [compressedSize][decompressedSize][zlib data] per
// chunk.
func buildZbb(t *testing.T, chunks ...[]byte) []byte {
	t.Helper()
	var total int
	var compressed [][]byte
	for _, c := range chunks {
		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		if _, err := zw.Write(c); err != nil {
			t.Fatalf("zlib write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("zlib close: %v", err)
		}
		compressed = append(compressed, zbuf.Bytes())
		total += len(c)
	}

	var out bytes.Buffer
	out.Write(zbbMagic[:])
	writeU32(&out, 0) // fieldA, unused by DecompressZbb
	writeU32(&out, uint32(total))
	for i, c := range compressed {
		writeU32(&out, uint32(len(c)))
		writeU32(&out, uint32(len(chunks[i])))
		out.Write(c)
	}
	return out.Bytes()
}

func TestDecompressZbb(t *testing.T) {
	chunk0 := bytes.Repeat([]byte("hello asura "), 100)
	chunk1 := []byte("second chunk, shorter")
	raw := buildZbb(t, chunk0, chunk1)

	got, err := DecompressZbb(raw)
	if err != nil {
		t.Fatalf("DecompressZbb: %v", err)
	}
	want := append(append([]byte{}, chunk0...), chunk1...)
	if !bytes.Equal(got, want) {
		t.Fatalf("decompressed mismatch: got %d bytes, want %d bytes", len(got), len(want))
	}
}

func TestDecompressZbbSingleChunk(t *testing.T) {
	chunk := []byte("just one chunk")
	raw := buildZbb(t, chunk)

	got, err := DecompressZbb(raw)
	if err != nil {
		t.Fatalf("DecompressZbb: %v", err)
	}
	if !bytes.Equal(got, chunk) {
		t.Fatalf("got %q, want %q", got, chunk)
	}
}

func TestDecompressZbbBadMagic(t *testing.T) {
	if _, err := DecompressZbb([]byte("Asura   not zbb")); err == nil {
		t.Fatal("expected an error for missing AsuraZbb magic")
	}
}

func TestDecompressZbbTruncatedHeader(t *testing.T) {
	if _, err := DecompressZbb(zbbMagic[:]); err == nil {
		t.Fatal("expected an error for a truncated header")
	}
}

func TestDecompressZbbSizeMismatch(t *testing.T) {
	raw := buildZbb(t, []byte("some data"))
	// Corrupt the declared total decompressed size (bytes 12:16) so it disagrees with what
	// the chunk table actually produces.
	raw[12] = 0xFF
	raw[13] = 0xFF
	if _, err := DecompressZbb(raw); err == nil {
		t.Fatal("expected an error when declared total size doesn't match decompressed output")
	}
}

func TestCheckZbbMagic(t *testing.T) {
	if !CheckZbbMagic(zbbMagic[:]) {
		t.Error("CheckZbbMagic(zbbMagic) = false, want true")
	}
	if CheckZbbMagic(Magic[:]) {
		t.Error("CheckZbbMagic(Magic) = true, want false")
	}
	if CheckZbbMagic(nil) {
		t.Error("CheckZbbMagic(nil) = true, want false")
	}
}
