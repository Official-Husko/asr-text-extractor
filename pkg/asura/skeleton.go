package asura

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Bone is one joint in a Skeleton: its bind-pose local position/rotation relative to its
// parent, and that parent's index within the same Skeleton's Bones slice. Bones are stored in
// topological order (a bone's parent always has a lower index than the bone itself) — bone 0
// is always the root; its ParentIndex is meaningless (conventionally equal to 0 in real
// samples, but never consulted).
type Bone struct {
	Name        string
	ParentIndex int
	LocalPos    [3]float32
	LocalRot    [4]float32 // quaternion (w, x, y, z) — see ParseSkeleton
}

// Skeleton is a decoded HSKN chunk: a named, flat bone hierarchy used to correctly position a
// Mesh's rigid sub-parts (see MeshVertex.BoneIDs/BoneWeights and Skin) relative to each other
// in the mesh's bind pose.
//
// This exists because some meshes' decoded vertex positions alone aren't the final, correctly
// assembled geometry — a real sample (Zombie Army 4's "carcano" rifle) decodes into a
// recognizable body and a recognizable bolt/bolt-handle/firing-pin/trigger, but those smaller
// parts sit slightly offset from where they belong. The HSKN chunk named "Carcano" that goes
// with it has exactly 5 bones — matching the mesh's 5 distinct BoneIDs value exactly — named
// Body/Bolt/Bolt_Handle/Firing_Pin/Trigger, all parented to Body, with small (centimeter-scale)
// position-only bind-pose offsets matching the direction and rough magnitude each part needed
// to move. Skin applies exactly that correction.
type Skeleton struct {
	Name  string
	Bones []Bone
}

// ParseSkeleton decodes an HSKN section (data must start with the "HSKN" tag) far enough to
// recover the bone hierarchy and bind-pose transforms — everything Skin needs. Bone names are
// best-effort: each is followed by a per-bone, length-prefixed blob of unconfirmed purpose (up
// to ~90KB in a real sample) that isn't needed for skinning and this parser doesn't attempt to
// interpret; if reading a name fails partway through, the remaining bones are simply left
// unnamed rather than aborting the whole skeleton. Nothing after the bone name records is read
// at all — the caller (package.go) advances past the whole section using its own declared
// length regardless of how far this function actually parsed.
//
// Layout, ported from a dedicated, independently-authored Zombie Army 4 reverse-engineering
// project (zombie_army_4_findings-master/ZombieArmy4Loader/chunks/hskn.py, class HSKN) rather
// than reverse-engineered from scratch: a ChunkHeader (tag, size, version, flags), an unused
// uint32, a bone count, a padded name string, then — only if the unused uint32 is non-zero AND
// bit 6 of flags is clear — a fixed skip (144 bytes for version >= 25, else 72; not exercised
// by any real sample this was validated against, where the unused field is always 0) — then
// boneCount parent-bone indices (uint32 each), then boneCount {position: 3 float32, rotation:
// 4 float32} bind-pose records. Validated against a real sample's "Carcano" HSKN chunk
// (version 29; the file's 295 HSKN chunks span exactly 4 distinct (version, flags)
// combinations, all version 29, differing only in flag bits this function doesn't need to
// branch on for the fields it reads).
func ParseSkeleton(data []byte) (*Skeleton, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("asura: HSKN section too short")
	}
	r := &reader{data: data, pos: 4} // skip the "HSKN" tag
	r.u32()                          // section size — the caller already knows this
	version := r.u32()
	flags := r.u32()
	unk := r.u32()
	boneCount := r.u32()
	if r.err != nil {
		return nil, fmt.Errorf("asura: HSKN: truncated header")
	}

	name, next, ok := alignedString(data, r.pos)
	if !ok {
		return nil, fmt.Errorf("asura: HSKN: unterminated name")
	}
	r.pos = next

	if unk != 0 && flags&0x40 == 0 {
		if version >= 25 {
			r.bytes(144)
		} else {
			r.bytes(72)
		}
	}

	// Each bone needs a parent index (4 bytes) plus a pos+rot record (28 bytes) = 32 bytes
	// minimum: bail out before allocating if boneCount is bogus rather than trusting it blindly.
	if uint64(boneCount) > uint64(r.remaining())/32 {
		return nil, fmt.Errorf("asura: HSKN %q: bone count %d exceeds remaining section", name, boneCount)
	}

	parents := make([]uint32, boneCount)
	for i := range parents {
		parents[i] = r.u32()
	}
	bones := make([]Bone, boneCount)
	for i := range bones {
		var pos [3]float32
		var rot [4]float32
		for j := range pos {
			pos[j] = math.Float32frombits(r.u32())
		}
		for j := range rot {
			rot[j] = math.Float32frombits(r.u32())
		}
		bones[i] = Bone{ParentIndex: int(parents[i]), LocalPos: pos, LocalRot: rot}
	}
	if r.err != nil {
		return nil, fmt.Errorf("asura: HSKN %q: truncated bone transform data", name)
	}

	// Best-effort bone names; see the doc comment above.
	pos := r.pos
	if version >= 10 {
		pos++ // a single byte between the transform records and the first bone name
	}
	for i := range bones {
		if pos < 0 || pos >= len(data) {
			break
		}
		bname, nameEnd, ok := alignedString(data, pos)
		if !ok {
			break
		}
		pos = nameEnd
		if version >= 10 {
			if pos >= len(data) {
				break
			}
			pos++ // a per-bone byte of unconfirmed meaning
		}
		if pos+4 > len(data) {
			break
		}
		dataLen := int(binary.LittleEndian.Uint32(data[pos : pos+4]))
		pos += 4
		if dataLen < 0 || pos+dataLen > len(data) {
			break
		}
		pos += dataLen
		bones[i].Name = bname
	}

	return &Skeleton{Name: name, Bones: bones}, nil
}

