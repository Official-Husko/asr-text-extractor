package asura

import (
	"bytes"
	"math"
	"testing"
)

// buildMeshPayload constructs a synthetic mesh payload in the shape confirmed against a
// dedicated, independently-authored Zombie Army 4 reverse-engineering project's own working
// Blender importer (see mesh.go's doc comments): a header (5 uint32 fields, then one 24-byte
// record per group, then a 3-float32 scale and a 3-float32 offset), a vertCount*48-byte vertex
// buffer, and an indexCount*2-byte index buffer.
func buildMeshPayload(t *testing.T, groups []MeshGroup, scale, offset [3]float32, verts [][3]uint16, uv0, uv1 [][2]uint16, tris [][3]uint16) []byte {
	t.Helper()
	indexCount := len(tris) * 3

	var buf bytes.Buffer
	writeU32(&buf, uint32(len(groups)))
	writeU32(&buf, uint32(len(verts)))
	writeU32(&buf, uint32(indexCount))
	writeU32(&buf, uint32(len(tris))) // "polygon count", redundant with indexCount/3
	writeU32(&buf, 0)                 // unconfirmed field

	for _, g := range groups {
		writeU32(&buf, g.Hash)
		writeU32(&buf, 0) // unconfirmed
		writeU32(&buf, uint32(g.IndexCount))
		writeU32(&buf, 0) // unconfirmed
		writeU32(&buf, 0) // unconfirmed
		writeU32(&buf, 0) // unconfirmed
	}

	for _, s := range scale {
		writeU32(&buf, math.Float32bits(s))
	}
	for _, o := range offset {
		writeU32(&buf, math.Float32bits(o))
	}

	for i, v := range verts {
		writeU16(&buf, v[0])
		writeU16(&buf, v[1])
		writeU16(&buf, v[2])
		buf.Write(make([]byte, 18)) // minus_one + normals + unk + unk_const, pad to UV offset (24)
		writeU16(&buf, uv0[i][0])
		writeU16(&buf, uv0[i][1])
		writeU16(&buf, uv1[i][0])
		writeU16(&buf, uv1[i][1])
		buf.Write(make([]byte, 16)) // weights + bone_ids, pad out the rest of the stride
	}

	for _, tri := range tris {
		writeU16(&buf, tri[0])
		writeU16(&buf, tri[1])
		writeU16(&buf, tri[2])
	}

	return buf.Bytes()
}

func writeU16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	buf.Write(b[:])
}

func TestParseMesh(t *testing.T) {
	verts := [][3]uint16{
		{0, 0, 0},
		{32767, 0, 0},
		{0, 32767, 0},
		{0, 0, 32767},
	}
	uv0 := [][2]uint16{
		{0x0000, 0x3C00}, // 0.0, 1.0
		{0x3C00, 0x0000}, // 1.0, 0.0
		{0x3800, 0x3800}, // 0.5, 0.5
		{0x0000, 0x0000}, // 0.0, 0.0
	}
	uv1 := [][2]uint16{
		{0x3C00, 0x3C00},
		{0x3C00, 0x3C00},
		{0x3C00, 0x3C00},
		{0x3C00, 0x3C00},
	}
	tris := [][3]uint16{{0, 1, 2}, {0, 2, 3}}
	scale := [3]float32{2.0, -4.0, 0.5}
	offset := [3]float32{1.0, 0.0, -0.5}
	groups := []MeshGroup{{Hash: 0xDEADBEEF, IndexCount: 6}}
	payload := buildMeshPayload(t, groups, scale, offset, verts, uv0, uv1, tris)

	m, err := ParseMesh("test_object", payload)
	if err != nil {
		t.Fatalf("ParseMesh: %v", err)
	}
	if m.Path != "test_object" {
		t.Errorf("Path = %q", m.Path)
	}
	if len(m.Vertices) != 4 {
		t.Fatalf("got %d vertices, want 4", len(m.Vertices))
	}
	if len(m.Triangles) != 2 {
		t.Fatalf("got %d triangles, want 2", len(m.Triangles))
	}
	if len(m.Groups) != 1 || m.Groups[0].Hash != 0xDEADBEEF || m.Groups[0].IndexCount != 6 {
		t.Errorf("Groups = %+v", m.Groups)
	}

	// vertex 1: raw pos (32767,0,0) -> (32767/32767)*(scale/2)+offset = 1*(2.0/2)+1.0 = 2.0
	// for X; raw 0 for Y,Z -> 0*(scale/2)+offset = offset.
	want := MeshVertex{
		Position: [3]float32{2.0, offset[1], offset[2]},
		UV0:      [2]float32{1, 0},
		UV1:      [2]float32{1, 1},
	}
	got := m.Vertices[1]
	if !vecAlmostEqual(got.Position, want.Position) {
		t.Errorf("vertex 1 Position = %v, want %v", got.Position, want.Position)
	}
	if !almostEqual(got.UV0[0], want.UV0[0]) || !almostEqual(got.UV0[1], want.UV0[1]) {
		t.Errorf("vertex 1 UV0 = %v, want %v", got.UV0, want.UV0)
	}
	if !almostEqual(got.UV1[0], want.UV1[0]) || !almostEqual(got.UV1[1], want.UV1[1]) {
		t.Errorf("vertex 1 UV1 = %v, want %v", got.UV1, want.UV1)
	}

	if m.Triangles[0] != [3]uint16{0, 1, 2} {
		t.Errorf("triangle 0 = %v", m.Triangles[0])
	}
	if m.Triangles[1] != [3]uint16{0, 2, 3} {
		t.Errorf("triangle 1 = %v", m.Triangles[1])
	}
}

