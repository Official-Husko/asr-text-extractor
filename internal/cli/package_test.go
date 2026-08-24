package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

func TestSplitLOD(t *testing.T) {
	cases := []struct {
		path     string
		wantLOD  string
		wantBase string
	}{
		{"carcano", "", "carcano"},
		{"l1#carcano", "l1", "carcano"},
		{"l6#chandelier_long_base", "l6", "chandelier_long_base"},
	}
	for _, c := range cases {
		lod, base := splitLOD(c.path)
		if lod != c.wantLOD || base != c.wantBase {
			t.Errorf("splitLOD(%q) = (%q, %q), want (%q, %q)", c.path, lod, base, c.wantLOD, c.wantBase)
		}
	}
}

func TestTextureRole(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`\graphics\weapons\rifles\carcano\carcano_body_a.tga`, "albedo"},
		{`\graphics\weapons\rifles\carcano\carcano_body_n.tga`, "normal"},
		{`\graphics\weapons\rifles\carcano\carcano_body_m.tga`, ""}, // metallic: deliberately unmatched, see doc comment
		{`\graphics\props\light_albedoroughness.tga`, "albedo"},    // packed albedo+roughness: safe to use as baseColorTexture, see doc comment
		{`\graphics\props\light_ar.tga`, "albedo"},                 // same convention, short form (Sniper Elite 5/Resistance's dominant suffix)
		{`\graphics\characters\skin_diffuse.tga`, "albedo"},
		{`\graphics\characters\skin_normals.tga`, "normal"},
		{`noextension`, ""},
		{`no_underscore.tga`, ""},
	}
	for _, c := range cases {
		if got := textureRole(c.path); got != c.want {
			t.Errorf("textureRole(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestMeshTexturesMatchesByFolderSegment(t *testing.T) {
	textures := []asura.TextureEntry{
		{Path: `\graphics\weapons\rifles\carcano\carcano_body_a.tga`},
		{Path: `\graphics\weapons\rifles\carcano\carcano_body_m.tga`},
		{Path: `\graphics\weapons\rifles\carcano\carcano_body_n.tga`},
		{Path: `\graphics\weapons\rifles\rifle_x\rifle_x_a.tga`}, // unrelated mesh, must not match
	}

	albedo, normal := meshTextures("carcano", textures)
	if albedo == nil || albedo.Path != `\graphics\weapons\rifles\carcano\carcano_body_a.tga` {
		t.Errorf("albedo = %v, want carcano_body_a", albedo)
	}
	if normal == nil || normal.Path != `\graphics\weapons\rifles\carcano\carcano_body_n.tga` {
		t.Errorf("normal = %v, want carcano_body_n", normal)
	}

	// An LOD-prefixed mesh path must match the same textures as its base.
	albedoLOD, _ := meshTextures("l1#carcano", textures)
	if albedoLOD == nil || albedoLOD.Path != albedo.Path {
		t.Errorf("l1#carcano albedo = %v, want the same match as the base mesh", albedoLOD)
	}

	// A mesh with no matching folder gets nothing.
	noAlbedo, noNormal := meshTextures("unrelated_prop", textures)
	if noAlbedo != nil || noNormal != nil {
		t.Errorf("unrelated_prop matched (%v, %v), want no match", noAlbedo, noNormal)
	}
}

func TestMeshTexturesRequiresExactPathSegment(t *testing.T) {
	// "carcanoscope" contains "carcano" as a substring but isn't the same path segment — must
	// not match a mesh named "carcano".
	textures := []asura.TextureEntry{
		{Path: `\graphics\weapons\scopes\carcanoscope\carcanoscope_a.tga`},
	}
	albedo, normal := meshTextures("carcano", textures)
	if albedo != nil || normal != nil {
		t.Errorf("matched (%v, %v) via substring, want an exact path-segment match only", albedo, normal)
	}
}

// TestMeshTexturesFallsBackToSharedParentFolder regression-tests the Sniper Elite 5/Resistance
// pattern found by direct survey: several sub-part meshes of one larger object (no exact folder
// or filename match of their own) share one parent-object texture set whose own filenames use a
// different vocabulary for the specific part. meshTextures must find the shared parent by
// progressively stripping trailing "_word" segments off the mesh's own name.
func TestMeshTexturesFallsBackToSharedParentFolder(t *testing.T) {
	textures := []asura.TextureEntry{
		{Path: `graphics\vehicles\german_heavy_truck\german_heavy_truck_cab_ar.png`},
		{Path: `graphics\vehicles\german_heavy_truck\german_heavy_truck_cab_n.png`},
		{Path: `graphics\vehicles\german_heavy_truck2\german_heavy_truck2_cab_ar.png`}, // unrelated, must not match
	}

	albedo, normal := meshTextures("german_heavy_truck_door_right", textures)
	if albedo == nil || albedo.Path != `graphics\vehicles\german_heavy_truck\german_heavy_truck_cab_ar.png` {
		t.Errorf("albedo = %v, want the shared parent folder's own albedo texture", albedo)
	}
	if normal == nil || normal.Path != `graphics\vehicles\german_heavy_truck\german_heavy_truck_cab_n.png` {
		t.Errorf("normal = %v, want the shared parent folder's own normal texture", normal)
	}
}

// TestMeshTexturesMatchesByFilenameStem regression-tests the other Sniper Elite 5/Resistance
// pattern found by survey: some objects have no per-object subfolder at all, only a
// generically-named one (e.g. "graphics\pickups\"), with the specific object identifiable only
// from each texture's own filename (role suffix stripped).
func TestMeshTexturesMatchesByFilenameStem(t *testing.T) {
	textures := []asura.TextureEntry{
		{Path: `graphics\pickups\pickup_crate_ar.png`},
		{Path: `graphics\pickups\pickup_crate_n.png`},
		{Path: `graphics\pickups\other_pickup_ar.png`}, // unrelated, must not match
	}

	albedo, normal := meshTextures("pickup_crate", textures)
	if albedo == nil || albedo.Path != `graphics\pickups\pickup_crate_ar.png` {
		t.Errorf("albedo = %v, want pickup_crate_ar via filename-stem match", albedo)
	}
	if normal == nil || normal.Path != `graphics\pickups\pickup_crate_n.png` {
		t.Errorf("normal = %v, want pickup_crate_n via filename-stem match", normal)
	}
}

// TestMeshTexturesStopsAtFirstMatchingCandidate makes sure the progressive trailing-word
// stripping stops as soon as it finds a real match, rather than continuing to strip further and
// potentially drifting onto an unrelated, overly generic identifier that also happens to match.
func TestMeshTexturesStopsAtFirstMatchingCandidate(t *testing.T) {
	textures := []asura.TextureEntry{
		{Path: `graphics\props\widget_small_ar.png`},  // matches the full, specific name
		{Path: `graphics\props\widget_unrelated.png`}, // would match the over-stripped "widget" alone
	}
	albedo, _ := meshTextures("widget_small", textures)
	if albedo == nil || albedo.Path != `graphics\props\widget_small_ar.png` {
		t.Errorf("albedo = %v, want the most specific match (widget_small), not a further-stripped one", albedo)
	}
}

func TestStripTrailingWord(t *testing.T) {
	cases := []struct{ in, want string }{
		{"german_heavy_truck_door_right", "german_heavy_truck_door"},
		{"german_heavy_truck", "german_heavy"},
		{"ab_cd", ""}, // stripping "cd" leaves "ab", too short (< 3 chars)
		{"abc", ""},   // no underscore to strip
		{"", ""},
	}
	for _, c := range cases {
		if got := stripTrailingWord(c.in); got != c.want {
			t.Errorf("stripTrailingWord(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func meshNamed(path string) asura.Mesh {
	return asura.Mesh{
		Path:      path,
		Vertices:  []asura.MeshVertex{{Position: [3]float32{0, 0, 0}}},
		Triangles: [][3]uint16{},
	}
}

func groupBases(groups []meshLODGroup) []string {
	bases := make([]string, len(groups))
	for i, g := range groups {
		bases[i] = g.base
	}
	return bases
}

func TestLODLabel(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"carcano", "LOD0"},
		{"l1#carcano", "LOD1"},
		{"l6#chandelier_long_base", "LOD6"},
		{"bulb_b_destroyed", "LOD0_Destroyed"},
		{"l1#bulb_b_destroyed", "LOD1_Destroyed"},
	}
	for _, c := range cases {
		if got := lodLabel(c.path); got != c.want {
			t.Errorf("lodLabel(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestGroupMeshesByBaseCollectsLODs(t *testing.T) {
	meshes := []asura.Mesh{
		meshNamed("chandelier_long_base"),
		meshNamed("l1#chandelier_long_base"),
		meshNamed("l2#chandelier_long_base"),
		meshNamed("carcano"), // unrelated, no LOD siblings
	}
	groups := groupMeshesByBase(meshes)

	if got, want := groupBases(groups), []string{"chandelier_long_base", "carcano"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group bases = %v, want %v", got, want)
	}
	if len(groups[0].meshes) != 3 {
		t.Errorf("chandelier group has %d meshes, want 3", len(groups[0].meshes))
	}
	if len(groups[1].meshes) != 1 {
		t.Errorf("carcano group has %d meshes, want 1 (no LOD siblings)", len(groups[1].meshes))
	}
}

func TestGroupMeshesByBaseFoldsDestroyedIntoHealthyCounterpart(t *testing.T) {
	meshes := []asura.Mesh{
		meshNamed("bulb_b"),
		meshNamed("bulb_b_destroyed"),
		meshNamed("l1#bulb_b_destroyed"),
	}
	groups := groupMeshesByBase(meshes)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1 (destroyed variants folded into the healthy one)", len(groups))
	}
	if groups[0].base != "bulb_b" {
		t.Errorf("group base = %q, want \"bulb_b\"", groups[0].base)
	}
	var paths []string
	for _, m := range groups[0].meshes {
		paths = append(paths, m.Path)
	}
	want := []string{"bulb_b", "bulb_b_destroyed", "l1#bulb_b_destroyed"}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("group meshes = %v, want %v", paths, want)
	}
}

func TestGroupMeshesByBaseLeavesOrphanDestroyedMeshAlone(t *testing.T) {
	// "some_prop_destroyed" with no "some_prop" counterpart in this package: folding it under
	// "some_prop" would misname its output file for a mesh that was never actually healthy here.
	meshes := []asura.Mesh{meshNamed("some_prop_destroyed")}
	groups := groupMeshesByBase(meshes)

	if len(groups) != 1 || groups[0].base != "some_prop_destroyed" {
		t.Fatalf("groups = %+v, want a single untouched \"some_prop_destroyed\" group", groups)
	}
}

func TestWriteOBJGroupOffsetsVertexIndices(t *testing.T) {
	m1 := asura.Mesh{
		Path: "carcano",
		Vertices: []asura.MeshVertex{
			{Position: [3]float32{0, 0, 0}}, {Position: [3]float32{1, 0, 0}}, {Position: [3]float32{0, 1, 0}},
		},
		Triangles: [][3]uint16{{0, 1, 2}},
	}
	m2 := asura.Mesh{
		Path: "l1#carcano",
		Vertices: []asura.MeshVertex{
			{Position: [3]float32{2, 0, 0}}, {Position: [3]float32{3, 0, 0}}, {Position: [3]float32{2, 1, 0}},
		},
		Triangles: [][3]uint16{{0, 1, 2}},
	}

	var buf bytes.Buffer
	if err := writeOBJGroup(&buf, "carcano", []asura.Mesh{m1, m2}); err != nil {
		t.Fatalf("writeOBJGroup: %v", err)
	}
	out := buf.String()

	vCount := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "v ") {
			vCount++
		}
	}
	if vCount != 6 {
		t.Errorf("got %d \"v\" lines, want 6 (3 per mesh)", vCount)
	}

	if !strings.Contains(out, "o LOD0\ng LOD0\n") {
		t.Errorf("missing base mesh's o/g block (want \"LOD0\", not its raw path); output:\n%s", out)
	}
	if !strings.Contains(out, "o LOD1\ng LOD1\n") {
		t.Errorf("missing LOD1's o/g block; output:\n%s", out)
	}

	// The second mesh's face indices must be offset by the first mesh's 3 vertices: its
	// triangle {0,1,2} (flipped to {0,2,1}) should reference OBJ vertices 4/6/5, not 1/3/2.
	if !strings.Contains(out, "f 4/4 6/6 5/5\n") {
		t.Errorf("second mesh's face indices weren't offset by the first mesh's vertex count; output:\n%s", out)
	}
	// The first mesh's own face should be untouched (offset 0): triangle {0,1,2} -> {0,2,1} -> 1/3/2.
	if !strings.Contains(out, "f 1/1 3/3 2/2\n") {
		t.Errorf("first mesh's face indices are wrong; output:\n%s", out)
	}
}
