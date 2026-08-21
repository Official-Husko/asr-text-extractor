package asura

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type yamlRecord struct {
	Command  string `yaml:"command"`
	Source   string `yaml:"source"`
	Override string `yaml:"override"`
}

// encodeYAML renders records as an ordered YAML sequence of the same shape as JSON.
func encodeYAML(records []Record) (string, error) {
	out := make([]yamlRecord, len(records))
	for i, r := range records {
		out[i] = yamlRecord{Command: r.Command, Source: r.SourceText, Override: r.OverrideText}
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeYAML(text string) ([]Record, error) {
	var in []yamlRecord
	if err := yaml.Unmarshal([]byte(text), &in); err != nil {
		return nil, fmt.Errorf("asura: invalid YAML records: %w", err)
	}
	records := make([]Record, len(in))
	for i, r := range in {
		records[i] = Record{Command: r.Command, SourceText: r.Source, OverrideText: r.Override}
	}
	return records, nil
}
