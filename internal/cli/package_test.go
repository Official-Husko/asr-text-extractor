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

	if !strings.Contains(out, "o carcano\ng carcano\n") {
		t.Errorf("missing base mesh's o/g block; output:\n%s", out)
	}
	if !strings.Contains(out, "o l1#carcano\ng l1#carcano\n") {
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
