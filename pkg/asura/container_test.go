package asura

import "testing"

func TestCheckMagic(t *testing.T) {
	valid := append(Magic[:], []byte("HTXT")...)
	if !CheckMagic(valid) {
		t.Error("expected valid magic to be recognized")
	}
	if CheckMagic([]byte("not an asura file")) {
		t.Error("expected invalid magic to be rejected")
	}
	if CheckMagic([]byte("short")) {
		t.Error("expected a too-short buffer to be rejected")
	}
}
