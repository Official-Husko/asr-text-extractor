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
		Use:   "unpack <file> [output]",
		Short: "Unpack a voice file's DLLN entries to an interchange file",
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
			records, err := asura.UnpackVoice(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			if err := asura.WriteRecords(out, records, format, encoding); err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d entries)\n", out, len(records))
			return nil
		},
	}
}

func newVoiceOverrideCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "override <file> <data> [outfile]",
		Short: "Write translated strings from an interchange file back into a voice file (Version 4 DLLN entries only)",
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
			overrides := make(map[string]asura.Record, len(recs))
			for _, rec := range recs {
				overrides[rec.Command] = rec
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// The original tool always force-applies voice overrides: it never guards on
			// the entry's current text matching the recorded source text the way text
			// overrides do.
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
