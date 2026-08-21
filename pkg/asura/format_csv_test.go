package asura

import "testing"

func TestCSVRoundTrip(t *testing.T) {
	records := []Record{
		{Command: "1493970712", SourceText: "1 of 2<END>", OverrideText: "1 of 2<END>"},
		{Command: "1522599863", SourceText: "has, a comma", OverrideText: "has \"quotes\" too"},
	}

	text, err := encodeCSV(records)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeCSV(text)
	if err != nil {
		t.Fatalf("decodeCSV: %v\ninput:\n%s", err, text)
	}
	if len(got) != len(records) {
		t.Fatalf("got %d records, want %d", len(got), len(records))
	}
	for i := range records {
		if got[i] != records[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], records[i])
		}
	}
}
