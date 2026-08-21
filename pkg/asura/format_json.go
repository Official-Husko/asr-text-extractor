package asura

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type jsonRecord struct {
	Command  string `json:"command"`
	Name     string `json:"name,omitempty"`
	Source   string `json:"source"`
	Override string `json:"override"`
}

// encodeJSON renders records as an ordered JSON array of {command, name, source, override}
// objects, matching entry order exactly (important for HTXT tables). name is omitted when
// empty (e.g. always, for voice entries — no symbol-name table has been found for those).
//
// Uses an Encoder with HTML-escaping disabled: json.Marshal's default behavior turns every
// '<' and '>' into "<"/">" (meant for JSON embedded in HTML), which would mangle
// every <TAG> placeholder in the unpacked text and make the file painful to hand-edit.
func encodeJSON(records []Record) (string, error) {
	out := make([]jsonRecord, len(records))
	for i, r := range records {
		out[i] = jsonRecord{Command: r.Command, Name: r.Name, Source: r.SourceText, Override: r.OverrideText}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func decodeJSON(text string) ([]Record, error) {
	var in []jsonRecord
	if err := json.Unmarshal([]byte(text), &in); err != nil {
		return nil, fmt.Errorf("asura: invalid JSON records: %w", err)
	}
	records := make([]Record, len(in))
	for i, r := range in {
		records[i] = Record{Command: r.Command, Name: r.Name, SourceText: r.Source, OverrideText: r.Override}
	}
	return records, nil
}
