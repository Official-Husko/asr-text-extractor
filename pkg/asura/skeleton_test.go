package asura

import (
	"bytes"
	"math"
	"testing"
)

// writePaddedString writes a NUL-terminated string padded so its total length (string + NUL +
// padding) is a multiple of 4, counted from wherever the string itself starts in buf — the
// same reference frame alignedString's own chunked reading uses (chunks counted from its pos
// argument, not from the start of the overall data it's reading from). Padding relative to
// buf's own absolute length instead would be wrong whenever the string doesn't happen to start
// at a multiple of 4 within buf — exactly the mistake an earlier version of this fixture made,
// masked by the fact that it usually doesn't matter... until it does.
func writePaddedString(buf *bytes.Buffer, s string) {
	start := buf.Len()
	buf.WriteString(s)
	buf.WriteByte(0)
	for (buf.Len()-start)%4 != 0 {
		buf.WriteByte(0)
	}
}

// buildHSKN constructs a synthetic HSKN section in the shape confirmed against a real Zombie
// Army 4 "Carcano" skeleton (version 29): a ChunkHeader, an unused uint32 (kept 0, so the
// version>=25-conditional skip never triggers — not exercised by any real sample), a bone
// count, a padded name, boneCount parent indices, boneCount {pos, rot} bind-pose records, then
// (if version>=10) a single byte followed by boneCount padded bone names each followed by a
// uint32-length-prefixed (kept empty here) data blob.
func buildHSKN(t *testing.T, name string, version, flags uint32, parents []uint32, poses [][3]float32, rots [][4]float32, boneNames []string) []byte {
	t.Helper()
	var body bytes.Buffer
	writePaddedString(&body, name)
	for _, p := range parents {
		writeU32(&body, p)
	}
	for i := range parents {
		for _, f := range poses[i] {
			writeU32(&body, math.Float32bits(f))
		}
		for _, f := range rots[i] {
			writeU32(&body, math.Float32bits(f))
		}
	}
	if version >= 10 {
		body.WriteByte(0)
	}
	for i := range parents {
		writePaddedString(&body, boneNames[i])
		if version >= 10 {
			body.WriteByte(1) // non-zero: a zero here would fool alignedString's next read
		}
		// A non-zero data length with real (non-zero) bytes — an all-zero blob here would
		// extend the padding's zero run into this field, the same class of ambiguity
		// alignedString's chunk-based reading exists to resolve correctly against real data,
		// which never has an all-zero run at this exact spot either.
		writeU32(&body, 3)
		body.Write([]byte{1, 2, 3})
	}

	var out bytes.Buffer
	out.WriteString("HSKN")
	writeU32(&out, uint32(16+8+body.Len())) // size field, not consulted by ParseSkeleton
	writeU32(&out, version)
	writeU32(&out, flags)
	writeU32(&out, 0) // unk
	writeU32(&out, uint32(len(parents)))
	out.Write(body.Bytes())
	return out.Bytes()
}

func TestParseSkeleton(t *testing.T) {
	parents := []uint32{0, 0, 0}
	poses := [][3]float32{
		{0, 0, 0},
		{1, 2, 3},
		{-1, 0.5, 0},
	}
	rots := [][4]float32{
		{1, 0, 0, 0},
		{1, 0, 0, 0},
		{1, 0, 0, 0},
	}
	names := []string{"Body", "Bolt", "Trigger"}
	data := buildHSKN(t, "Carcano", 29, 0xC9, parents, poses, rots, names)

	sk, err := ParseSkeleton(data)
	if err != nil {
		t.Fatalf("ParseSkeleton: %v", err)
	}
	if sk.Name != "Carcano" {
		t.Errorf("Name = %q", sk.Name)
	}
	if len(sk.Bones) != 3 {
		t.Fatalf("got %d bones, want 3", len(sk.Bones))
	}
	for i, want := range names {
		if sk.Bones[i].Name != want {
			t.Errorf("bone %d Name = %q, want %q", i, sk.Bones[i].Name, want)
		}
		if sk.Bones[i].LocalPos != poses[i] {
			t.Errorf("bone %d LocalPos = %v, want %v", i, sk.Bones[i].LocalPos, poses[i])
		}
		if sk.Bones[i].ParentIndex != int(parents[i]) {
			t.Errorf("bone %d ParentIndex = %d, want %d", i, sk.Bones[i].ParentIndex, parents[i])
		}
	}
}

func TestParseSkeletonBadMagic(t *testing.T) {
	if _, err := ParseSkeleton([]byte("not an HSKN section, but long enough to pass the length check")); err == nil {
		t.Fatal("expected an error for data not starting with a plausible HSKN header")
	}
}

