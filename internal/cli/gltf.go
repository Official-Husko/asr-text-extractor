package cli

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/Official-Husko/asr-text-extractor/pkg/asura"
)

// glTF 2.0 accessor component types, buffer view targets, and the .glb container's own chunk
// type tags — see the glTF 2.0 spec, sections 3.6 and 5.
const (
	gltfComponentUByte  = 5121
	gltfComponentUShort = 5123
	gltfComponentFloat  = 5126

	gltfTargetArrayBuffer        = 34962
	gltfTargetElementArrayBuffer = 34963
)

type gltfAsset struct {
	Version   string `json:"version"`
	Generator string `json:"generator,omitempty"`
}

type gltfBuffer struct {
	ByteLength int `json:"byteLength"`
}

type gltfBufferView struct {
	Buffer     int `json:"buffer"`
	ByteOffset int `json:"byteOffset"`
	ByteLength int `json:"byteLength"`
	Target     int `json:"target,omitempty"`
}

type gltfAccessor struct {
	BufferView    int       `json:"bufferView"`
	ComponentType int       `json:"componentType"`
	Count         int       `json:"count"`
	Type          string    `json:"type"`
	Normalized    bool      `json:"normalized,omitempty"`
	Min           []float64 `json:"min,omitempty"`
	Max           []float64 `json:"max,omitempty"`
}

type gltfPrimitive struct {
	Attributes map[string]int `json:"attributes"`
	Indices    int            `json:"indices"`
}

type gltfMesh struct {
	Name       string          `json:"name,omitempty"`
	Primitives []gltfPrimitive `json:"primitives"`
}

type gltfNode struct {
	Name        string    `json:"name,omitempty"`
	Children    []int     `json:"children,omitempty"`
	Translation []float64 `json:"translation,omitempty"`
	Rotation    []float64 `json:"rotation,omitempty"`
	Mesh        *int      `json:"mesh,omitempty"`
	Skin        *int      `json:"skin,omitempty"`
}

type gltfSkin struct {
	Name                string `json:"name,omitempty"`
	InverseBindMatrices int    `json:"inverseBindMatrices"`
	Skeleton            int    `json:"skeleton"`
	Joints              []int  `json:"joints"`
}

type gltfScene struct {
	Nodes []int `json:"nodes"`
}

type gltfDocument struct {
	Asset       gltfAsset        `json:"asset"`
	Scene       int              `json:"scene"`
	Scenes      []gltfScene      `json:"scenes"`
	Nodes       []gltfNode       `json:"nodes"`
	Meshes      []gltfMesh       `json:"meshes,omitempty"`
	Skins       []gltfSkin       `json:"skins,omitempty"`
	Accessors   []gltfAccessor   `json:"accessors,omitempty"`
	BufferViews []gltfBufferView `json:"bufferViews,omitempty"`
	Buffers     []gltfBuffer     `json:"buffers"`
}

// gltfBinBuilder accumulates the single binary buffer backing a .glb file's accessors. Every
// bufferView start is 4-byte-aligned unconditionally — overkill for the UBYTE joint/weight
// views, but it satisfies the FLOAT and MAT4 views' real alignment requirement without needing
// to track per-view alignment individually.
type gltfBinBuilder struct {
	buf   bytes.Buffer
	views []gltfBufferView
}

func (b *gltfBinBuilder) addView(data []byte, target int) int {
	for b.buf.Len()%4 != 0 {
		b.buf.WriteByte(0)
	}
	view := gltfBufferView{ByteOffset: b.buf.Len(), ByteLength: len(data), Target: target}
	b.buf.Write(data)
	b.views = append(b.views, view)
	return len(b.views) - 1
}

func appendF32(dst []byte, f float32) []byte {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], math.Float32bits(f))
	return append(dst, buf[:]...)
}

func appendU16(dst []byte, v uint16) []byte {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], v)
	return append(dst, buf[:]...)
}

func appendMat4(dst []byte, m [16]float32) []byte {
	for _, f := range m {
		dst = appendF32(dst, f)
	}
	return dst
}

func vec3ToFloat64(v [3]float32) []float64 {
	return []float64{float64(v[0]), float64(v[1]), float64(v[2])}
}

// quatToGLTF converts this project's internal quaternion order (w, x, y, z — see
// asura.Bone.LocalRot) to glTF's required scalar-last order (x, y, z, w).
func quatToGLTF(q [4]float32) []float64 {
	return []float64{float64(q[1]), float64(q[2]), float64(q[3]), float64(q[0])}
}

func intPtr(i int) *int { return &i }

