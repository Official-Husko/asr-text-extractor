package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func newVoiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "voice",
		Short: "Work with DLLN voice line chunks",
	}
	cmd.AddCommand(newVoiceUnpackCmd(), newVoiceOverrideCmd())
	return cmd
}

func newVoiceUnpackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack <file> [csv]",
		Short: "Unpack a voice file's DLLN entries to a tab-separated CSV",
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
			entries, err := asura.UnpackVoice(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			lines := make([]string, len(entries))
			for i, e := range entries {
				rec := asura.CSVRecord{Command: e.Command, SourceText: e.SourceText, OverrideText: e.OverrideText}
				lines[i] = rec.Line()
			}
			if err := asura.WriteUTF16LELines(out, lines); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d entries)\n", out, len(entries))
			return nil
		},
	}
}

func newVoiceOverrideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "override <file> <csv> [outfile]",
		Short: "Write translated strings from a CSV back into a voice file (Version 4 DLLN entries only)",
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
			overrides := make(map[string]asura.CSVRecord, len(lines))
			for _, line := range lines {
				rec, err := asura.ParseCSVRecord(line)
				if err != nil {
					return err
				}
				overrides[rec.Command] = rec
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// The original tool always force-applies voice overrides: it never guards on
			// the entry's current text matching the CSV's recorded source text the way
			// text overrides do.
			result, err := asura.OverrideVoice(data, overrides, true)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			if err := os.WriteFile(out, result, 0o644); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "wrote", out)
			return nil
		},
	}
}
