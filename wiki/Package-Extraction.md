# Package Extraction

Covers the `package` command: extracting manifest-referenced sub-files and embedded textures
from an `AsuraZbb`-compressed level-package file (`.pc`, `.pc_entdata`). The same command also
works directly on `.gui` files (UI menu/texture archives, `AsuraZbb`-wrapped the same way, just
with an `FNFO` header that has no `RSFL` manifest behind it — see below).

## Background

A level's `.pc`/`.pc_entdata` files are wrapped in a distinct compression format signed
`"AsuraZbb"` (not the plain `"Asura   "` magic every other chunk type in this tool understands).
The wrapper is: the 8-byte magic, an unused `uint32`, a `uint32` giving the total decompressed
size, then repeating `[compressedSize uint32][decompressedSize uint32][zlib-compressed bytes]`
chunks (each 2MB decompressed except the last) until the declared total is reached. Confirmed
byte-for-byte against a real 255MB Zombie Army 4 level package (226 chunks, exact declared
total, exact end-of-file landing). Decompressing yields a normal `"Asura   "`-signed buffer.

That decompressed buffer starts with an `FNFO`-tagged header followed by an `RSFL` manifest —
a flat table of sub-files bundled into the package (`.anim`, `.pfx`, `.snd`, `.nav`, `.cut`,
`.ent`, and more), each recorded as a NUL-terminated path plus a `uint32` offset and `uint32`
size. Two things about this manifest took real sample data to pin down correctly:

- **Entry offsets are relative to the end of the `RSFL` section itself**, not the start of the
  package's decompressed content — confirmed by real entries only decoding cleanly (a small
  count field followed by the sub-file's own name repeated verbatim) once that base is added.
- **The padding between a path's NUL terminator and its offset field pads to a fixed 4-byte
  file-absolute alignment**, not "skip every zero byte present." An earlier implementation did
  the latter and silently mis-parsed 2 of 282 entries in a real sample — both had an offset
  field whose own low byte happened to be zero, which a greedy zero-byte scan mistook for one
  more byte of padding, shifting every field after it (offset, size, and the next entry's own
  path) by one byte. `pkg/asura/package_test.go`'s
  `TestParseRSFLManifestOffsetWithZeroLeadByte` regression-tests this exact scenario.

Between the end of the manifest and the manifest's own referenced sub-files sits a long run of
further tagged sections — geometry/spatial data (`PBRV`, `SDPH`), a small unidentified one
(`IRTX`), and many section types that turn out to be per-level-object records (`CONA` entity
transforms, `SDSM`/`SDEV` spatial data, `HSKN`/`HSKL`/`HSBB`/`HSKE`/`HMPT` skeleton/hitbox data,
`FAAN`/`TXAN` animation references, and more). Every one of these — including `RSFL` and `FNFO`
themselves — shares the same generic framing: a 4-byte uppercase-ASCII tag followed by a
`uint32` giving the section's total length counted from the tag's own start, which is enough to
skip over a section without understanding its contents.

**`RSCF` entries are interleaved one at a time throughout this run**, not packed into their own
contiguous block — confirmed against a real sample where the very first `RSCF`-tagged section
immediately after the manifest turned out to be a single bare resource reference (no embedded
payload, just a self-referential path) followed directly by ~2,200 unrelated `CONA` entity
records, with the real texture entries starting much further in and each one followed by its
own unrelated section rather than by another texture. Extraction therefore walks every section
generically and decodes each one tagged `RSCF` as a possible texture or mesh entry inline (same
per-entry decoder as standalone [`RSCF` texture archives](Texture-Extraction.md) — see that page
for the entry field layout). In a real 473MB decompressed sample, 3,071 `RSCF`-tagged sections
break down by their resource-type field as: 2,502 textures (matching an independent whole-file
search for the `"DDS "` magic exactly), 550 per-object meshes (see Meshes below), 1 bare
package self-reference (0 payload bytes), and 2 further resource-type-0 entries
(`"inst (dynamic)"`/`"inst (static)"`) that are neither — almost certainly a reference into the
separate, much larger `INST` section, not extracted or interpreted.

