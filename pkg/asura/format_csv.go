package asura

import (
	"encoding/csv"
	"fmt"
	"strings"
)

var csvHeader = []string{"command", "name", "source", "override"}

// encodeCSV renders records as a proper RFC 4180 CSV (comma-separated, quoted as needed)
// with a header row. name is written as an empty field when unset (e.g. always, for voice
// entries — no symbol-name table has been found for those).
func encodeCSV(records []Record) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(csvHeader); err != nil {
		return "", err
	}
	for _, r := range records {
		if err := w.Write([]string{r.Command, r.Name, r.SourceText, r.OverrideText}); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// decodeCSV parses a CSV written by encodeCSV (or a compatible hand-edited file with a
// command,name,source,override header).
func decodeCSV(text string) ([]Record, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = 4
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("asura: invalid CSV: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	rows = rows[1:] // header

	records := make([]Record, len(rows))
	for i, row := range rows {
		records[i] = Record{Command: row[0], Name: row[1], SourceText: row[2], OverrideText: row[3]}
	}
	return records, nil
}
