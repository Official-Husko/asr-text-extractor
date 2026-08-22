package cli

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
	"github.com/Official-Husko/asr-text-extractor/pkg/dds"
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
	var convert string
	cmd := &cobra.Command{
		Use:   "unpack <file> [output-dir]",
		Short: "Extract every embedded texture from an RSCF archive",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if convert != "dds" && convert != "png" {
				return fmt.Errorf("--convert must be \"dds\" or \"png\", got %q", convert)
			}
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

			skipped := 0
			for _, e := range f.Entries {
				payload := e.Data
				ext := ".dds"
				if convert == "png" {
					ext = ".png"
					img, err := dds.Decode(e.Data)
					if err != nil {
						fmt.Fprintf(os.Stderr, "skipping %s: %v\n", e.Path, err)
						skipped++
						continue
					}
					var buf bytes.Buffer
					// BestSpeed: default compression takes ~4x longer for only ~15% smaller
					// files on these textures, and this tool may be encoding hundreds of
					// them (some multi-megapixel) in one run.
					enc := png.Encoder{CompressionLevel: png.BestSpeed}
					if err := enc.Encode(&buf, img); err != nil {
						return fmt.Errorf("%s: encoding PNG: %w", e.Path, err)
					}
					payload = buf.Bytes()
				}

				dest := filepath.Join(outDir, relPathWithExt(e.Path, ext))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dest, payload, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}
			if skipped > 0 {
				fmt.Fprintf(os.Stderr, "%d of %d entries skipped (unsupported pixel format)\n", skipped, len(f.Entries))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&convert, "convert", "dds",
		"output image format: dds (raw, default, always succeeds) or png (decoded, lossless; entries in an unsupported pixel format are skipped with a warning)")
	return cmd
}

// relPathWithExt converts a texture manifest path into a safe, OS-appropriate relative path
// with ext substituted for the manifest path's own extension — the manifest path's extension
// reflects the original pre-conversion art source (.tga, .png, ...), not what's extracted.
func relPathWithExt(assetPath, ext string) string {
	rel := assetRelPath(assetPath)
	return strings.TrimSuffix(rel, filepath.Ext(rel)) + ext
}
