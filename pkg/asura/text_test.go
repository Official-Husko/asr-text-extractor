package asura

import "testing"

func TestGlyphTableRoundTrip(t *testing.T) {
	// Every tag individually.
	for _, m := range glyphTable {
		if got := DecodeText(EncodeText(m.tag)); got != m.tag {
			t.Errorf("round trip mismatch for %s: got %q", m.tag, got)
		}
	}

	// All tags concatenated, to catch any cross-pattern interference the
	// per-tag test above wouldn't see (e.g. one tag's glyph output being
	// re-matched by a neighboring pattern).
	var all string
	for _, m := range glyphTable {
		all += m.tag
	}
	if got := DecodeText(EncodeText(all)); got != all {
		t.Fatalf("concatenated round trip mismatch:\n got: %q\nwant: %q", got, all)
	}
}

func TestEncodeDecodeText(t *testing.T) {
	cases := []string{
		"hello world",
		"1 of 2<END>",
		"<HIGHLIGHT_SET_START>colored<HIGHLIGHT_SET_END>",
		"<HIGHLIGHT_END>",
		"press <INPUT_FRONTEND_A> to continue",
		"line1<NL>line2<TAB>indented<NR>",
		"<HIGHLIGHT2_START>MOUSE<HIGHLIGHT2_END> settings",
	}
	for _, s := range cases {
		if got := DecodeText(EncodeText(s)); got != s {
			t.Errorf("round trip mismatch for %q: got %q", s, got)
		}
	}
}
