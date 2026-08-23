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
	"strconv"
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
	var meshFormat string
	var separateLODs bool
	cmd := &cobra.Command{
		Use:   "unpack <file> [output-dir]",
		Short: "Extract every manifest-referenced sub-file and embedded texture from a level package",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if convert != "dds" && convert != "png" {
				return fmt.Errorf("--convert must be \"dds\" or \"png\", got %q", convert)
			}
			if meshFormat != "gltf" && meshFormat != "obj" && meshFormat != "both" {
				return fmt.Errorf("--mesh-format must be \"gltf\", \"obj\", or \"both\", got %q", meshFormat)
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

			if separateLODs {
				for _, m := range pkg.Meshes {
					if err := writeMeshOutputs(outDir, meshFormat, m, pkg.Textures); err != nil {
						return err
					}
				}
			} else {
				for _, g := range groupMeshesByBase(pkg.Meshes) {
					if len(g.meshes) == 1 {
						if err := writeMeshOutputs(outDir, meshFormat, g.meshes[0], pkg.Textures); err != nil {
							return err
						}
						continue
					}
					if err := writeMeshGroupOutputs(outDir, meshFormat, g.base, g.meshes, pkg.Textures); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&convert, "convert", "dds",
		"output image format for embedded textures: dds (raw, default, always succeeds) or png (decoded, lossless; entries in an unsupported pixel format are skipped with a warning)")
	cmd.Flags().StringVar(&meshFormat, "mesh-format", "gltf",
		"output format for meshes: gltf (default; a real Blender-importable skinned armature for meshes with a matching skeleton, letting rigid sub-parts like a rifle's bolt be independently selected/posed by bone), obj (a plain, unrigged mesh — multi-part meshes are instead split into per-part o/g groups, for tools/workflows that don't handle glTF skinning), or both")
	cmd.Flags().BoolVar(&separateLODs, "separate-lods", false,
		"write each LOD variant (l1#, l2#, ...) of a mesh, and any \"<name>_destroyed\" counterpart, to its own file instead of combining every variant of the same base mesh into one file (the default)")
	return cmd
}

// writeMeshFile creates <outDir>/meshes/<m.Path-with-ext> (and any needed parent directories)
// and calls write to fill it, closing the file and translating any write/close error into one
// that names the mesh. Shared by the .glb and .obj output paths in newPackageUnpackCmd.
func writeMeshFile(outDir, ext string, m asura.Mesh, write func(io.Writer, asura.Mesh) error) (string, error) {
	dest := filepath.Join(outDir, "meshes", relPathWithExt(m.Path, ext))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	err = write(f, m)
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dest, nil
}

// writeMeshGroupFile is writeMeshFile's counterpart for a combined multi-variant file: creates
// <outDir>/meshes/<base-with-ext> and calls write with the whole variant group.
func writeMeshGroupFile(outDir, ext, base string, meshes []asura.Mesh, write func(io.Writer, string, []asura.Mesh) error) (string, error) {
	dest := filepath.Join(outDir, "meshes", relPathWithExt(base, ext))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	err = write(f, base, meshes)
	closeErr := f.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return dest, nil
}

// writeMeshOutputs writes a single mesh through writeGLB and/or writeOBJ according to
// meshFormat ("gltf", "obj", or "both"), printing a "wrote <path>" diagnostic per file. Used
// directly by --separate-lods, and as the fallback in the default (combining) path for a base
// mesh with no LOD/state siblings to combine with — see groupMeshesByBase. textures is the
// whole package's texture list, used by the glTF path to embed a matching material (see
// gltf.go's addMaterial); the OBJ path ignores it, since OBJ export has no texture embedding.
func writeMeshOutputs(outDir, meshFormat string, m asura.Mesh, textures []asura.TextureEntry) error {
	if meshFormat == "gltf" || meshFormat == "both" {
		dest, err := writeMeshFile(outDir, ".glb", m, func(w io.Writer, m asura.Mesh) error {
			return writeGLB(w, m, textures)
		})
		if err != nil {
			return fmt.Errorf("%s: writing glTF: %w", m.Path, err)
		}
		fmt.Fprintln(os.Stderr, "wrote", dest)
	}
	if meshFormat == "obj" || meshFormat == "both" {
		dest, err := writeMeshFile(outDir, ".obj", m, writeOBJ)
		if err != nil {
			return fmt.Errorf("%s: writing OBJ: %w", m.Path, err)
		}
		fmt.Fprintln(os.Stderr, "wrote", dest)
	}
	return nil
}

// writeMeshGroupOutputs is writeMeshOutputs's counterpart for a combined multi-variant group
// (see groupMeshesByBase) — the default package-unpack behavior for a base mesh that has LOD
// and/or "_destroyed" siblings.
func writeMeshGroupOutputs(outDir, meshFormat, base string, meshes []asura.Mesh, textures []asura.TextureEntry) error {
	if meshFormat == "gltf" || meshFormat == "both" {
		dest, err := writeMeshGroupFile(outDir, ".glb", base, meshes, func(w io.Writer, base string, meshes []asura.Mesh) error {
			return writeGLBGroup(w, base, meshes, textures)
		})
		if err != nil {
			return fmt.Errorf("%s: writing glTF: %w", base, err)
		}
		fmt.Fprintln(os.Stderr, "wrote", dest)
	}
	if meshFormat == "obj" || meshFormat == "both" {
		dest, err := writeMeshGroupFile(outDir, ".obj", base, meshes, writeOBJGroup)
		if err != nil {
			return fmt.Errorf("%s: writing OBJ: %w", base, err)
		}
		fmt.Fprintln(os.Stderr, "wrote", dest)
	}
	return nil
}

// meshLODGroup is one group of mesh variants sharing a base name — see groupMeshesByBase.
type meshLODGroup struct {
	base   string
	meshes []asura.Mesh
}

// splitLOD splits a mesh path into its LOD prefix (e.g. "l1", or "" for the base/LOD0 variant)
// and base name (e.g. "carcano") — the same "l1#carcano" convention pkg/asura/package.go's own
// meshBaseName strips when matching a mesh to its skeleton.
func splitLOD(path string) (lod, base string) {
	if before, after, ok := strings.Cut(path, "#"); ok {
		return before, after
	}
	return "", path
}

// destroyedSuffix marks a mesh as a broken/destructible-state counterpart of a plain base mesh
// with the same name minus this suffix (e.g. "bulb_b_destroyed" alongside "bulb_b") — a
// user-observed real-sample naming convention, folded into the same group as its healthy
// counterpart by groupMeshesByBase, the same way an "l1#"/"l2#"/... LOD prefix is.
const destroyedSuffix = "_destroyed"

// groupMeshesByBase groups meshes that belong in the same combined output file: every LOD
// variant sharing one base name (see splitLOD — e.g. "chandelier_long_base" and its
// "l1#chandelier_long_base".."l6#chandelier_long_base" siblings), plus, when both exist, a
// "<base>_destroyed" group folded into its healthy "<base>" counterpart (including that
// destroyed mesh's own LOD variants, if any). A "<name>_destroyed" mesh with no healthy
// "<name>" counterpart in this package is left as its own standalone group, untouched — folding
// it under a name that doesn't otherwise exist here would misname its output file for no
// benefit. Order is preserved throughout (both across groups and within one) rather than
// re-sorted, since real samples already list a base mesh immediately followed by its LOD
// variants in ascending order.
func groupMeshesByBase(meshes []asura.Mesh) []meshLODGroup {
	index := make(map[string]int)
	var groups []meshLODGroup
	for _, m := range meshes {
		_, base := splitLOD(m.Path)
		if i, ok := index[base]; ok {
			groups[i].meshes = append(groups[i].meshes, m)
			continue
		}
		index[base] = len(groups)
		groups = append(groups, meshLODGroup{base: base, meshes: []asura.Mesh{m}})
	}

	merged := make([]bool, len(groups))
	for i, g := range groups {
		if !strings.HasSuffix(g.base, destroyedSuffix) {
			continue
		}
		healthyIdx, ok := index[strings.TrimSuffix(g.base, destroyedSuffix)]
		if !ok {
			continue
		}
		groups[healthyIdx].meshes = append(groups[healthyIdx].meshes, g.meshes...)
		merged[i] = true
	}

	kept := groups[:0]
	for i, g := range groups {
		if !merged[i] {
			kept = append(kept, g)
		}
	}
	return kept
}

// lodLabel computes a clean "LOD<n>" (or "LOD<n>_Destroyed") display label for a mesh path,
// replacing its raw "l1#name"/"name_destroyed" form when writing a combined multi-variant file
// (see writeGLBGroup/writeOBJGroup) — the un-prefixed base/LOD0 variant becomes "LOD0" rather
// than reusing its own bare name, which used to collide with a combined file's own former
// container-node name and get auto-suffixed ".001" by Blender on import. An "lN" prefix that
// isn't of that exact numeric shape (not expected in any real sample seen) falls back to "LOD0"
// rather than guessing further.
func lodLabel(path string) string {
	lod, base := splitLOD(path)
	n := 0
	if lod != "" {
		if parsed, err := strconv.Atoi(strings.TrimPrefix(lod, "l")); err == nil {
			n = parsed
		}
	}
	label := fmt.Sprintf("LOD%d", n)
	if strings.HasSuffix(base, destroyedSuffix) {
		label += "_Destroyed"
	}
	return label
}

// meshTextures finds textures belonging to m's mesh path by folder-name convention: a real
// sample shows a mesh's own textures living in a same-named folder (e.g. the "carcano" mesh's
// diffuse/normal maps live under "...\rifles\carcano\..."), each ending in a role suffix (see
// textureRole). Returns the first matching albedo/normal texture found (nil if neither role
// matches) — a heuristic, not a confirmed link: this project has no reverse-engineered way to
// resolve a Mesh's own MeshGroup.Hash (its actual material identifier) back to a specific
// texture, so folder-name matching is what's available instead of a byte-exact one, and is used
// by gltf.go's addMaterial to embed a texture into exported meshes.
func meshTextures(meshPath string, textures []asura.TextureEntry) (albedo, normal *asura.TextureEntry) {
	_, base := splitLOD(meshPath)
	base = strings.ToLower(base)
	for i := range textures {
		t := &textures[i]
		if !hasPathSegment(t.Path, base) {
			continue
		}
		switch textureRole(t.Path) {
		case "albedo":
			if albedo == nil {
				albedo = t
			}
		case "normal":
			if normal == nil {
				normal = t
			}
		}
	}
	return albedo, normal
}

// hasPathSegment reports whether path (backslash- or slash-separated, matching how RSCF/RSFL
// paths are stored regardless of this tool's own host OS) has seg as one of its components,
// case-insensitively.
func hasPathSegment(path, seg string) bool {
	for _, s := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '\\' || r == '/' }) {
		if s == seg {
			return true
		}
	}
	return false
}

