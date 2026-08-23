# Package Extraction

Covers the `package` command: extracting manifest-referenced sub-files and embedded textures
from an `AsuraZbb`-compressed level-package file (`.pc`, `.pc_entdata`).

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

The internal layout of `PBRV` (a *separate* geometry/spatial-data block, several megabytes in a
typical level — not the same thing as the RSCF mesh entries described below) has not been
reverse-engineered and isn't parsed; it's only skipped over via its declared length, same as
every other unidentified section.

## Meshes

Some `RSCF` entries (resource-type 0) are per-object render meshes rather than textures: a
header, one or more material groups, a vertex buffer, and a shared triangle-index buffer. This
format was **not** reverse-engineered from this project's own sample data — it's a direct,
field-for-field port of a dedicated, independently-authored Zombie Army 4 reverse-engineering
project's own working Blender importer
(`zombie_army_4_findings-master/ZombieArmy4Loader/model.py`), which had this format fully
solved already. (An earlier version of this tool's mesh support was instead based on a format
hypothesis ported from a *different* Rebellion game, Evil Genius 2 — that version's exported
models turned out to be garbled nonsense once actually opened in Blender, not the "close
enough" the numbers alone had suggested; see `pkg/asura/mesh.go`'s doc comments for what went
wrong. That version is gone from the code now, but its lesson stands: a format hypothesis
validated only by total-byte-count reconciliation, with no way to visually inspect the result,
is not validated.)

The header is `44 + 24*groupCount` bytes: 5 `uint32` fields (group count, vertex count, total
index count, a redundant triangle count, and one unconfirmed field), then one 24-byte record
per group (a material hash plus that group's own index count), then a 3-float position
dequantization scale and a 3-float offset. Per vertex (48-byte stride): a quantized position (3
× `uint16` at offset 0, dequantized per axis as `raw/32767 * (scale/2) + offset`), two UV
channels (2 × half-float each) at offsets 24 and 28, and up to 8 bone weight/index pairs used
for skinning (see below). Normals aren't decoded.

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

Each bone's transform is applied on its own, without composing it through its parent's rotation
the way a general skeletal-animation system normally would — an earlier version did compose
through the hierarchy (the textbook-correct approach), and it was wrong for this data: the
root's real 180-degree rotation, composed into a child bone's own rotation, canceled out to a
net identity (180+180=360), while composing it into the child's *position* still flipped that
offset's sign — so the root mesh (correctly rotated) looked right while its rigid sub-parts
didn't. That mismatch — main body correct, individually-positioned sub-parts upside-down — is
exactly what a user visual check in Blender caught; the numeric bounding-box/edge-length
validation this project could do on its own couldn't have caught it, since it doesn't check
per-part *orientation*, only aggregate size and local coherence.

Meshes with no matching skeleton, or whose vertices carry no bone weight at all, are returned
unchanged — this is automatic and free for meshes that don't need it.

`HSKN`'s own on-disk layout has several fields gated by a version number and a flags bitfield
that this tool doesn't fully decode (only what's needed to reach the bind-pose transform data);
per-bone names are parsed best-effort past that point but aren't required for skinning to work.

Exported OBJ files split into named `g` groups by bone (e.g. `g Body`, `g Bolt`) whenever a
skeleton was matched — a *different* grouping than the mesh's own material `Groups` (which
"carcano" has only one of, despite having five distinct bones) — so individual rigid sub-parts
can be selected/hidden/moved independently in Blender. Meshes with no matching skeleton export
as a single ungrouped mesh, same as before.

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

## Commands

```text
asr-text-extractor package unpack <file> [output-dir] [--convert dds|png]
```

Extracts:

- every manifest-referenced sub-file, to `<output-dir>/files/<relative-path>` (backslashes
  normalized, any `.`/`..` component dropped) — raw bytes, format not otherwise interpreted
- every embedded texture found among the package's tagged sections, to
  `<output-dir>/textures/<relative-path>.dds` (or `.png` with `--convert png`) — same decoding
  and `--convert` behavior as the standalone [`texture`](Texture-Extraction.md) command
- every decodable mesh (see Meshes above) found among the same tagged sections, to
  `<output-dir>/meshes/<name>.obj` — a plain Wavefront OBJ with vertex positions, UVs, and
  triangle faces, importable directly into Blender or any other 3D tool. No normals are
  written (`Mesh` doesn't decode any) — use Blender's Shade Smooth / Recalculate Normals after
  import.

If `output-dir` is omitted, it defaults to the input's base name. Creates subdirectories as
needed, and prints a one-line diagnostic to stderr (`Entries: N  Textures: N  Meshes: N`)
before extracting.

```sh
asr-text-extractor package unpack h_hellbase.pc
# -> h_hellbase/files/LevelExportTemp0/ZA/Dust/ZA4_Mist_UnderLights_Small.pfx
# -> h_hellbase/textures/graphics/za4/rocks/scan_rock_cluster_01_ar.dds
# -> h_hellbase/meshes/ammo_crate_insideboxes_c.obj
# -> ... (282 files, 2502 textures, 550 meshes)

asr-text-extractor package unpack h_hellbase.pc_entdata
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.snd
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.nav
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.cut
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.ent
# (no textures or meshes section in this file — Textures: 0  Meshes: 0)
```

Like `sound` and `texture`, this is extract-only: there's no `--format`/`--encoding` and no
repack path yet.

## Known limitations

- `PBRV` and every other non-`RSCF` tagged section between the manifest and its sub-files are
  skipped, not decoded.
- `Mesh` doesn't decode normals, even though the reference implementation this was ported from
  does. OBJ export therefore has no shading data; Blender's own recalculate-normals fills the
  gap reasonably well for most props, static or skinned.
- Skinning applies each vertex's bone weights, but OBJ itself has no concept of a skeleton or
  vertex groups — the export is a single static (already-skinned) mesh in its bind pose, not a
  riggable one. A richer export format (glTF, most likely) would be needed to carry the
  skeleton itself across for actual re-posing/animation work in Blender.
- `HSKN`'s own on-disk layout has several fields gated by a version number and a flags bitfield
  that aren't decoded, only skipped past (bone names are the exception — parsed best-effort,
  since they aren't needed for skinning to work but are useful to have). Every real `HSKN`
  chunk sampled uses the same version, so only 4 of the format's possible flag/version
  combinations have been exercised.
- Mesh decoding was validated by checking that decoded triangle edge lengths are small relative
  to each mesh's own bounding box and that bounding-box sizes are physically plausible across a
  wide range of named objects, and skinning was confirmed by an actual user visual check in
  Blender on one real multi-part mesh (see Skinning above) — not a systematic check across many
  rigged meshes. If an exported OBJ looks wrong once opened in a 3D viewer, that's a real signal
  worth reporting back.
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