// composeMat4 builds a column-major, scale-1 4x4 transform matrix that rotates by rot and then
// translates by trans — the same "T * R" composition order glTF's own separate node.rotation/
// node.translation fields imply (spec section 5.25). Used by inverseBindMatrix below; also
// exercised directly by gltf_test.go to confirm a joint's bind-pose node transform and its
// inverse bind matrix cancel to identity, which is the correctness condition the whole skin
// export in writeGLB depends on.
func composeMat4(rot [4]float32, trans [3]float32) [16]float32 {
	qw, qx, qy, qz := rot[0], rot[1], rot[2], rot[3]
	x2, y2, z2 := qx+qx, qy+qy, qz+qz
	xx, yy, zz := qx*x2, qy*y2, qz*z2
	xy, xz, yz := qx*y2, qx*z2, qy*z2
	wx, wy, wz := qw*x2, qw*y2, qw*z2

	return [16]float32{
		1 - (yy + zz), xy + wz, xz - wy, 0,
		xy - wz, 1 - (xx + zz), yz + wx, 0,
		xz + wy, yz - wx, 1 - (xx + yy), 0,
		trans[0], trans[1], trans[2], 1,
	}
}

// inverseBindMatrix returns bone's inverse bind matrix: the algebraic inverse of the same
// bind-pose transform gltfBoneNode gives that bone's own node (translation =
// quatRotateVec(bone.LocalRot, bone.LocalPos), rotation = bone.LocalRot). Working through
// asura.Skeleton.Skin's `rotate(rot, local + pos)` formula algebraically, that inverse always
// reduces to "rotate by the bone's conjugate rotation, then translate by -bone.LocalPos" — no
// per-bone case analysis needed. This is what makes glTF's standard skinning formula
// (jointGlobalTransform * inverseBindMatrix * vertex) reproduce Skin's already-applied vertex
// positions unchanged at bind pose/rest: see TestInverseBindMatrixCancelsAtBindPose.
func inverseBindMatrix(b asura.Bone) [16]float32 {
	conj := [4]float32{b.LocalRot[0], -b.LocalRot[1], -b.LocalRot[2], -b.LocalRot[3]}
	return composeMat4(conj, [3]float32{-b.LocalPos[0], -b.LocalPos[1], -b.LocalPos[2]})
}

// quatRotateVec rotates v by quaternion q (w, x, y, z form) — the same formula as
// pkg/asura/skeleton.go's unexported helper of the same name, duplicated here rather than
// exported from pkg/asura because this file works entirely in glTF node-transform terms
// (computing where a *bone node* sits), not Asura container decoding, for a single call site
// outside that package.
func quatRotateVec(q [4]float32, v [3]float32) [3]float32 {
	qw, qx, qy, qz := q[0], q[1], q[2], q[3]
	uvx := qy*v[2] - qz*v[1]
	uvy := qz*v[0] - qx*v[2]
	uvz := qx*v[1] - qy*v[0]
	uuvx := qy*uvz - qz*uvy
	uuvy := qz*uvx - qx*uvz
	uuvz := qx*uvy - qy*uvx
	return [3]float32{
		v[0] + 2*(qw*uvx+uuvx),
		v[1] + 2*(qw*uvy+uuvy),
		v[2] + 2*(qw*uvz+uuvz),
	}
}

func boneNodeName(b asura.Bone, index int) string {
	if b.Name != "" {
		return b.Name
	}
	return fmt.Sprintf("bone_%d", index)
}