func TestSkeletonSkinTranslationOnly(t *testing.T) {
	// A root bone at the origin and a child offset by (0, -0.05, -0.02) — the same shape as
	// the real "Body"/"Bolt" pair this was modeled on (small translation, identity rotation).
	sk := &Skeleton{
		Name: "test",
		Bones: []Bone{
			{Name: "Body", ParentIndex: 0, LocalPos: [3]float32{0, 0, 0}, LocalRot: [4]float32{1, 0, 0, 0}},
			{Name: "Bolt", ParentIndex: 0, LocalPos: [3]float32{0, -0.05, -0.02}, LocalRot: [4]float32{1, 0, 0, 0}},
		},
	}
	mesh := &Mesh{
		Vertices: []MeshVertex{
			{Position: [3]float32{1, 2, 3}, BoneIDs: [8]uint8{0}, BoneWeights: [8]uint8{255}},
			{Position: [3]float32{1, 2, 3}, BoneIDs: [8]uint8{1}, BoneWeights: [8]uint8{255}},
			{Position: [3]float32{0, 0, 0}, BoneIDs: [8]uint8{0}, BoneWeights: [8]uint8{0}}, // unweighted
		},
	}
	got := sk.Skin(mesh)

	if !vecAlmostEqual(got[0], [3]float32{1, 2, 3}) {
		t.Errorf("bone-0 vertex = %v, want unchanged {1,2,3}", got[0])
	}
	want1 := [3]float32{1, 2 - 0.05, 3 - 0.02}
	if !vecAlmostEqual(got[1], want1) {
		t.Errorf("bone-1 vertex = %v, want %v", got[1], want1)
	}
	if !vecAlmostEqual(got[2], [3]float32{0, 0, 0}) {
		t.Errorf("unweighted vertex = %v, want unchanged origin", got[2])
	}
}

func TestSkeletonSkinAppliesRotation(t *testing.T) {
	// A root bone with a genuine 90-degree rotation about Z (not identity) — bone transforms
	// apply directly, no parent composition (see worldTransforms' doc comment for why).
	quarterTurnZ := [4]float32{float32(math.Sqrt(0.5)), 0, 0, float32(math.Sqrt(0.5))} // w,x,y,z
	sk := &Skeleton{
		Bones: []Bone{
			{ParentIndex: 0, LocalPos: [3]float32{0, 0, 0}, LocalRot: quarterTurnZ},
		},
	}
	mesh := &Mesh{
		Vertices: []MeshVertex{
			{Position: [3]float32{1, 0, 0}, BoneIDs: [8]uint8{0}, BoneWeights: [8]uint8{255}},
		},
	}
	got := sk.Skin(mesh)[0]
	want := [3]float32{0, 1, 0} // (1,0,0) rotated 90 degrees about Z
	if !vecAlmostEqual(got, want) {
		t.Errorf("vertex = %v, want %v", got, want)
	}
}

// TestSkeletonSkinChildDoesNotInheritParentRotation regression-tests the actual bug found
// against a real sample ("carcano"): a root bone with a genuine 180-degree rotation (not
// identity), and a child bone parented to it sharing that *same* rotation and a small
// translation offset. Two earlier, individually-wrong formulas were tried and user-corrected
// against a real render before landing on Skin's current one — see its doc comment for the
// full derivation. This test locks in that a root-bone vertex and a child-bone vertex both end
// up correctly, consistently rotated (not one rotated and the other not, which is what both
// earlier attempts got wrong in different ways).
func TestSkeletonSkinChildDoesNotInheritParentRotation(t *testing.T) {
	rot180Z := [4]float32{0, 0, 0, 1} // w,x,y,z: a genuine 180-degree rotation about Z
	sk := &Skeleton{
		Bones: []Bone{
			{ParentIndex: 0, LocalPos: [3]float32{0, 0, 0}, LocalRot: rot180Z},
			{ParentIndex: 0, LocalPos: [3]float32{0, -1, 0}, LocalRot: rot180Z},
		},
	}
	mesh := &Mesh{
		Vertices: []MeshVertex{
			{Position: [3]float32{1, 0, 0}, BoneIDs: [8]uint8{0}, BoneWeights: [8]uint8{255}},
			{Position: [3]float32{1, 0, 0}, BoneIDs: [8]uint8{1}, BoneWeights: [8]uint8{255}},
		},
	}
	got := sk.Skin(mesh)
	// Root: rotate180Z(vertex + offset(0,0,0)) = rotate180Z(1,0,0) = (-1,0,0).
	wantRoot := [3]float32{-1, 0, 0}
	if !vecAlmostEqual(got[0], wantRoot) {
		t.Errorf("root-bone vertex = %v, want %v", got[0], wantRoot)
	}
	// Child: rotate180Z(vertex + offset(0,-1,0)) = rotate180Z(1,-1,0) = (-1,1,0).
	wantChild := [3]float32{-1, 1, 0}
	if !vecAlmostEqual(got[1], wantChild) {
		t.Errorf("child-bone vertex = %v, want %v (this is the exact bug shape found in real data)", got[1], wantChild)
	}
}

func TestSkeletonSkinBlendedWeights(t *testing.T) {
	sk := &Skeleton{
		Bones: []Bone{
			{ParentIndex: 0, LocalPos: [3]float32{0, 0, 0}, LocalRot: [4]float32{1, 0, 0, 0}},
			{ParentIndex: 0, LocalPos: [3]float32{10, 0, 0}, LocalRot: [4]float32{1, 0, 0, 0}},
		},
	}
	mesh := &Mesh{
		Vertices: []MeshVertex{
			{Position: [3]float32{0, 0, 0}, BoneIDs: [8]uint8{0, 1}, BoneWeights: [8]uint8{128, 127}},
		},
	}
	got := sk.Skin(mesh)[0]
	// Roughly an even 50/50 blend between bone 0 (offset 0) and bone 1 (offset 10) -> ~5.
	if got[0] < 4.5 || got[0] > 5.5 {
		t.Errorf("blended X = %v, want roughly 5", got[0])
	}
}

func TestQuatRotateVecIdentity(t *testing.T) {
	v := [3]float32{1, 2, 3}
	got := quatRotateVec([4]float32{1, 0, 0, 0}, v)
	if !vecAlmostEqual(got, v) {
		t.Errorf("identity rotation changed vector: got %v, want %v", got, v)
	}
}
