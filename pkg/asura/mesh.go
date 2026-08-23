package asura

import (
	"encoding/binary"
	"fmt"
	"math"
)

// meshVertexStride is the fixed byte size of each vertex record in a mesh's vertex buffer.
const meshVertexStride = 48

// MeshGroup is one material/sub-mesh grouping within a Mesh's shared triangle list: the first
// IndexCount triangle-indices of Mesh.Triangles belong to the first group, the next group's own
// IndexCount to the second, and so on. Hash is a source-material identifier (its hash algorithm
// isn't known, so it can't be turned back into a name) rather than the actual material data.
type MeshGroup struct {
	Hash       uint32
	IndexCount int
}

// MeshVertex is one dequantized vertex of a decoded Mesh: a local-space position (already
// converted from its on-disk quantized form), its two UV channels, and up to 8 bone
// influences (BoneIDs[i] weighted by BoneWeights[i]/255, zero-weight entries meaning "unused"
// — see Skeleton.Skin).
type MeshVertex struct {
	Position    [3]float32
	UV0         [2]float32
	UV1         [2]float32
	BoneIDs     [8]uint8
	BoneWeights [8]uint8
}

// Mesh is a decoded 3D object mesh from an RSCF resource-type-0 entry's payload (see
// rscfResourceTypeMesh) — a single object's render geometry: a flat vertex buffer, one or more
// material Groups, and a triangle-list index buffer shared across all of them.
//
// The on-disk format was not reverse-engineered from this project's own sample data — it was
// recovered from a dedicated, independently-authored Zombie Army 4 reverse-engineering project
// (community_scripts/zombie_army_4_findings-master/, specifically
// ZombieArmy4Loader/model.py and the bts/model.bt binary template), which already had this
// format fully solved with a working Blender importer. An earlier version of this decoder,
// based on a format hypothesis ported from a different Rebellion game (Evil Genius 2) and only
// validated by total-byte-count reconciliation (never against real decoded geometry — this
// project has no way to visually inspect a 3D model), got the header size and vertex layout
// wrong in a way that byte-counted correctly by coincidence but produced garbage positions;
// this version is a direct, field-for-field port of the known-working reference instead.
type Mesh struct {
	Path      string
	Vertices  []MeshVertex
	Groups    []MeshGroup
	Triangles [][3]uint16
}