// writeGLB writes a mesh as a binary glTF 2.0 (.glb) file — the default mesh output format (see
// writeOBJ for the plain, unrigged alternative). Position uses the same axis convention as
// writeOBJ (see objAxes — the identity, confirmed by user testing in Blender), since Blender's
// glTF importer converts glTF's mandated Y-up convention into Blender's native Z-up axes with
// the exact same rotation its default OBJ import axis settings (Y up, -Z forward) already apply
// — so whatever raw axis values make writeOBJ come out correct make this come out correct too.
// Triangle winding is flipped the same way as writeOBJ, for the same reason (see writeOBJ).
//
// When m.Skeleton is nil, this writes a single static mesh node — no different in spirit from
// writeOBJ's ungrouped output. When it's set, this instead builds a real, Blender-importable
// armature: one flat (non-nested — see inverseBindMatrix's doc comment on why nesting bones
// under each other here would reintroduce the parent-composition bug asura.Skeleton.Skin's own
// doc comment describes) joint node per bone, and a single mesh bound to all of them via
// GPU-style vertex skinning (JOINTS_0/WEIGHTS_0, plus JOINTS_1/WEIGHTS_1 if any vertex uses more
// than 4 of MeshVertex's 8 possible bone influences) rather than one node per rigid part. This
// reproduces the already-applied Skin() vertex positions unchanged at rest and lets each bone be
// independently selected and posed in Blender — a real armature for rigging, not just corrected
// static geometry. Bones don't inherit each other's posing (Bone.ParentIndex isn't used to nest
// joint nodes, matching Skin's own formula) — a known, documented limitation, not an oversight.
func writeGLB(w io.Writer, m asura.Mesh) error {
	if len(m.Vertices) == 0 {
		return fmt.Errorf("asura: mesh %q has no vertices", m.Path)
	}

	bin := &gltfBinBuilder{}
	doc := gltfDocument{
		Asset:  gltfAsset{Version: "2.0", Generator: "asr-text-extractor"},
		Scenes: []gltfScene{{}},
	}

	var posData, uvData []byte
	minPos, maxPos := m.Vertices[0].Position, m.Vertices[0].Position
	for _, v := range m.Vertices {
		p := objAxes(v.Position)
		posData = appendF32(posData, p[0])
		posData = appendF32(posData, p[1])
		posData = appendF32(posData, p[2])
		uvData = appendF32(uvData, v.UV0[0])
		uvData = appendF32(uvData, v.UV0[1])
		for i := 0; i < 3; i++ {
			if v.Position[i] < minPos[i] {
				minPos[i] = v.Position[i]
			}
			if v.Position[i] > maxPos[i] {
				maxPos[i] = v.Position[i]
			}
		}
	}
	minPos, maxPos = objAxes(minPos), objAxes(maxPos)
	for i := 0; i < 3; i++ {
		if minPos[i] > maxPos[i] {
			minPos[i], maxPos[i] = maxPos[i], minPos[i]
		}
	}

	posAcc := len(doc.Accessors)
	doc.Accessors = append(doc.Accessors, gltfAccessor{
		BufferView: bin.addView(posData, gltfTargetArrayBuffer), ComponentType: gltfComponentFloat,
		Count: len(m.Vertices), Type: "VEC3",
		Min: vec3ToFloat64(minPos), Max: vec3ToFloat64(maxPos),
	})
	uvAcc := len(doc.Accessors)
	doc.Accessors = append(doc.Accessors, gltfAccessor{
		BufferView: bin.addView(uvData, gltfTargetArrayBuffer), ComponentType: gltfComponentFloat,
		Count: len(m.Vertices), Type: "VEC2",
	})
	attrs := map[string]int{"POSITION": posAcc, "TEXCOORD_0": uvAcc}

	if m.Skeleton != nil {
		useSecondary := false
		for _, v := range m.Vertices {
			for k := 4; k < 8; k++ {
				if v.BoneWeights[k] != 0 {
					useSecondary = true
				}
			}
		}

		var joints0, weights0, joints1, weights1 []byte
		for _, v := range m.Vertices {
			joints0 = append(joints0, v.BoneIDs[0], v.BoneIDs[1], v.BoneIDs[2], v.BoneIDs[3])
			weights0 = append(weights0, v.BoneWeights[0], v.BoneWeights[1], v.BoneWeights[2], v.BoneWeights[3])
			if useSecondary {
				joints1 = append(joints1, v.BoneIDs[4], v.BoneIDs[5], v.BoneIDs[6], v.BoneIDs[7])
				weights1 = append(weights1, v.BoneWeights[4], v.BoneWeights[5], v.BoneWeights[6], v.BoneWeights[7])
			}
		}

		j0Acc := len(doc.Accessors)
		doc.Accessors = append(doc.Accessors, gltfAccessor{
			BufferView: bin.addView(joints0, gltfTargetArrayBuffer), ComponentType: gltfComponentUByte,
			Count: len(m.Vertices), Type: "VEC4",
		})
		w0Acc := len(doc.Accessors)
		doc.Accessors = append(doc.Accessors, gltfAccessor{
			BufferView: bin.addView(weights0, gltfTargetArrayBuffer), ComponentType: gltfComponentUByte,
			Count: len(m.Vertices), Type: "VEC4", Normalized: true,
		})
		attrs["JOINTS_0"], attrs["WEIGHTS_0"] = j0Acc, w0Acc

		if useSecondary {
			j1Acc := len(doc.Accessors)
			doc.Accessors = append(doc.Accessors, gltfAccessor{
				BufferView: bin.addView(joints1, gltfTargetArrayBuffer), ComponentType: gltfComponentUByte,
				Count: len(m.Vertices), Type: "VEC4",
			})
			w1Acc := len(doc.Accessors)
			doc.Accessors = append(doc.Accessors, gltfAccessor{
				BufferView: bin.addView(weights1, gltfTargetArrayBuffer), ComponentType: gltfComponentUByte,
				Count: len(m.Vertices), Type: "VEC4", Normalized: true,
			})
			attrs["JOINTS_1"], attrs["WEIGHTS_1"] = j1Acc, w1Acc
		}
	}

	var idxData []byte
	for _, t := range m.Triangles {
		a, b, c := flipWinding(t)
		idxData = appendU16(idxData, a)
		idxData = appendU16(idxData, b)
		idxData = appendU16(idxData, c)
	}
	idxAcc := len(doc.Accessors)
	doc.Accessors = append(doc.Accessors, gltfAccessor{
		BufferView: bin.addView(idxData, gltfTargetElementArrayBuffer), ComponentType: gltfComponentUShort,
		Count: len(m.Triangles) * 3, Type: "SCALAR",
	})

	doc.Meshes = []gltfMesh{{
		Name:       m.Path,
		Primitives: []gltfPrimitive{{Attributes: attrs, Indices: idxAcc}},
	}}

	if m.Skeleton == nil {
		doc.Nodes = []gltfNode{{Name: m.Path, Mesh: intPtr(0)}}
		doc.Scenes[0].Nodes = []int{0}
		return writeGLBContainer(w, doc, bin)
	}

	joints := make([]int, len(m.Skeleton.Bones))
	boneNodes := make([]gltfNode, len(m.Skeleton.Bones))
	var invBindData []byte
	for i, b := range m.Skeleton.Bones {
		joints[i] = 1 + i // node 0 is the armature root
		boneNodes[i] = gltfNode{
			Name:        boneNodeName(b, i),
			Translation: vec3ToFloat64(quatRotateVec(b.LocalRot, b.LocalPos)),
			Rotation:    quatToGLTF(b.LocalRot),
		}
		invBindData = appendMat4(invBindData, inverseBindMatrix(b))
	}
	invBindAcc := len(doc.Accessors)
	doc.Accessors = append(doc.Accessors, gltfAccessor{
		BufferView: bin.addView(invBindData, 0), ComponentType: gltfComponentFloat,
		Count: len(m.Skeleton.Bones), Type: "MAT4",
	})

	doc.Skins = []gltfSkin{{
		Name: m.Skeleton.Name, InverseBindMatrices: invBindAcc, Skeleton: 0, Joints: joints,
	}}

	meshNodeIdx := 1 + len(joints)
	doc.Nodes = append([]gltfNode{{Name: m.Skeleton.Name, Children: joints}}, boneNodes...)
	doc.Nodes = append(doc.Nodes, gltfNode{Name: m.Path, Mesh: intPtr(0), Skin: intPtr(0)})
	doc.Scenes[0].Nodes = []int{0, meshNodeIdx}

	return writeGLBContainer(w, doc, bin)
}

