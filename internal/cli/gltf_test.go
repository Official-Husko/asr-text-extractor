package cli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

// parseGLB unpacks a .glb container's own framing (see writeGLBContainer) back into its JSON
// document and binary chunk, so tests can assert on writeGLB's actual output shape rather than
// just the math helpers it's built from.
func parseGLB(t *testing.T, data []byte) (gltfDocument, []byte) {
	t.Helper()
	if len(data) < 12 || string(data[0:4]) != "glTF" {
		t.Fatalf("not a glb file (bad magic)")
	}
	total := binary.LittleEndian.Uint32(data[8:12])
	if int(total) != len(data) {
		t.Fatalf("header length %d doesn't match actual file length %d", total, len(data))
	}

	pos := 12
	jsonLen := binary.LittleEndian.Uint32(data[pos : pos+4])
	if string(data[pos+4:pos+8]) != "JSON" {
		t.Fatalf("expected JSON chunk at %d", pos)
	}
	jsonData := data[pos+8 : pos+8+int(jsonLen)]
	pos += 8 + int(jsonLen)

	binLen := binary.LittleEndian.Uint32(data[pos : pos+4])
	if string(data[pos+4:pos+8]) != "BIN\x00" {
		t.Fatalf("expected BIN chunk at %d", pos)
	}
	binData := data[pos+8 : pos+8+int(binLen)]

	var doc gltfDocument
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		t.Fatalf("unmarshaling JSON chunk: %v", err)
	}
	return doc, binData
}

func simpleTestMesh() asura.Mesh {
	return asura.Mesh{
		Path: "widget",
		Vertices: []asura.MeshVertex{
			{Position: [3]float32{0, 0, 0}, UV0: [2]float32{0, 0}},
			{Position: [3]float32{1, 0, 0}, UV0: [2]float32{1, 0}},
			{Position: [3]float32{0, 1, 0}, UV0: [2]float32{0, 1}},
		},
		Triangles: [][3]uint16{{0, 1, 2}},
	}
}

func TestWriteGLBUnskinnedShape(t *testing.T) {
	var buf bytes.Buffer
	if err := writeGLB(&buf, simpleTestMesh()); err != nil {
		t.Fatalf("writeGLB: %v", err)
	}
	doc, _ := parseGLB(t, buf.Bytes())

	if len(doc.Meshes) != 1 || len(doc.Meshes[0].Primitives) != 1 {
		t.Fatalf("got %d meshes, want 1 with 1 primitive", len(doc.Meshes))
	}
	attrs := doc.Meshes[0].Primitives[0].Attributes
	if _, ok := attrs["JOINTS_0"]; ok {
		t.Errorf("unskinned mesh has a JOINTS_0 attribute, want none")
	}
	if len(doc.Skins) != 0 {
		t.Errorf("unskinned mesh has %d skins, want 0", len(doc.Skins))
	}
	posAcc := doc.Accessors[attrs["POSITION"]]
	if posAcc.Count != 3 {
		t.Errorf("POSITION accessor count = %d, want 3", posAcc.Count)
	}
	idxAcc := doc.Accessors[doc.Meshes[0].Primitives[0].Indices]
	if idxAcc.Count != 3 {
		t.Errorf("indices accessor count = %d, want 3", idxAcc.Count)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].Mesh == nil {
		t.Fatalf("want exactly 1 mesh node, got %+v", doc.Nodes)
	}
}

func TestWriteGLBSkinnedShape(t *testing.T) {
	m := simpleTestMesh()
	m.Vertices[0].BoneIDs, m.Vertices[0].BoneWeights = [8]uint8{0}, [8]uint8{255}
	m.Vertices[1].BoneIDs, m.Vertices[1].BoneWeights = [8]uint8{1}, [8]uint8{255}
	m.Vertices[2].BoneIDs, m.Vertices[2].BoneWeights = [8]uint8{1}, [8]uint8{255}
	m.Skeleton = &asura.Skeleton{
		Name: "Widget",
		Bones: []asura.Bone{
			{Name: "Body", LocalRot: [4]float32{1, 0, 0, 0}},
			{Name: "Bolt", LocalPos: [3]float32{0, -0.05, -0.02}, LocalRot: [4]float32{1, 0, 0, 0}},
		},
	}

	var buf bytes.Buffer
	if err := writeGLB(&buf, m); err != nil {
		t.Fatalf("writeGLB: %v", err)
	}
	doc, _ := parseGLB(t, buf.Bytes())

	if len(doc.Skins) != 1 {
		t.Fatalf("got %d skins, want 1", len(doc.Skins))
	}
	skin := doc.Skins[0]
	if len(skin.Joints) != 2 {
		t.Fatalf("got %d joints, want 2", len(skin.Joints))
	}
	if got := doc.Nodes[skin.Joints[0]].Name; got != "Body" {
		t.Errorf("joint 0 node name = %q, want \"Body\"", got)
	}
	if got := doc.Nodes[skin.Joints[1]].Name; got != "Bolt" {
		t.Errorf("joint 1 node name = %q, want \"Bolt\"", got)
	}
	boltNode := doc.Nodes[skin.Joints[1]]
	wantTrans := vec3ToFloat64([3]float32{0, -0.05, -0.02}) // identity rotation: translation passes through unchanged
	for i := range wantTrans {
		if math.Abs(boltNode.Translation[i]-wantTrans[i]) > 1e-4 {
			t.Errorf("bolt node translation = %v, want %v", boltNode.Translation, wantTrans)
		}
	}

	invBindAcc := doc.Accessors[skin.InverseBindMatrices]
	if invBindAcc.Type != "MAT4" || invBindAcc.Count != 2 {
		t.Errorf("inverseBindMatrices accessor = %+v, want MAT4 x2", invBindAcc)
	}

	attrs := doc.Meshes[0].Primitives[0].Attributes
	if _, ok := attrs["JOINTS_0"]; !ok {
		t.Errorf("skinned mesh missing JOINTS_0 attribute")
	}
	if _, ok := attrs["WEIGHTS_0"]; !ok {
		t.Errorf("skinned mesh missing WEIGHTS_0 attribute")
	}
	if _, ok := attrs["JOINTS_1"]; ok {
		t.Errorf("mesh with no vertex using influence slots 4-7 has a JOINTS_1 attribute, want none")
	}

	// One root node (armature) + 2 joint nodes + 1 mesh node.
	if len(doc.Nodes) != 4 {
		t.Fatalf("got %d nodes, want 4 (root + 2 joints + mesh)", len(doc.Nodes))
	}
	meshNodeIdx := -1
	for i, n := range doc.Nodes {
		if n.Mesh != nil {
			meshNodeIdx = i
		}
	}
	if meshNodeIdx < 0 {
		t.Fatalf("no node has a Mesh reference")
	}
	if doc.Nodes[meshNodeIdx].Skin == nil {
		t.Errorf("mesh node has no Skin reference")
	}
}