// ParseMesh decodes a single mesh payload (an RSCF entry's payload whose resource-type code is
// rscfResourceTypeMesh).
//
// Layout: a header of 5 uint32 fields (group count, vertex count, total index count, total
// triangle/"polygon" count, and one field of unconfirmed meaning), then one 24-byte record per
// group (a material hash, 4 fields of unconfirmed meaning, and that group's own index count —
// see MeshGroup), then a 3-float32 position dequantization scale and a 3-float32 offset (so
// the header's total size is 44 + 24*groupCount, not fixed — the earlier hypothesis's fixed
// 80-byte header only ever accidentally matched a real file when groupCount was exactly 1.5,
// which never happens; see the Mesh doc comment), then the vertex buffer, then the index
// buffer (indexCount uint16 values, indexCount/3 triangles).
//
// Each vertex is a fixed 48-byte record: a quantized position (3 uint16) at offset 0, an int16
// sentinel (always -1 in every real sample, meaning unconfirmed) at offset 6, quantized
// normals (3 int16) at offset 8, 3 more int16 fields of unconfirmed meaning at offset 14, 2
// uint16 fields of unconfirmed meaning at offset 20, two UV channels (2 float16 each) at
// offsets 24 and 28, 8 bytes of bone skin weights at offset 32, and 8 bytes of bone indices at
// offset 40. Normals aren't decoded; position, both UV channels, and the bone weights/indices
// are (see Skeleton.Skin for how the latter two combine with a matching HSKN chunk to
// correctly position rigid sub-parts — e.g. a rifle bolt — relative to the rest of a Mesh).
// Position dequantizes as `(raw/32767) * (scale/2) + offset` per axis (raw read as an unsigned
// 16-bit value, per the reference implementation, despite the asymmetric-looking /32767
// divisor).
func ParseMesh(path string, payload []byte) (*Mesh, error) {
	r := &reader{data: payload}
	groupCount := r.u32()
	vertCount := r.u32()
	indexCount := r.u32()
	r.u32() // "polygon count" — redundant with indexCount/3 in every real sample checked
	r.u32() // meaning not understood
	if r.err != nil {
		return nil, fmt.Errorf("asura: mesh %q: truncated header", path)
	}
	// Each group record is 24 bytes: bail out before allocating if groupCount is bogus
	// (corrupt data, or this payload isn't a mesh at all) rather than trusting it blindly.
	if uint64(groupCount) > uint64(r.remaining())/24 {
		return nil, fmt.Errorf("asura: mesh %q: group count %d exceeds remaining payload", path, groupCount)
	}

	groups := make([]MeshGroup, groupCount)
	for i := range groups {
		hash := r.u32()
		r.bytes(4) // meaning not understood
		idxCount := r.u32()
		r.bytes(4 * 3) // 3 more fields, meaning not understood
		groups[i] = MeshGroup{Hash: hash, IndexCount: int(idxCount)}
	}

	var scale, offset [3]float32
	for i := range scale {
		scale[i] = math.Float32frombits(r.u32())
	}
	for i := range offset {
		offset[i] = math.Float32frombits(r.u32())
	}
	if r.err != nil {
		return nil, fmt.Errorf("asura: mesh %q: truncated header", path)
	}

	want := r.pos + int(vertCount)*meshVertexStride + int(indexCount)*2
	if want != len(payload) {
		return nil, fmt.Errorf("asura: mesh %q: payload size %d doesn't match predicted %d bytes (groups=%d verts=%d indices=%d)",
			path, len(payload), want, groupCount, vertCount, indexCount)
	}
	if indexCount%3 != 0 {
		return nil, fmt.Errorf("asura: mesh %q: index count %d isn't a multiple of 3", path, indexCount)
	}

	vertices := make([]MeshVertex, vertCount)
	vertStart := r.pos
	for i := range vertices {
		off := vertStart + i*meshVertexStride
		v := payload[off : off+meshVertexStride]
		var pos [3]float32
		for axis := 0; axis < 3; axis++ {
			raw := binary.LittleEndian.Uint16(v[axis*2 : axis*2+2])
			pos[axis] = float32(raw)/32767*(scale[axis]/2) + offset[axis]
		}
		mv := MeshVertex{
			Position: pos,
			UV0: [2]float32{
				float16ToFloat32(binary.LittleEndian.Uint16(v[24:26])),
				float16ToFloat32(binary.LittleEndian.Uint16(v[26:28])),
			},
			UV1: [2]float32{
				float16ToFloat32(binary.LittleEndian.Uint16(v[28:30])),
				float16ToFloat32(binary.LittleEndian.Uint16(v[30:32])),
			},
		}
		copy(mv.BoneWeights[:], v[32:40])
		copy(mv.BoneIDs[:], v[40:48])
		vertices[i] = mv
	}

	triStart := vertStart + int(vertCount)*meshVertexStride
	triangles := make([][3]uint16, indexCount/3)
	for i := range triangles {
		off := triStart + i*6
		triangles[i] = [3]uint16{
			binary.LittleEndian.Uint16(payload[off : off+2]),
			binary.LittleEndian.Uint16(payload[off+2 : off+4]),
			binary.LittleEndian.Uint16(payload[off+4 : off+6]),
		}
	}

	return &Mesh{Path: path, Vertices: vertices, Groups: groups, Triangles: triangles}, nil
}

// asMesh interprets the entry as a mesh: nil unless its resource-type code is
// rscfResourceTypeMesh and ParseMesh accepts its payload (some resource-type-0 entries are a
// different, unrelated kind of resource — see rscfResourceTypeMesh's doc comment — and
// ParseMesh's own size-reconciliation check is what rules those out).
func (e *rscfEntry) asMesh() *Mesh {
	if e.resType != rscfResourceTypeMesh {
		return nil
	}
	m, err := ParseMesh(e.path, e.payload)
	if err != nil {
		return nil
	}
	return m
}

// float16ToFloat32 converts an IEEE 754 binary16 (half-precision) value to float32.
func float16ToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1F
	frac := uint32(h & 0x3FF)

	var bits uint32
	switch {
	case exp == 0 && frac == 0:
		bits = sign
	case exp == 0:
		// Subnormal half -> normal float32.
		e := uint32(127 - 15 + 1)
		for frac&0x400 == 0 {
			frac <<= 1
			e--
		}
		frac &= 0x3FF
		bits = sign | (e << 23) | (frac << 13)
	case exp == 0x1F:
		bits = sign | 0x7F800000 | (frac << 13) // Inf/NaN.
	default:
		bits = sign | ((exp - 15 + 127) << 23) | (frac << 13)
	}
	return math.Float32frombits(bits)
}