// worldTransform is a bone's bind-pose transform composed all the way to the root.
type worldTransform struct {
	pos [3]float32
	rot [4]float32
}

func (s *Skeleton) worldTransforms() []worldTransform {
	out := make([]worldTransform, len(s.Bones))
	for i, b := range s.Bones {
		if i == 0 || b.ParentIndex == i || b.ParentIndex < 0 || b.ParentIndex >= i {
			out[i] = worldTransform{pos: b.LocalPos, rot: b.LocalRot}
			continue
		}
		parent := out[b.ParentIndex]
		out[i] = worldTransform{
			pos: addVec3(parent.pos, quatRotateVec(parent.rot, b.LocalPos)),
			rot: quatMul(parent.rot, b.LocalRot),
		}
	}
	return out
}

// Skin returns a copy of mesh's vertex positions repositioned according to the skeleton's
// bind-pose bone transforms and each vertex's bone weights (see MeshVertex.BoneIDs/
// BoneWeights): a standard linear-blend skin, `sum(weight_i * (bone_i.rotation * localPos +
// bone_i.position)) / sum(weight_i)` over each vertex's up-to-8 non-zero-weight influences.
// Vertices with no bone influence (all-zero weights, or every referenced bone ID out of range)
// are returned unchanged, so calling this on a mesh that isn't actually rigged to skeleton is
// harmless.
func (s *Skeleton) Skin(mesh *Mesh) [][3]float32 {
	transforms := s.worldTransforms()
	out := make([][3]float32, len(mesh.Vertices))
	for i, v := range mesh.Vertices {
		var totalWeight float32
		var acc [3]float32
		for j := 0; j < 8; j++ {
			w := v.BoneWeights[j]
			if w == 0 {
				continue
			}
			boneID := int(v.BoneIDs[j])
			if boneID < 0 || boneID >= len(transforms) {
				continue
			}
			wf := float32(w) / 255
			t := transforms[boneID]
			p := addVec3(t.pos, quatRotateVec(t.rot, v.Position))
			acc = addVec3(acc, scaleVec3(p, wf))
			totalWeight += wf
		}
		if totalWeight == 0 {
			out[i] = v.Position
			continue
		}
		out[i] = scaleVec3(acc, 1/totalWeight)
	}
	return out
}

func addVec3(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] + b[0], a[1] + b[1], a[2] + b[2]}
}

func scaleVec3(v [3]float32, s float32) [3]float32 {
	return [3]float32{v[0] * s, v[1] * s, v[2] * s}
}

// quatMul multiplies two quaternions in (w, x, y, z) form: a composed with b, applied as
// "rotate by b, then by a".
func quatMul(a, b [4]float32) [4]float32 {
	aw, ax, ay, az := a[0], a[1], a[2], a[3]
	bw, bx, by, bz := b[0], b[1], b[2], b[3]
	return [4]float32{
		aw*bw - ax*bx - ay*by - az*bz,
		aw*bx + ax*bw + ay*bz - az*by,
		aw*by - ax*bz + ay*bw + az*bx,
		aw*bz + ax*by - ay*bx + az*bw,
	}
}

// quatRotateVec rotates v by quaternion q (w, x, y, z form).
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
