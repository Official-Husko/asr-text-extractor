package asura

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

// glyphTable pairs a human-editable <TAG> placeholder with the raw UTF-16 code-unit
// sequence it stands in for (gamepad button glyphs, and a few control characters and
// multi-codepoint "highlight" control sequences from the private-use area).
//
// Listed longest-sequence-first as documentation of intent: a decoded sequence should never
// be matched by a shorter pattern that is really just its prefix. In practice every sequence
// here is already mutually exclusive by its second code unit, so strings.Replacer (which
// matches in argument order at each position, not longest-match) never actually depends on
// this ordering — but keeping it defends against a future entry breaking that invariant.
var glyphTable = []struct {
	tag string
	seq string
}{
	{"<HIGHLIGHT_END>", "\uE001\u0001\uE002"},
	{"<HIGHLIGHT_SET_START>", "\uE001\u0002"},
	{"<HIGHLIGHT_RGB_SET_START>", "\uE001\u0003"},
	{"<HIGHLIGHT_SET_END>", "\uE002"},
	{"<NL>", "\n"},
	{"<NR>", "\r"},
	{"<TAB>", "\t"},
	{"<END>", "\x00"},
	{"<INPUT_FRONTEND_A>", "\uE800"},
	{"<INPUT_FRONTEND_B>", "\uE801"},
	{"<INPUT_FRONTEND_UP>", "\uE802"},
	{"<INPUT_FRONTEND_DOWN>", "\uE803"},
	{"<INPUT_FRONTEND_LEFT>", "\uE804"},
	{"<INPUT_FRONTEND_RIGHT>", "\uE805"},
	{"<INPUT_FRONTEND_X>", "\uE806"},
	{"<INPUT_FRONTEND_Y>", "\uE807"},
	{"<INPUT_FRONTEND_LS>", "\uE808"},
	{"<INPUT_FRONTEND_SELECT>", "\uE809"},
	{"<INPUT_FRONTEND_START>", "\uE80A"},
	{"<INPUT_FRONTEND_LB>", "\uE80B"},
	{"<INPUT_FRONTEND_RB>", "\uE80C"},
	{"<INPUT_FRONTEND_LT>", "\uE80D"},
	{"<INPUT_FRONTEND_RT>", "\uE80E"},
	{"<INPUT_FRONTEND_RS>", "\uE81A"},
	{"<INPUT_FRONTEND_SKIP_CUTSCENE>", "\uE82E"},
	{"<INPUT_OPEN_VOTE>", "\uE827"},
	{"<INPUT_STANCE>", "\uE82A"},
	{"<INPUT_SHOOT>", "\uE838"},
	{"<INPUT_USE_ITEM>", "\uE83B"},
	{"<INPUT_TRAVERSE>", "\uE83F"},
	{"<INPUT_MAP_OBJECTIVES>", "\uE840"},
	{"<INPUT_CAMERA_NEXT>", "\uE843"},
	{"<INPUT_CAMERA_PREVIOUS>", "\uE844"},
	{"<INPUT_SPAWN>", "\uE845"},
	{"<INPUT_EMPTY_LUNG>", "\uE846"},
	{"<INPUT_INTERACT>", "\uE847"},
	{"<INPUT_USE_COVER>", "\uE848"},
	{"<INPUT_PICK_UP_BODY>", "\uE84B"},
	{"<INPUT_PICK_UP_WEAPON>", "\uE84C"},
	{"<INPUT_ZOOM_IN>", "\uE84D"},
	{"<INPUT_ZOOM_OUT>", "\uE84E"},
	{"<INPUT_SCOPE_ZOOM_IN>", "\uE852"},
	{"<INPUT_SCOPE_ZOOM_OUT>", "\uE853"},
	{"<INPUT_TAG_ENVIRONMENT>", "\uE854"},
	{"<INPUT_REVIVE_BUDDY>", "\uE858"},
	{"<INPUT_BINOCULARS>", "\uE859"},
	{"<INPUT_VOICE_CHAT>", "\uE863"},
	{"<INPUT_RADIAL_MENU>", "\uE869"},
	{"<INPUT_CLOSE_MAP>", "\uE86B"},
	{"<INPUT_SEARCH_CORPSE>", "\uE872"},
	{"<INPUT_SWAP_SECONDARY>", "\uE873"},
	{"<INPUT_TAKEDOWN>", "\uE879"},
	{"<INPUT_VIEW_COLLECTIBLE>", "\uE87C"},
	{"<INPUT_DROP_DOWN>", "\uE87D"},
	{"<INPUT_DEFUSE>", "\uE87E"},
	{"<INPUT_INCREASE_SCOPE_RANGE>", "\uE882"},
	{"<INPUT_DECREASE_SCOPE_RANGE>", "\uE883"},
	{"<INPUT_CAMERA_SWAP>", "\uE892"},
	{"<INPUT_TOGGLE_VOICE_CHAT>", "\uE89E"},
	{"<INPUT_FRONTEND_MAP_MY_POSITION>", "\uE963"},
	{"<INPUT_FRONTEND_MAP_TRACK>", "\uE964"},
	{"<INPUT_FRONTEND_MAP_TOGGLE_ZOOM>", "\uE965"},
	{"<INPUT_FRONTEND_ZOOM_IN>", "\uE966"},
	{"<INPUT_FRONTEND_ZOOM_OUT>", "\uE967"},
	{"<INPUT_FRONTEND_SELECT_UNK>", "\uE968"},
	{"<INPUT_FRONTEND_MAP_SHOW_OBJECTIVES>", "\uE96E"},
	{"<INPUT_FRONTEND_LEFTRIGHT>", "\uE9FA"},
	{"<INPUT_FRONTEND_UPDOWN>", "\uE9FB"},
}

var glyphsToTagsReplacer = buildReplacer(false)
var tagsToGlyphsReplacer = buildReplacer(true)

func buildReplacer(tagToSeq bool) *strings.Replacer {
	pairs := make([]string, 0, len(glyphTable)*2)
	for _, m := range glyphTable {
		if tagToSeq {
			pairs = append(pairs, m.tag, m.seq)
		} else {
			pairs = append(pairs, m.seq, m.tag)
		}
	}
	return strings.NewReplacer(pairs...)
}

// DecodeText converts raw UTF-16LE chunk bytes into a human-editable string, replacing
// gamepad-glyph and control-character code units with <TAG> placeholders.
func DecodeText(data []byte) string {
	return glyphsToTagsReplacer.Replace(string(utf16.Decode(decodeUTF16LE(data))))
}

// EncodeText reverses DecodeText: replaces <TAG> placeholders with their original code
// units and re-encodes the result as UTF-16LE.
func EncodeText(s string) []byte {
	return encodeUTF16LE(utf16.Encode([]rune(tagsToGlyphsReplacer.Replace(s))))
}

func decodeUTF16LE(data []byte) []uint16 {
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return units
}

func encodeUTF16LE(units []uint16) []byte {
	data := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[i*2:], u)
	}
	return data
}