func TestParseMeshMultiGroup(t *testing.T) {
	verts := [][3]uint16{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	uv0 := [][2]uint16{{0, 0}, {0, 0}, {0, 0}, {0, 0}}
	uv1 := uv0
	tris := [][3]uint16{{0, 1, 2}, {1, 2, 3}}
	groups := []MeshGroup{{Hash: 1, IndexCount: 3}, {Hash: 2, IndexCount: 3}}
	payload := buildMeshPayload(t, groups, [3]float32{1, 1, 1}, [3]float32{0, 0, 0}, verts, uv0, uv1, tris)

	m, err := ParseMesh("multi_group", payload)
	if err != nil {
		t.Fatalf("ParseMesh: %v", err)
	}
	if len(m.Groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(m.Groups))
	}
	if len(m.Triangles) != 2 {
		t.Fatalf("got %d triangles, want 2", len(m.Triangles))
	}
}

func TestParseMeshSizeMismatch(t *testing.T) {
	verts := [][3]uint16{{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}}
	uv0 := [][2]uint16{{0, 0}, {0, 0}, {0, 0}, {0, 0}}
	tris := [][3]uint16{{0, 1, 2}}
	payload := buildMeshPayload(t, []MeshGroup{{Hash: 1, IndexCount: 3}}, [3]float32{1, 1, 1}, [3]float32{0, 0, 0}, verts, uv0, uv0, tris)
	// Corrupt the payload by truncating it, so the declared counts no longer reconcile with
	// the actual length.
	payload = payload[:len(payload)-1]
	if _, err := ParseMesh("truncated", payload); err == nil {
		t.Fatal("expected an error when the payload size doesn't match the header-predicted size")
	}
}

func TestParseMeshTooShort(t *testing.T) {
	if _, err := ParseMesh("short", make([]byte, 10)); err == nil {
		t.Fatal("expected an error for a payload shorter than even the fixed part of the header")
	}
}

func TestRSCFEntryAsMesh(t *testing.T) {
	verts := [][3]uint16{{0, 0, 0}}
	uv0 := [][2]uint16{{0, 0}}
	payload := buildMeshPayload(t, nil, [3]float32{1, 1, 1}, [3]float32{0, 0, 0}, verts, uv0, uv0, nil)
	e := &rscfEntry{path: "obj", resType: rscfResourceTypeMesh, payload: payload}
	if m := e.asMesh(); m == nil {
		t.Fatal("asMesh() = nil for a valid mesh entry")
	}

	texEntry := &rscfEntry{path: "obj", resType: rscfResourceTypeTexture, payload: payload}
	if m := texEntry.asMesh(); m != nil {
		t.Error("asMesh() should be nil for a non-mesh resource type")
	}

	badEntry := &rscfEntry{path: "inst (dynamic)", resType: rscfResourceTypeMesh, payload: []byte("not a mesh, too short")}
	if m := badEntry.asMesh(); m != nil {
		t.Error("asMesh() should be nil for a resource-type-0 entry that doesn't decode as a mesh")
	}
}

func vecAlmostEqual(a, b [3]float32) bool {
	return almostEqual(a[0], b[0]) && almostEqual(a[1], b[1]) && almostEqual(a[2], b[2])
}

func almostEqual(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-3
}

func TestFloat16ToFloat32(t *testing.T) {
	cases := []struct {
		bits uint16
		want float32
	}{
		{0x0000, 0},
		{0x8000, 0}, // -0, compared as 0 below via almostEqual
		{0x3C00, 1.0},
		{0xC000, -2.0},
		{0x3800, 0.5},
		{0x4900, 10.0},
	}
	for _, c := range cases {
		got := float16ToFloat32(c.bits)
		if !almostEqual(got, c.want) {
			t.Errorf("float16ToFloat32(%#04x) = %v, want %v", c.bits, got, c.want)
		}
	}
}