func vecAlmostEqual(a, b [3]float32) bool {
	const eps = 1e-4
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > eps {
			return false
		}
	}
	return true
}

func mat4MulPoint(m [16]float32, v [3]float32) [3]float32 {
	return [3]float32{
		m[0]*v[0] + m[4]*v[1] + m[8]*v[2] + m[12],
		m[1]*v[0] + m[5]*v[1] + m[9]*v[2] + m[13],
		m[2]*v[0] + m[6]*v[1] + m[10]*v[2] + m[14],
	}
}

// TestInverseBindMatrixCancelsAtBindPose is the numeric self-consistency check writeGLB's whole
// skinning export depends on, since this project has no way to visually verify a glTF file in
// Blender the way its earlier OBJ/skeleton work was checked (see skeleton.go's Skin doc
// comment). It composes a bone's own bind-pose node transform (translation =
// quatRotateVec(rot, pos), rotation = rot — exactly what writeGLB puts on that bone's node) with
// inverseBindMatrix's result, and checks the composition is the identity — the condition glTF's
// skinning formula (jointGlobalTransform * inverseBindMatrix * vertex) requires so that a mesh
// renders unchanged at rest, before any posing is applied. It also cross-checks that the same
// composed transform reproduces asura.Skeleton.Skin's own already-validated
// `rotate(rot, local + pos)` formula on a raw vertex position, so the glTF export and the
// existing OBJ export's positions can't silently diverge.
func TestInverseBindMatrixCancelsAtBindPose(t *testing.T) {
	cases := []struct {
		name string
		bone asura.Bone
	}{
		{"identity", asura.Bone{LocalPos: [3]float32{2, 3, 4}, LocalRot: [4]float32{1, 0, 0, 0}}},
		{
			"90-about-Z-with-offset",
			asura.Bone{
				LocalPos: [3]float32{0, -1, 0},
				LocalRot: [4]float32{float32(math.Sqrt(0.5)), 0, 0, float32(math.Sqrt(0.5))},
			},
		},
		{
			"180-about-Z-with-offset", // the exact shape of the real bug this project hit once
			asura.Bone{LocalPos: [3]float32{0, -1, 0}, LocalRot: [4]float32{0, 0, 0, 1}},
		},
	}

	raw := [3]float32{1, 0.5, -2}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			forward := composeMat4(c.bone.LocalRot, quatRotateVec(c.bone.LocalRot, c.bone.LocalPos))
			inv := inverseBindMatrix(c.bone)

			got := mat4MulPoint(forward, mat4MulPoint(inv, raw))
			if !vecAlmostEqual(got, raw) {
				t.Errorf("forward*inverseBind*raw = %v, want unchanged %v (bind pose isn't inert)", got, raw)
			}

			gotSkin := mat4MulPoint(forward, raw)
			sk := &asura.Skeleton{Bones: []asura.Bone{c.bone}}
			mesh := &asura.Mesh{Vertices: []asura.MeshVertex{
				{Position: raw, BoneIDs: [8]uint8{0}, BoneWeights: [8]uint8{255}},
			}}
			wantSkin := sk.Skin(mesh)[0]
			if !vecAlmostEqual(gotSkin, wantSkin) {
				t.Errorf("forward*raw = %v, want asura.Skeleton.Skin's own result %v", gotSkin, wantSkin)
			}
		})
	}
}

func TestQuatToGLTFReordersScalarLast(t *testing.T) {
	got := quatToGLTF([4]float32{1, 2, 3, 4}) // w, x, y, z
	want := []float64{2, 3, 4, 1}             // x, y, z, w
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("quatToGLTF = %v, want %v", got, want)
		}
	}
}