**Not every `AsuraZbb`-wrapped file has an `FNFO`/`RSFL` manifest at all.** A real DLC sample,
`DLCData/disc_h_hellbase.asr` (a texture-override pack, not a level), decompresses straight into
the *same* run of `RSCF`/`HSKN`/generic-tagged sections described above, but with no manifest
and no sub-files anywhere: four `RSCF` texture entries interleaved with one small, unidentified
`TTXT` section, then a zero footer. This was found via the [`scan`](Scan.md) command flagging it
as a parse failure — and it wasn't a scan-only cosmetic problem: `package unpack` itself failed
on this file too before the fix (0 textures extracted). The manifest is now only parsed if the
file's very first section actually is one; otherwise every section is walked from the very start
of the file, the same way a full package's post-manifest run always was. `Package.Entries` is
simply empty for a file like this, since there's no manifest to populate it from.

**A third variant has an `FNFO` section, but it's never followed by an `RSFL` manifest either** —
found via the same [`scan`](Scan.md) whole-install survey, this time flagging every real
`GUIMenu/*.gui` file and `Chars/mp.pc` with `"expected RSFL manifest at offset 32"`. In both real
samples, the section immediately after `FNFO` is already `RSCF` (or, for `mp.pc`, just the zero
footer) — never `RSFL`. `FNFO`'s own 16-byte body in this variant is `{1, 0, totalLen-4, 8}`
(confirmed identical in shape across two very different real samples: a 36-byte `mp.pc` and a
20MB `frontend.gui`, with the third field exactly matching each file's own total decompressed
length minus its trailing 4-byte zero footer both times) — a self-describing "this whole file,
no manifest" header rather than a real entry table. The fix mirrors the no-manifest-at-all case:
before committing to parsing an `RSFL` manifest, the parser now peeks at whether an actual
`"RSFL"` tag is really there; if not, `FNFO` is left for the generic tagged-section walk to skip
over. Before this fix, `frontend.gui` — a real 20MB UI texture archive (icons/atlases) — was
completely unextractable; it now yields 40 real textures. `mp.pc` decompresses to just the bare
36-byte `FNFO` stub with nothing else in it at all (a placeholder file, not a bug — `Entries: 0
Textures: 0 Meshes: 0 Audio: 0` is the correct result for it).

The internal layout of `PBRV` (a *separate* geometry/spatial-data block, several megabytes in a
typical level — not the same thing as the RSCF mesh entries described below) has not been
reverse-engineered and isn't parsed; it's only skipped over via its declared length, same as
every other unidentified section.

## Meshes

Some `RSCF` entries (resource-type 0) are per-object render meshes rather than textures: a
header, one or more material groups, a vertex buffer, and a shared triangle-index buffer. This
format was **not** reverse-engineered from this project's own sample data alone — it matches a
known-working reference implementation of the format field-for-field. (An earlier version of
this tool's mesh support was instead based on a format hypothesis carried over from a
*different* Rebellion game — that version's exported models turned out to be garbled nonsense
once actually opened in Blender, not the "close enough" the numbers alone had suggested; see
`pkg/asura/mesh.go`'s doc comments for what went wrong. That version is gone from the code now,
but its lesson stands: a format hypothesis validated only by total-byte-count reconciliation,
with no way to visually inspect the result, is not validated.)

