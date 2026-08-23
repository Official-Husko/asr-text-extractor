package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
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
//
// When m.BoneNames is populated (a matching skeleton was found — see skeleton.go), triangles
// are split by their first vertex's primary bone (BoneIDs[0]) — e.g. a rifle's Body/Bolt/
// Trigger — independently of Mesh.Groups (that's the mesh's own *material* grouping, unrelated:
// a real sample has one material group but five distinct bones). Each part switch emits both an
// "o" (object) and a "g" (group) line: "o" is what makes Blender's OBJ importer split the
// result into separate, independently selectable objects — "g" alone often doesn't, landing
// everything in one combined object with named vertex groups instead, depending on import
// settings — so both are written for reliability across Blender versions/settings. A part
// switch mid-triangle-list just emits new "o"/"g" lines; OBJ doesn't require these to be
// declared up front or triangles to already be sorted by part.
func writeOBJ(w io.Writer, m asura.Mesh) error {
	buf := bufio.NewWriter(w)
	for _, v := range m.Vertices {
		p := objAxes(v.Position)
		if _, err := fmt.Fprintf(buf, "v %g %g %g\n", p[0], p[1], p[2]); err != nil {
			return err
		}
	}
	for _, v := range m.Vertices {
		if _, err := fmt.Fprintf(buf, "vt %g %g\n", v.UV0[0], v.UV0[1]); err != nil {
			return err
		}
	}

	// Group triangles by part (primary bone) and sort so each part's "o"/"g" block is fully
	// contiguous — writing them in original (interleaved) triangle order technically still
	// respects OBJ's rule that a name switch just starts a new block, but repeating the same
	// object name multiple times non-contiguously is exactly the kind of thing real-world
	// importers handle inconsistently (some silently merge same-named blocks regardless of
	// order, some don't, some auto-suffix repeats as separate ".001" objects).
	names, indices := groupTrianglesByBone(m)
	for _, name := range names {
		if name != "" {
			if _, err := fmt.Fprintf(buf, "o %s\ng %s\n", name, name); err != nil {
				return err
			}
		}
		for _, ti := range indices[name] {
			t := m.Triangles[ti]
			// Flipped relative to the raw triangle order — see objAxes's doc comment for why.
			a, b, c := t[0]+1, t[2]+1, t[1]+1 // OBJ indices are 1-based
			if _, err := fmt.Fprintf(buf, "f %d/%d %d/%d %d/%d\n", a, a, b, b, c, c); err != nil {
				return err
			}
		}
	}
	return buf.Flush()
}

// groupTrianglesByBone buckets a mesh's triangle indices by the part (bone) name each belongs
// to, keyed by its first vertex's primary bone (BoneIDs[0]) — see writeOBJ. names is ordered by
// ascending bone ID for a stable, sensible file order (e.g. a rifle's Body before its Bolt).
// If m.BoneNames is empty (no matching skeleton found), returns a single "" group holding every
// triangle in its original order, so writeOBJ skips the "o"/"g" lines entirely.
func groupTrianglesByBone(m asura.Mesh) (names []string, indices map[string][]int) {
	if len(m.BoneNames) == 0 {
		all := make([]int, len(m.Triangles))
		for i := range all {
			all[i] = i
		}
		return []string{""}, map[string][]int{"": all}
	}

	indices = make(map[string][]int)
	order := make([]int, 0, len(m.BoneNames))
	seen := make(map[string]bool)
	for i, t := range m.Triangles {
		boneID := int(m.Vertices[t[0]].BoneIDs[0])
		name := fmt.Sprintf("bone_%d", boneID)
		if boneID >= 0 && boneID < len(m.BoneNames) && m.BoneNames[boneID] != "" {
			name = m.BoneNames[boneID]
		}
		indices[name] = append(indices[name], i)
		if !seen[name] {
			seen[name] = true
			order = append(order, boneID)
		}
	}
	sort.Ints(order)
	names = make([]string, len(order))
	for i, boneID := range order {
		name := fmt.Sprintf("bone_%d", boneID)
		if boneID >= 0 && boneID < len(m.BoneNames) && m.BoneNames[boneID] != "" {
			name = m.BoneNames[boneID]
		}
		names[i] = name
	}
	return names, indices
}

// objAxes converts a decoded Mesh position into OBJ's own coordinate convention. As of this
// version, that's the identity — no position transform — which is the result of composing the
// two data points gathered so far, not a fresh guess:
//
//  1. Writing a 90-degree-about-X rotation of the raw position (Y,Z -> -Z,Y) into the file
//     came out with the model right-side-up but rotated 90 degrees the wrong way (barrel
//     pointing straight up).
//  2. The user then manually fixed *that* result with Blender's own Rotate tool: X axis, -90
//     degrees — confirmed as exactly correct.
//
// Composing those (undo the file's own +90-about-X with the confirmed -90-about-X fix) cancels
// out to the identity — i.e., raw, untransformed positions — and this holds regardless of
// exactly what axis convention Blender's own OBJ importer applies internally, as long as it's
// some rotation about X (true of every standard Y-up/Z-up import convention, since X is the
// axis that doesn't move in that particular swap): rotations about the same axis always
// commute, so undoing this project's own +90 with the user's confirmed -90 always leaves raw
// positions, whatever Blender's own import rotation actually is.
//
// This is in tension with the *original* bug report (raw positions, before any of this,
// called "upside-down") — worth flagging rather than glossing over. The likely reconciliation:
// that original symptom may have been a face-winding/normals problem (backfaces rendering as
// black or missing, which reads as "wrong" at a glance) rather than a position problem — see
// the triangle winding flip in writeOBJ, applied for the first time in this version, which an
// all-position-focused earlier attempt didn't have. If the model is still wrong after this
// change, that reconciliation is probably what's incorrect, not the position math above.
func objAxes(p [3]float32) [3]float32 {
	return p
}
