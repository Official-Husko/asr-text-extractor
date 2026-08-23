package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"image/png"
	"io"
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
			fmt.Fprintf(os.Stderr, "Entries: %d  Textures: %d  Meshes: %d\n", len(pkg.Entries), len(pkg.Textures), len(pkg.Meshes))

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

			for _, m := range pkg.Meshes {
				dest := filepath.Join(outDir, "meshes", relPathWithExt(m.Path, ".obj"))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					return err
				}
				f, err := os.Create(dest)
				if err != nil {
					return err
				}
				err = writeOBJ(f, m)
				closeErr := f.Close()
				if err != nil {
					return fmt.Errorf("%s: writing OBJ: %w", m.Path, err)
				}
				if closeErr != nil {
					return closeErr
				}
				fmt.Fprintln(os.Stderr, "wrote", dest)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&convert, "convert", "dds",
		"output image format for embedded textures: dds (raw, default, always succeeds) or png (decoded, lossless; entries in an unsupported pixel format are skipped with a warning)")
	return cmd
}

// writeOBJ writes a mesh as a Wavefront OBJ: a "v" line per vertex position, a "vt" line per
// vertex's first UV channel (Mesh.UV0 — the second channel, UV1, isn't exposed via OBJ), and a
// triangle "f" line per Mesh triangle referencing them by the shared, 1-indexed (OBJ
// convention) vertex/UV index. Mesh has no decoded normals, so this doesn't write "vn" lines;
// Blender's own "Shade Smooth" / "Recalculate Normals" produces a reasonable result from the
// geometry alone.
func writeOBJ(w io.Writer, m asura.Mesh) error {
	buf := bufio.NewWriter(w)
	for _, v := range m.Vertices {
		if _, err := fmt.Fprintf(buf, "v %g %g %g\n", v.Position[0], v.Position[1], v.Position[2]); err != nil {
			return err
		}
	}
	for _, v := range m.Vertices {
		if _, err := fmt.Fprintf(buf, "vt %g %g\n", v.UV0[0], v.UV0[1]); err != nil {
			return err
		}
	}
	for _, t := range m.Triangles {
		a, b, c := t[0]+1, t[1]+1, t[2]+1 // OBJ indices are 1-based
		if _, err := fmt.Fprintf(buf, "f %d/%d %d/%d %d/%d\n", a, a, b, b, c, c); err != nil {
			return err
		}
	}
	return buf.Flush()
}
