// Package cli implements the asr-text-extractor command-line interface on top of the
// reusable format library in pkg/asura.
package cli

import (
	"os"

	"github.com/spf13/cobra"
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
		Short:   "Unpack, translate, and repack Asura Engine string tables",
		Version: version,
		Long: `asr-text-extractor reads and writes the "Asura" container format used by
Rebellion's Asura Engine titles (Sniper Elite 4, Zombie Army 4, and others).

It handles two chunk types:
  text  - HTXT chunks: menu/UI strings, one table per file
  voice - DLLN chunks: voice line strings, many entries per file`,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newTextCmd())
	root.AddCommand(newVoiceCmd())
	return root
}
