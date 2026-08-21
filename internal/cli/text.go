package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func newTextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "text",
		Short: "Work with HTXT menu/UI text chunks",
	}
	cmd.AddCommand(newTextUnpackCmd(), newTextOverrideCmd(), newTextCompareCmd())
	return cmd
}

// htxtRecords decodes every entry's text into a Record and returns the records plus the
// hashes in first-seen order. If a hash repeats, the later entry's text wins but keeps the
// earlier entry's position — matching how the original tool's Dictionary<uint,string>
// behaves when re-inserting an existing key.
func htxtRecords(f *asura.HTXTFile) (order []uint32, records map[uint32]asura.Record) {
	order = make([]uint32, 0, len(f.Entries))
	seen := make(map[uint32]bool, len(f.Entries))
	records = make(map[uint32]asura.Record, len(f.Entries))
	for _, e := range f.Entries {
		text := DecodeTextEntry(e)
		if !seen[e.Hash] {
			seen[e.Hash] = true
			order = append(order, e.Hash)
		}
		records[e.Hash] = text
	}
	return order, records
}

// DecodeTextEntry renders a single HTXT entry as a Record (command = decimal hash, source =
// override = its decoded text, until a translation overrides it).
func DecodeTextEntry(e asura.TextEntry) asura.Record {
	text := asura.DecodeText(e.Data)
	command := fmt.Sprint(e.Hash)
	return asura.Record{Command: command, SourceText: text, OverrideText: text}
}

func newTextUnpackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack <file> [output]",
		Short: "Unpack an HTXT file's strings to an interchange file",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, encoding, err := formatAndEncoding(cmd)
			if err != nil {
				return err
			}
			path := args[0]
			out := defaultOutputPath(path, format)
			if len(args) == 2 {
				out = args[1]
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			f, err := asura.ParseHTXT(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			fmt.Fprintf(os.Stderr, "Version: %d  Entries: %d  LanguageID: %d\n", f.Version, len(f.Entries), f.LanguageID)

			order, values := htxtRecords(f)
			records := make([]asura.Record, len(order))
			for i, h := range order {
				records[i] = values[h]
			}
			if err := asura.WriteRecords(out, records, format, encoding); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
}

func newTextOverrideCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "override <file> <data> [outfile]",
		Short: "Write translated strings from an interchange file back into an HTXT file",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, encoding, err := formatAndEncoding(cmd)
			if err != nil {
				return err
			}
			path, dataPath := args[0], args[1]
			out := path
			explicit := len(args) == 3
			if explicit {
				out = args[2]
			}
			if err := backupIfInPlace(path, explicit); err != nil {
				return err
			}

			recs, err := asura.ReadRecords(dataPath, format, encoding)
			if err != nil {
				return err
			}
			overrides := make(map[uint32]asura.Record, len(recs))
			for i, rec := range recs {
				code, err := requireCode(rec, i)
				if err != nil {
					return err
				}
				overrides[code] = rec
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			f, err := asura.ParseHTXT(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			f.Override(overrides, force)
			if err := os.WriteFile(out, f.Encode(), 0o644); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override entries even when their current text doesn't match the recorded source text")
	return cmd
}

func newTextCompareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <fileA> <fileB> [output]",
		Short: "Build a source/override comparison table across two HTXT language files",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, encoding, err := formatAndEncoding(cmd)
			if err != nil {
				return err
			}
			pathA, pathB := args[0], args[1]
			out := defaultOutputPath(pathA, format)
			if len(args) == 3 {
				out = args[2]
			}

			dataA, err := os.ReadFile(pathA)
			if err != nil {
				return err
			}
			dataB, err := os.ReadFile(pathB)
			if err != nil {
				return err
			}
			fA, err := asura.ParseHTXT(dataA)
			if err != nil {
				return fmt.Errorf("%s: %w", pathA, err)
			}
			fB, err := asura.ParseHTXT(dataB)
			if err != nil {
				return fmt.Errorf("%s: %w", pathB, err)
			}

			orderA, valuesA := htxtRecords(fA)
			_, valuesB := htxtRecords(fB)
			records := make([]asura.Record, len(orderA))
			for i, h := range orderA {
				recA := valuesA[h]
				other := recA.SourceText
				if recB, ok := valuesB[h]; ok {
					other = recB.SourceText
				}
				records[i] = asura.Record{Command: recA.Command, SourceText: recA.SourceText, OverrideText: other}
			}
			if err := asura.WriteRecords(out, records, format, encoding); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
}
