package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func newTextureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "texture",
		Short: "Work with RSCF texture archives",
	}
	cmd.AddCommand(newTextureUnpackCmd())
	return cmd
}

func newTextureUnpackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unpack <file> [output-dir]",
		Short: "Extract every embedded DDS texture from an RSCF archive",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			outDir := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if len(args) == 2 {
				outDir = args[1]
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			f, err := asura.ParseRSCF(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			fmt.Fprintf(os.Stderr, "Entries: %d\n", len(f.Entries))

			for _, e := range f.Entries {
				dest := filepath.Join(outDir, ddsRelPath(e.Path))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dest, e.Data, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}
			return nil
		},
	}
}

// ddsRelPath converts a texture manifest path into a safe, OS-appropriate relative path
// ending in ".dds" — the extracted data is always a DDS file regardless of the manifest
// path's own extension, which instead reflects the original pre-conversion art source
// (.tga, .png, ...).
func ddsRelPath(assetPath string) string {
	rel := assetRelPath(assetPath)
	return strings.TrimSuffix(rel, filepath.Ext(rel)) + ".dds"
}
