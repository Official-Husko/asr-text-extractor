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

func newPackageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Work with AsuraZbb-compressed level-package files (.pc, .pc_entdata)",
	}
	cmd.AddCommand(newPackageUnpackCmd())
	return cmd
}

func newPackageUnpackCmd() *cobra.Command {
	var convert string
	cmd := &cobra.Command{
		Use:   "unpack <file> [output-dir]",
		Short: "Extract every manifest-referenced sub-file and embedded texture from a level package",
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

			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			pkg, err := asura.ParsePackage(raw)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			fmt.Fprintf(os.Stderr, "Entries: %d  Textures: %d\n", len(pkg.Entries), len(pkg.Textures))

			for _, e := range pkg.Entries {
				dest := filepath.Join(outDir, "files", assetRelPath(e.Path))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dest, e.Data, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}

			skipped := 0
			for _, t := range pkg.Textures {
				payload := t.Data
				ext := ".dds"
				if convert == "png" {
					ext = ".png"
					img, err := dds.Decode(t.Data)
					if err != nil {
						fmt.Fprintf(os.Stderr, "skipping %s: %v\n", t.Path, err)
						skipped++
						continue
					}
					var buf bytes.Buffer
					// BestSpeed: default compression takes ~4x longer for only ~15% smaller
					// files on these textures, and a single package can embed thousands of
					// them (some multi-megapixel).
					enc := png.Encoder{CompressionLevel: png.BestSpeed}
					if err := enc.Encode(&buf, img); err != nil {
						return fmt.Errorf("%s: encoding PNG: %w", t.Path, err)
					}
					payload = buf.Bytes()
				}

				dest := filepath.Join(outDir, "textures", relPathWithExt(t.Path, ext))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(dest, payload, 0o644); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}
			if skipped > 0 {
				fmt.Fprintf(os.Stderr, "%d of %d textures skipped (unsupported pixel format)\n", skipped, len(pkg.Textures))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&convert, "convert", "dds",
		"output image format for embedded textures: dds (raw, default, always succeeds) or png (decoded, lossless; entries in an unsupported pixel format are skipped with a warning)")
	return cmd
}