// writeGLBContainer wraps doc/bin in the .glb binary container: a 12-byte header (magic
// "glTF", version 2, total byte length), a JSON chunk (space-padded to 4 bytes), and a BIN
// chunk (zero-padded to 4 bytes) — see the glTF 2.0 spec, appendix C.
func writeGLBContainer(w io.Writer, doc gltfDocument, bin *gltfBinBuilder) error {
	doc.BufferViews = bin.views
	doc.Buffers = []gltfBuffer{{ByteLength: bin.buf.Len()}}

	jsonData, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	binData := bin.buf.Bytes()
	for len(binData)%4 != 0 {
		binData = append(binData, 0)
	}

	var u32 [4]byte
	writeU32 := func(out *bytes.Buffer, v uint32) {
		binary.LittleEndian.PutUint32(u32[:], v)
		out.Write(u32[:])
	}

	var out bytes.Buffer
	out.WriteString("glTF")
	writeU32(&out, 2)
	writeU32(&out, uint32(12+8+len(jsonData)+8+len(binData)))

	writeU32(&out, uint32(len(jsonData)))
	out.WriteString("JSON")
	out.Write(jsonData)

	writeU32(&out, uint32(len(binData)))
	out.WriteString("BIN\x00")
	out.Write(binData)

	_, err = w.Write(out.Bytes())
	return err
}

// flipWinding returns t's three vertex indices reordered so triangles come out front-facing in
// consumers that expect standard (counter-clockwise) winding — see writeOBJ's doc comment for
// how this was derived from real user feedback in Blender. Shared by writeOBJ and writeGLB so
// the reasoning only needs to live in one place.
func flipWinding(t [3]uint16) (a, b, c uint16) {
	return t[0], t[2], t[1]
}