The header is `44 + 24*groupCount` bytes: 5 `uint32` fields (group count, vertex count, total
index count, a redundant triangle count, and one unconfirmed field), then one 24-byte record
per group (a material hash plus that group's own index count), then a 3-float position
dequantization scale and a trailing offset. Per vertex (48-byte stride): a quantized position (3
× `uint16` at offset 0, dequantized per axis as `raw/32767 * (scale/2) + offset`), two UV
channels (2 × half-float each) at offsets 24 and 28, and up to 8 bone weight/index pairs used
for skinning (see below). Normals aren't decoded.

**The trailing offset is 3 floats (X, Y, Z) in Zombie Army 4, but only 2 (X, Y — Z has no
stored offset) in Sniper Elite 5 and Sniper Elite Resistance** — a real, confirmed
engine-revision difference (see [games/Sniper-Elite-5.md](games/Sniper-Elite-5.md#mesh-decoding-the-fix)
for the full byte-level derivation), not a guess. `ParseMesh` tries the 3-float layout first and
falls back to the 2-float one only if the first doesn't reconcile the payload's own declared
size exactly — the same size-reconciliation check that already distinguishes a real mesh entry
from an unrelated resource-type-0 blob (see below) is what safely picks the right layout, with
no separate per-game flag needed.

All group counts are supported — real samples have 1, 2, 4, and 10. Validated by checking that
decoded triangle edge lengths are small relative to each mesh's own bounding box (i.e.
triangles connect nearby vertices, the signature of a coherent mesh rather than scrambled
data) and that bounding-box sizes are physically plausible across a huge range of named
objects, from a 6cm light bulb to an 8m fractured structure piece — and, since then, by an
actual user check in Blender: a real multi-part mesh (Zombie Army 4's "carcano" rifle) came out
with a recognizable body and recognizable bolt/bolt-handle/firing-pin/trigger sub-parts, just
not quite correctly positioned relative to the body. That confirmed the core geometry decode is
right and pointed at a real gap — see Skinning below for the fix.

### Skinning multi-part meshes

Some meshes (like a rifle's bolt assembly) decode as multiple loosely-connected sub-parts
rather than one solid shape, and each vertex's raw decoded position isn't quite the final,
correctly assembled one — it needs a small correction from a matching skeleton's bind pose.
This tool decodes `HSKN` chunks (also found among the same tagged sections `RSCF` entries live
in) as a named bone hierarchy, matches each mesh to a same-named skeleton (case-insensitively,
and ignoring an `l1#`/`l2#`/... LOD prefix — all LOD variants of a rigged mesh share one
skeleton), and applies standard linear-blend skinning using each vertex's bone weights before
the mesh is handed back. A real sample confirms the shape of the fix exactly: the "carcano"
mesh's 5 distinct bone IDs match an HSKN chunk named "Carcano" with exactly 5 bones — Body,
Bolt, Bolt_Handle, Firing_Pin, Trigger — all parented to Body, sharing Body's own rotation (a
genuine 180-degree turn, not identity — despite `(w,x,y,z)=(0,0,0,1)` reading like a "default"
value at a glance) plus small (centimeter-scale) position-only offsets per bone.

Each vertex's final position is `rotate(bone.rotation, vertexPosition + bone.offset)` — adding
the bone's own small offset to the raw vertex position *before* rotating the combined result,
rather than the more textbook "rotate the vertex, translate separately" — and `Bone.ParentIndex`
isn't consulted by this formula at all, unlike a general skeletal-animation system that would
compose a child bone's transform through its parent's. Both of those choices were reverse
engineered from a real render, not assumed upfront, across three attempts: composing through
the parent hierarchy (textbook-correct in general) canceled the root's real 180-degree rotation
into a net identity for child bones while still flipping their translation's sign, so the main
body looked right but its rigid sub-parts (a rifle's bolt, trigger, etc.) didn't; dropping
hierarchy composition fixed each part's *orientation* but put it in the wrong *place* relative
to the body; only rotating the combined vertex-plus-offset together got both right at once. None
of this project's own numeric checks (bounding-box size, triangle edge length) could have caught
either intermediate wrong state — only an actual user visual check in Blender could, and did.

Meshes with no matching skeleton, or whose vertices carry no bone weight at all, are returned
unchanged — this is automatic and free for meshes that don't need it.

`HSKN`'s own on-disk layout has several fields gated by a version number and a flags bitfield
that this tool doesn't fully decode (only what's needed to reach the bind-pose transform data);
per-bone names are parsed best-effort past that point but aren't required for skinning to work.

### Exporting a real, riggable armature (glTF, the default)

A `Mesh` with a matched `Skeleton` isn't just repositioned — the whole `Skeleton` (not just
derived bone names) travels with it (`Mesh.Skeleton`), so a caller can emit an actual
Blender-importable armature rather than a single static, already-skinned mesh. `package unpack`
does exactly this by default: for a skinned mesh, the exported `.glb` contains one glTF joint
node per bone (translation/rotation set from that bone's bind-pose transform) and a single mesh
bound to all of them via real GPU-style vertex skinning (`JOINTS_0`/`WEIGHTS_0`, using the same
bone weights `Skeleton.Skin` itself consumes — spilling into a second `JOINTS_1`/`WEIGHTS_1` set
only if some vertex actually uses more than 4 of `MeshVertex`'s 8 possible bone influences).
Opened in Blender, this shows up as a real Armature object with named, individually selectable
and posable bones (Body, Bolt, Bolt_Handle, Firing_Pin, Trigger for "carcano") driving one mesh —
not five separate static objects glued together by export-time grouping.

The joint nodes are deliberately **flat** (all children of one armature-root node, never nested
under each other) rather than mirroring `Bone.ParentIndex` — nesting them would reintroduce the
exact parent-composition bug described above (`Skeleton.Skin`'s formula doesn't compose through
`ParentIndex` either, for the same reason). The practical effect: posing one bone in Blender does
**not** automatically carry its children along the way a conventional hand-authored rig would
(moving "Body" won't drag "Bolt" with it) — each bone poses independently. This is a known,
documented limitation, not an oversight; building true parent-relative local transforms from
this project's shared-frame bone data is possible in principle but wasn't attempted, since the
immediate goal was a correctly-posed *static* bind pose with real, individually posable bones,
not full hierarchical rig authoring. This is also, by design, only ever a static/bind-pose
export — the game's `.anim` files aren't parsed and no animation data is imported; only the rig
itself.

Each joint's inverse bind matrix is derived algebraically from `Skeleton.Skin`'s own formula
(see `inverseBindMatrix`'s doc comment in `internal/cli/gltf.go`) rather than computed
numerically, and is checked against that derivation with a dedicated test
(`TestInverseBindMatrixCancelsAtBindPose` in `internal/cli/gltf_test.go`) that verifies, for
several representative bone transforms including the exact 180-degree-rotation shape the real
skinning bug above had, that composing a joint's bind-pose transform with its own inverse bind
matrix cancels to the identity — the condition a glTF skin needs to render unchanged at rest,
before any posing is applied. This project has no way to visually verify a `.glb` file in
Blender the way the skinning formula itself was checked, so this numeric self-consistency check
stands in for that.

**A note on the `glTF_not_exported` collection with an icosphere in it after importing:** this
is normal, expected Blender behavior, not a defect in the exported file. Blender's glTF importer
generates a small icosphere mesh as a *custom bone shape* for each bone, so bones stay
visible/clickable in the viewport, and parks those shape meshes in a `glTF_not_exported`
collection so they're excluded if the scene is ever re-exported — they're a Blender-only visual
aid, unrelated to the actual rig or skinning data, and deleting that collection is safe (it just
reverts bones to Blender's default display). Confirmed against public Blender/glTF-Blender-IO
bug reports describing the exact same collection/icosphere combination on import for unrelated
files. Blender 4.0.2 specifically had a real bug where these shapes came out disproportionately
oversized (fixed in 4.1); if the icosphere looks huge rather than just present, that's most
likely this, not something to chase in this project's exporter.

### Embedded textures (glTF, the default)

A mesh's `.glb` also gets a real material with its own textures embedded directly in the file
(no external image files to keep track of). This is a heuristic, not a byte-exact link — a
mesh's actual material identifier (`MeshGroup.Hash`) has an unknown hash algorithm (see Meshes
above), and in real Sniper Elite 5/Resistance samples that field is uniformly zero anyway (not
populated at all in those titles) — so there's no way to resolve a mesh to its texture directly,
and name-based matching is what's available instead.

`meshTextures` tries progressively shorter candidate names — the mesh's own base name first,
then that name with one trailing `_word` segment removed at a time (e.g.
`german_heavy_truck_door_right` → `german_heavy_truck_door` → `german_heavy_truck`) — against
two different real texture-organization conventions found across samples from every supported
title:

- **Folder segment**: a texture whose path has the candidate name as an exact folder component
  (e.g. `\graphics\weapons\rifles\carcano\carcano_body_a.tga` for a mesh named "carcano") — the
  original convention this heuristic was built against.
- **Filename stem**: a texture whose own filename, with its role suffix removed, exactly equals
  the candidate — found necessary once Sniper Elite 5/Resistance samples were surveyed, where
  many unrelated objects' maps are lumped into one generically-named folder (e.g.
  `graphics\pickups\`) and the specific object is identifiable only from the filename itself
  (e.g. `pickup_crate_explosives_ar.png`).

The progressive trailing-word stripping is what catches the single most common real pattern:
several distinct sub-part meshes of one larger object (e.g. `german_heavy_truck_door_right`,
`german_heavy_truck_grill_left`) sharing one parent-object texture set whose own textures use a
different vocabulary for the specific part (`cab`, `container`, `interior` — never `door` or
`grill`) — an exact per-part match was never going to exist, but the shared parent identifier
does. Matching stops at the first (most specific) candidate that finds anything, so it doesn't
keep stripping and drift onto an unrelated, overly generic identifier once a good match is
already found. Verified against real Sniper Elite 5/Resistance samples: this raised real
texture-match coverage (the fraction of exported `.glb` files that get any embedded material at
all) from under 4% (exact folder match only, the original single-title heuristic) to roughly
40–50% on real level packages from every title tested, including Zombie Army 4 itself — a large
fraction of its own props turned out to follow the same generic-folder-plus-filename convention
the original heuristic never checked for, not just Sniper Elite 5/Resistance's own objects.

Within a matched folder or filename, a texture's own filename suffix decides its role, going by
the naming convention found across real samples: `_a`/`_d`/`_albedo`/`_diff`/`_diffuse` and
`_ar`/`_albedoroughness` (a packed albedo+roughness map, Sniper Elite 5/Resistance's own
dominant color-map suffix — using only its RGB as `baseColorTexture` is safe, since glTF never
reads that texture's alpha as anything else here) both count as a diffuse/albedo map, and
`_n`/`_normal`/`_norm`/`_nm` as a tangent-space normal map. Whichever of those are found get
decoded and re-encoded as PNG (DDS isn't a valid glTF image format) and embedded as the
material's `baseColorTexture` and `normalTexture`. A `_m`-suffixed (metallic) map is deliberately
left unmatched — this project doesn't know whether a "_m" map is plain grayscale metalness or
already packed to glTF's own roughness-in-green/metalness-in-blue `metallicRoughnessTexture`
convention, and guessing wrong would look worse than no metallic/roughness texture at all
(unlike `_ar`, where only ever reading the RGB channel carries no such risk). Every embedded
material instead gets a fixed non-metal, medium-rough default (`metallicFactor: 0`,
`roughnessFactor: 0.6`) rather than glTF's own spec defaults (fully metallic, fully rough with no
texture), which render as a near-black mirror-like blob in Blender — a plain reasonable guess
beats an accurate-looking wrong one here.

When combining LOD/state variants into one file (the default — see below), every variant that
shares a base name shares one embedded material and image set rather than duplicating the same
image bytes once per LOD.

**On resolution**: the embedded texture is whatever this same package file itself carries for
that texture, which is not necessarily the game's true, full-resolution asset — this engine
streams textures, and a level package like `h_hellbase.pc` was found to embed only a small,
always-resident *fallback* copy (128×128 for "carcano_body_a", complete with its own small local
mip chain down to 1×1) while the real, full 2048×2048 texture (12 mip levels) streams at runtime
from a completely separate ~53GB pool of files (`streaming_textures/release*.pc_textures`,
`dlc*.pc_textures`, found elsewhere in a real install) that isn't referenced from inside the
level package at all — matching is only possible by identical filename across the two
independent archives, and finding which of ~20 pool files holds a given texture currently means
grepping through all of them (`grep -qa "<texture-name>" *.pc_textures`, confirmed against a
real sample: `carcano_body_a` turned up in `release5.pc_textures`, not any of the other 21 pool
files checked). The standalone [`texture`](Texture-Extraction.md) command already extracts a
streaming-pool file just as well as any other standalone RSCF archive, so a higher-res version
is always just a `texture unpack` away once you know which pool file has it — this project
doesn't currently do that lookup or substitution automatically.

### OBJ export (`--mesh-format obj`)

Plain Wavefront OBJ export is still available via `--mesh-format obj` (or `both`, to get both
formats) for tools/workflows that don't handle glTF skinning. OBJ has no concept of a skeleton
or vertex groups, so multi-part meshes are instead split into separate parts by bone (e.g. Body,
Bolt, Bolt_Handle) whenever a skeleton was matched — a *different* grouping than the mesh's own
material `Groups` (which "carcano" has only one of, despite having five distinct bones) — so
individual rigid sub-parts can still be selected/hidden/moved independently in Blender, just not
posed as a rig. Each part gets both an `o` (object) and a `g` (group) declaration — `o` is what
actually makes Blender's OBJ importer create separate, independently selectable objects; `g`
alone was tried first and didn't (landed everything in one combined mesh named after the file,
inside a same-named collection — a real result from testing, not a guess) — and triangles are
grouped and reordered so each part's block is fully contiguous in the file (each part name
appears exactly once), not interleaved with others, since that's the form most OBJ importers
handle reliably. Meshes with no matching skeleton export as a single ungrouped mesh.

### Combining LOD and destroyed-state variants (the default)

A real level package's meshes include many `l1#`/`l2#`/.../`l6#`-prefixed LOD (level-of-detail)
variants of the same base object, plus, for destructible props, a separate `<name>_destroyed`
mesh (and that mesh's own LOD variants) — a real sample has a 7-way LOD chain for one chandelier
alone, each variant previously landing in its own separate file (`chandelier_long_base.glb`,
`l1#chandelier_long_base.glb`, ..., `l6#chandelier_long_base.glb`). By default, `package unpack`
instead combines every variant of the same base object into one file: all LOD siblings sharing a
base name (stripping the `l1#`/`l2#`/... prefix), plus a `<base>_destroyed` counterpart when one
exists for that base — folded in exactly like an extra LOD, including *that* mesh's own LOD
variants. A real sample confirms both parts of this: `chandelier_long_base.glb` alone replaces 7
separate files, and `bulb_b.glb` bundles the intact bulb's 4 LOD levels with its
`bulb_b_destroyed` broken-state variant, cutting a real package's total mesh file count from 550
down to 272.

A `<name>_destroyed` mesh with **no** matching `<name>` counterpart in the same package (i.e. a
prop that's only ever seen in its destroyed state) is left as its own untouched, separately-named
file — folding it under a base name that doesn't otherwise exist in the package would misname its
output file for no benefit.

For `.glb` output, each variant keeps its own independent geometry, armature, and skin — variants
aren't merged into one mesh or rigged to a shared skeleton instance, since that would need new,
cross-variant assumptions this project hasn't validated against real data. There's no synthetic
"container" node wrapping the variants inside the file: each variant's own object (or armature)
sits directly at the scene root, since Blender's glTF importer already creates one Collection per
imported file containing every top-level object — that Collection *is* the natural grouping for a
combined file's LOD/state chain, without this project inventing its own node-based substitute for
one.

Each variant is renamed from its raw path (`l1#carcano`, `bulb_b_destroyed`) to a clean `LOD<n>`
label (the un-prefixed base variant becomes `LOD0`), with a `_Destroyed` suffix for a folded-in
destroyed-state chain (`LOD0_Destroyed`, `LOD1_Destroyed`, ...). This isn't just cosmetic: every
LOD of the same rigged mesh shares one skeleton, so every one of (say) "carcano"'s 5 LOD armatures
would otherwise be named "Carcano" identically — Blender's Object namespace is global across every
object type, so same-named top-level objects collide and get auto-suffixed `.001`, `.002`, ... on
import (a real symptom hit during development: the un-prefixed base variant importing as
`chandelier_long_base.001` instead of a clean name, because it happened to share a name with this
feature's own first-draft container node). A skinned variant's mesh object (a separate Blender
object from its armature, parented to it) is named `LOD<n> Mesh` so it doesn't in turn collide
with its own armature's `LOD<n>` name in that same global namespace.

For `.obj` output, since OBJ has no scene-graph nesting, each variant's parts are labeled with the
same `LOD<n>` scheme instead of the bone name alone (e.g. `LOD1_Bolt` rather than just `Bolt`), so
same-named parts across different variants stay distinguishable and individually selectable once
imported.

Pass `--separate-lods` to opt back out and restore the original one-file-per-variant layout (550
mesh files for the same real sample) — useful for a workflow that specifically wants to inspect
or import a single LOD level in isolation, without pulling in every other variant's geometry.

### A note on axis orientation

Exported positions currently get **no** transform (raw, as decoded) — the triangle winding
order is flipped instead. This took two wrong attempts to get here (a single-axis negation,
then a 90-degree rotation about X that came out rotated the wrong way), and the current state
is a mathematical composition of what's been confirmed so far, not a fresh guess — see
`objAxes`'s doc comment in `internal/cli/package.go` for the derivation and, importantly, the
tension it's still in with the *original* bug report (this exact position math was what
produced the original "upside-down" complaint) — the working theory is that the original issue
was actually a face-winding/normals problem, which the winding flip now addresses, but that's
unconfirmed. If models still look wrong after this change, that's the part to revisit.

This same `objAxes` identity mapping (and the same winding flip, via the shared `flipWinding`
helper) is reused for `.glb` export, not re-derived separately: glTF mandates a Y-up convention,
and Blender's glTF importer converts that to Blender's native Z-up with the exact same rotation
its default OBJ import axis settings already apply — so whatever raw axis values make OBJ import
correctly should make glTF import correctly too, without needing its own independent user-tested
derivation.

## Commands

```text
asr-text-extractor package unpack <file> [output-dir] [--convert dds|png] [--mesh-format gltf|obj|both] [--separate-lods]
```

Extracts:

- every manifest-referenced sub-file, to `<output-dir>/files/<relative-path>` (backslashes
  normalized, any `.`/`..` component dropped) — raw bytes, format not otherwise interpreted
- every embedded texture found among the package's tagged sections, to
  `<output-dir>/textures/<relative-path>.dds` (or `.png` with `--convert png`) — same decoding
  and `--convert` behavior as the standalone [`texture`](Texture-Extraction.md) command
- every decodable mesh (see Meshes above) found among the same tagged sections, to
  `<output-dir>/meshes/<name>.glb` by default — binary glTF 2.0, importable directly into
  Blender or any other modern 3D tool. Meshes with a matched skeleton (see Skinning above) come
  out as a real, riggable armature with individually posable bones, not just static geometry.
  LOD variants and destroyed-state counterparts of the same base object are combined into that
  one file by default (see "Combining LOD and destroyed-state variants" above); pass
  `--separate-lods` for the original one-file-per-variant layout. Pass `--mesh-format obj` (or
  `both`, for both formats) to also/instead get a plain Wavefront OBJ — no armature, but usable
  in tools that don't handle glTF skinning; see OBJ export above. Neither format writes normals
  (`Mesh` doesn't decode any) — use Blender's Shade Smooth / Recalculate Normals after import.
- any embedded RSCF audio entries (resource-type 3, see [Sound Extraction](Sound-Extraction.md))
  found among the same tagged sections, to `<output-dir>/audio/<relative-path>.wav` — not yet
  seen in a real sample (every real audio RSCF section found so far has been a standalone
  `.pc.sounds` file, extracted via `sound unpack` instead), but handled the same way if one
  ever turns up.

If `output-dir` is omitted, it defaults to the input's base name. Creates subdirectories as
needed, and prints a one-line diagnostic to stderr (`Entries: N  Textures: N  Meshes: N  Audio: N`)
before extracting.

```sh
asr-text-extractor package unpack h_hellbase.pc
# -> h_hellbase/files/LevelExportTemp0/ZA/Dust/ZA4_Mist_UnderLights_Small.pfx
# -> h_hellbase/textures/graphics/za4/rocks/scan_rock_cluster_01_ar.dds
# -> h_hellbase/meshes/ammo_crate_insideboxes_c.glb
# -> h_hellbase/meshes/chandelier_long_base.glb  (bundles that object's 7 LOD variants)
# -> ... (282 files, 2502 textures, 550 meshes decoded into 272 combined mesh files)

asr-text-extractor package unpack h_hellbase.pc_entdata
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.snd
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.nav
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.cut
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.ent
# (no textures or meshes section in this file — Textures: 0  Meshes: 0  Audio: 0)

asr-text-extractor package unpack GUIMenu/frontend.gui --convert png
# -> frontend/textures/graphics/autottl_frontend_4.png
# (a .gui file is itself an AsuraZbb package, just with an FNFO header and no RSFL manifest
# behind it — see the "third variant" note above; Entries: 0  Textures: 40  Meshes: 0  Audio: 0)
```

Like `sound` and `texture`, this is extract-only: there's no `--format`/`--encoding` and no
repack path yet.

## Known limitations

See [games/](games/Zombie-Army-4.md) for per-game verification status — mesh decoding, sub-file
extraction, and texture extraction are all confirmed working on every title tested (Zombie Army
4, Sniper Elite 5, Sniper Elite Resistance). Mesh decoding needed a real, confirmed
engine-revision-specific fix to get there — see
[games/Sniper-Elite-5.md](games/Sniper-Elite-5.md#mesh-decoding-the-fix) for the byte-level
detail.

- `PBRV` and every other non-`RSCF` tagged section between the manifest and its sub-files are
  skipped, not decoded.
- `Mesh` doesn't decode normals, even though the reference implementation this was ported from
  does. Neither export format therefore has shading data; Blender's own recalculate-normals
  fills the gap reasonably well for most props, static or skinned.
- The glTF armature's joint nodes are flat (not nested per `Bone.ParentIndex`), so posing one
  bone doesn't carry its children along automatically the way a conventional hand-authored rig
  would — see "Exporting a real, riggable armature" above for why. Only the bind-pose rig itself
  is exported; the game's own `.anim` animation data isn't parsed or imported.
- OBJ export has no concept of a skeleton or vertex groups at all — multi-part meshes are
  instead split into separate, independently selectable-but-not-posable objects by bone (see OBJ
  export above). Use the default glTF export for anything that needs actual posing/rigging.
- Combined LOD/destroyed-state files nest each variant as its own independent armature/skin
  rather than sharing one skeleton instance across variants — posing one LOD's bone has no
  effect on any other variant's mesh. Only the exact `<name>_destroyed` naming convention is
  recognized for state variants; other real naming patterns for alternate object states, if any
  exist in other samples, aren't folded in.
- Texture-to-mesh matching (see Embedded textures above) is a name-based heuristic, not a
  confirmed byte-level link — a mesh with no related texture folder or filename anywhere in the
  package, or textures that don't follow the recognized suffix convention, gets no embedded
  material at all. Verified real coverage on real level packages is roughly 40–50% of exported
  meshes across every title tested, not 100% — objects whose real textures use a name with no
  shared word at all (e.g. a mesh named "weldingkit_cylinder_tall_red" whose real texture folder
  is named "welding_tank_01") are a genuine, expected gap, not a bug. Metallic/roughness maps
  aren't embedded (channel-packing convention unconfirmed for a plain `_m` suffix), and the
  embedded texture is whatever resolution this package itself carries, which for a streamed
  texture can be a small fallback copy rather than the game's true resolution (see the "On
  resolution" note above).
- `HSKN`'s own on-disk layout has several fields gated by a version number and a flags bitfield
  that aren't decoded, only skipped past (bone names are the exception — parsed best-effort,
  since they aren't needed for skinning to work but are useful to have). Every real `HSKN`
  chunk sampled uses the same version, so only 4 of the format's possible flag/version
  combinations have been exercised.
- Mesh decoding was validated by checking that decoded triangle edge lengths are small relative
  to each mesh's own bounding box and that bounding-box sizes are physically plausible across a
  wide range of named objects, and skinning was confirmed by an actual user visual check in
  Blender on one real multi-part mesh (see Skinning above) — not a systematic check across many
  rigged meshes. If an exported mesh looks wrong once opened in a 3D viewer, that's a real
  signal worth reporting back.
- The manifest entry's trailing `uint32` field (after offset and size) is always `1` across
  every entry in the real samples tested; its meaning beyond that isn't confirmed.
- `.snd`, `.cut`, and `.ent` sub-files extract as raw, uninterpreted bytes — no attempt is made
  to further parse their internal structure (`.nav`, by contrast, starts with a recognizable
  `WPSG`-tagged section, also unparsed). `.anim`/`.pfx` sub-files decode to a small count field
  followed by the sub-file's own name, which isn't parsed further either.
- Only tested against one real `.pc` (473MB decompressed, 282 sub-files, 2,502 textures) and one
  real `.pc_entdata` (no embedded textures) sample, both from the same Zombie Army 4 level.
- Extract-only: no way to repack an edited sub-file or texture back into a package yet (true of
  every asset type except text/voice strings — see
  [Home](Home.md#planned-not-yet-implemented)).
- `.navmesh` files (e.g. `navmesh/Hellbase/h_hellbase_actor.navmesh`) are `AsuraZbb`-wrapped
  like a `.pc`, but decompress into an unrelated `ARNM`-tagged chunk — a large, real navigation
  mesh, not a manifest — that this tool doesn't understand yet. `package unpack` runs against
  one without erroring (it's silently skipped by the generic tagged-section walk, matching its
  own declared length) but extracts nothing. Initial research (not yet implemented): `ARNM`
  contains hundreds of variable-length records each starting with a repeating `"VAND"` marker,
  most spanning roughly 300-450 bytes, whose payload contains float triples that look like real,
  spatially-clustered level-space vertex coordinates (some values repeat 2-3 times within one
  record, consistent with a triangle/polygon fan sharing vertices) — a genuine navmesh geometry
  format, not yet cracked.