// textureRole classifies a texture path by its filename suffix (the part after the last "_",
// before the extension), using the naming convention found across a real sample's ~3,700
// texture paths (708 "_n"-suffixed names, 485 "_a"-suffixed, among others): "albedo" for a
// diffuse/color map, "normal" for a tangent-space normal map, "" for anything else — including
// combined/packed maps like the real sample's "_albedoroughness"/"_ar"/"_m" (metallic) suffixes,
// deliberately left unmatched rather than guessing at their channel layout (e.g. whether a "_m"
// map is plain grayscale metalness or already packed to glTF's roughness-in-green/
// metalness-in-blue convention isn't known, and assigning it to the wrong channel would produce
// a worse result than no metallic/roughness texture at all).
func textureRole(path string) string {
	name := path
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ToLower(name)
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	i := strings.LastIndexByte(name, '_')
	if i < 0 {
		return ""
	}
	switch name[i+1:] {
	case "a", "d", "albedo", "diff", "diffuse":
		return "albedo"
	case "n", "normal", "normals", "norm", "nm":
		return "normal"
	default:
		return ""
	}
}

// writeOBJGroup combines every LOD/state variant of one base mesh (see groupMeshesByBase) into a
// single OBJ file — the default package-unpack behavior, matching writeGLBGroup — instead of one
// file per variant. OBJ has no scene-graph nesting and its "v"/"vt" indices are shared across the
// whole file, so each variant's vertex data is appended (with face indices offset by every prior
// variant's vertex count) and each variant's own parts (or the whole variant, if it has no
// matching skeleton) are labeled by their "LOD<n>" display label (see lodLabel — e.g. "LOD1" or
// "LOD1_Bolt") rather than just the bone name writeOBJ alone uses, so multiple variants'
// same-named bones/parts stay distinguishable and individually selectable once imported.
func writeOBJGroup(w io.Writer, _ string, meshes []asura.Mesh) error {
	buf := bufio.NewWriter(w)
	vertexOffset := 0
	for _, m := range meshes {
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

		meshLabel := lodLabel(m.Path)
		names, indices := groupTrianglesByBone(m)
		for _, name := range names {
			label := meshLabel
			if name != "" {
				label = meshLabel + "_" + name
			}
			if _, err := fmt.Fprintf(buf, "o %s\ng %s\n", label, label); err != nil {
				return err
			}
			for _, ti := range indices[name] {
				t := m.Triangles[ti]
				x, y, z := flipWinding(t)
				a, b, c := int(x)+1+vertexOffset, int(y)+1+vertexOffset, int(z)+1+vertexOffset
				if _, err := fmt.Fprintf(buf, "f %d/%d %d/%d %d/%d\n", a, a, b, b, c, c); err != nil {
					return err
				}
			}
		}
		vertexOffset += len(m.Vertices)
	}
	return buf.Flush()
}

