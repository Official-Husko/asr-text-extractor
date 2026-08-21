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

// decodeHTXTMap decodes every entry's text and returns it keyed by hash, plus the hashes in
// first-seen order. If a hash repeats, the later entry's text wins but keeps the earlier
// entry's position — matching how the original tool's Dictionary<uint,string> behaves when
// re-inserting an existing key. addCode prefixes each value with "<hash>\t", matching
// TextByte.getCsvText(addCode) in the original.
func decodeHTXTMap(f *asura.HTXTFile, addCode bool) (order []uint32, values map[uint32]string) {
	order = make([]uint32, 0, len(f.Entries))
	seen := make(map[uint32]bool, len(f.Entries))
	values = make(map[uint32]string, len(f.Entries))
	for _, e := range f.Entries {
		text := asura.DecodeText(e.Data)
		if addCode {
			text = fmt.Sprintf("%d\t%s", e.Hash, text)
		}
		if !seen[e.Hash] {
			seen[e.Hash] = true
			order = append(order, e.Hash)
		}
		values[e.Hash] = text
	}
	return order, values
}

func newTextUnpackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack <file> [csv]",
		Short: "Unpack an HTXT file's strings to a tab-separated CSV",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			out := defaultCSVPath(path)
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

			order, values := decodeHTXTMap(f, true)
			lines := make([]string, len(order))
			for i, h := range order {
				lines[i] = values[h]
			}
			if err := asura.WriteUTF16LELines(out, lines); err != nil {
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
		Use:   "override <file> <csv> [outfile]",
		Short: "Write translated strings from a CSV back into an HTXT file",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, csvPath := args[0], args[1]
			out := path
			explicit := len(args) == 3
			if explicit {
				out = args[2]
			}
			if err := backupIfInPlace(path, explicit); err != nil {
				return err
			}

			lines, err := asura.ReadUTF16LELines(csvPath)
			if err != nil {
				return err
			}
			overrides := make(map[uint32]asura.CSVRecord, len(lines))
			for _, line := range lines {
				rec, err := asura.ParseCSVRecord(line)
				if err != nil {
					return err
				}
				code, err := rec.Code()
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
	cmd.Flags().BoolVar(&force, "force", false, "override entries even when their current text doesn't match the CSV's recorded source text")
	return cmd
}

func newTextCompareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <fileA> <fileB> [csv]",
		Short: "Build a source/override comparison table across two HTXT language files",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			pathA, pathB := args[0], args[1]
			out := defaultCSVPath(pathA)
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

			orderA, valuesA := decodeHTXTMap(fA, true)
			_, valuesB := decodeHTXTMap(fB, false)
			lines := make([]string, len(orderA))
			for i, h := range orderA {
				name := valuesA[h]
				if other, ok := valuesB[h]; ok {
					lines[i] = name + "\t" + other
				} else {
					lines[i] = name + "\t" + name
				}
			}
			if err := asura.WriteUTF16LELines(out, lines); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
}
