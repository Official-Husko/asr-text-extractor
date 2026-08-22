// Package cli implements the asr-text-extractor command-line interface on top of the
// reusable format library in pkg/asura.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

const version = "0.1.0"

// Execute runs the root command and exits the process with a non-zero status on failure.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "asr-text-extractor",
		Short:   "Unpack, translate, and repack Asura Engine game assets",
		Version: version,
		Long: `asr-text-extractor reads and writes the "Asura" container format used by
Rebellion's Asura Engine titles (Sniper Elite 4, Zombie Army 4, and others).

It handles these chunk types:
  text    - HTXT chunks: menu/UI strings, one table per file (translate + repack)
  voice   - DLLN chunks: voice line strings, many entries per file (translate + repack)
  sound   - ASTS chunks: embedded WAV assets in a streamsounds manifest (extract only)
  texture - RSCF chunks: embedded DDS textures in a texture archive (extract only)
  package - AsuraZbb-compressed level packages (.pc, .pc_entdata): manifest
            sub-files and embedded textures (extract only)`,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().String("format", string(asura.FormatJSON),
		"interchange format: txt, csv, json, yaml, or xml")
	root.PersistentFlags().String("encoding", "",
		"interchange text encoding: utf8 or utf16le (default: utf16le for txt, utf8 otherwise)")
	root.AddCommand(newTextCmd())
	root.AddCommand(newVoiceCmd())
	root.AddCommand(newSoundCmd())
	root.AddCommand(newTextureCmd())
	root.AddCommand(newPackageCmd())
	return root
}

// formatAndEncoding resolves the shared --format/--encoding flags for a subcommand.
func formatAndEncoding(cmd *cobra.Command) (asura.Format, asura.Encoding, error) {
	formatStr, err := cmd.Flags().GetString("format")
	if err != nil {
		return "", "", err
	}
	format, err := asura.ParseFormat(formatStr)
	if err != nil {
		return "", "", err
	}
	encodingStr, err := cmd.Flags().GetString("encoding")
	if err != nil {
		return "", "", err
	}
	encoding, err := asura.ParseEncoding(encodingStr)
	if err != nil {
		return "", "", err
	}
	return format, encoding, nil
}

// requireCode adapts Record.Code() for use as a map-building step, wrapping the error with
// the offending line's position for a more useful message.
func requireCode(rec asura.Record, index int) (uint32, error) {
	code, err := rec.Code()
	if err != nil {
		return 0, fmt.Errorf("record %d: %w", index, err)
	}
	return code, nil
}