// writeOBJ writes a mesh as a Wavefront OBJ — the plain, unrigged alternative to the default
// glTF/.glb output (see writeGLB): no armature, no bone weights, just static geometry, for
// tools/workflows that don't handle glTF skinning. A "v" line per vertex position, a "vt" line
// per vertex's first UV channel (Mesh.UV0 — the second channel, UV1, isn't exposed via OBJ), and
// a triangle "f" line per Mesh triangle referencing them by the shared, 1-indexed (OBJ
// convention) vertex/UV index. Mesh has no decoded normals, so this doesn't write "vn" lines;
// Blender's own "Shade Smooth" / "Recalculate Normals" produces a reasonable result from the
// geometry alone.
//
// When m.Skeleton is populated (a matching skeleton was found — see skeleton.go), triangles are
// split by their first vertex's primary bone (BoneIDs[0]) — e.g. a rifle's Body/Bolt/Trigger —
// independently of Mesh.Groups (that's the mesh's own *material* grouping, unrelated: a real
// sample has one material group but five distinct bones). Each part switch emits both an "o"
// (object) and a "g" (group) line: "o" is what makes Blender's OBJ importer split the result
// into separate, independently selectable objects — "g" alone often doesn't, landing everything
// in one combined object with named vertex groups instead, depending on import settings — so
// both are written for reliability across Blender versions/settings. A part switch
// mid-triangle-list just emits new "o"/"g" lines; OBJ doesn't require these to be declared up
// front or triangles to already be sorted by part.
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
// If m.Skeleton is nil (no matching skeleton found), returns a single "" group holding every
// triangle in its original order, so writeOBJ skips the "o"/"g" lines entirely.
func groupTrianglesByBone(m asura.Mesh) (names []string, indices map[string][]int) {
	if m.Skeleton == nil {
		all := make([]int, len(m.Triangles))
		for i := range all {
			all[i] = i
		}
		return []string{""}, map[string][]int{"": all}
	}

	bones := m.Skeleton.Bones
	boneName := func(boneID int) string {
		if boneID >= 0 && boneID < len(bones) && bones[boneID].Name != "" {
			return bones[boneID].Name
		}
		return fmt.Sprintf("bone_%d", boneID)
	}

	indices = make(map[string][]int)
	order := make([]int, 0, len(bones))
	seen := make(map[string]bool)
	for i, t := range m.Triangles {
		boneID := int(m.Vertices[t[0]].BoneIDs[0])
		name := boneName(boneID)
		indices[name] = append(indices[name], i)
		if !seen[name] {
			seen[name] = true
			order = append(order, boneID)
		}
	}
	sort.Ints(order)
	names = make([]string, len(order))
	for i, boneID := range order {
		names[i] = boneName(boneID)
	}
	return names, indices
}

// objAxes converts a decoded Mesh position into OBJ's (and, since Blender's glTF importer
// applies the identical Y-up-to-Z-up conversion its default OBJ import axis settings do,
// writeGLB's) own coordinate convention. As of this version, that's the identity — no position
// transform — which is the result of composing the two data points gathered so far, not a fresh
// guess:
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
