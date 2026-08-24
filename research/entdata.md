# `.pc_entdata` / `.cut` / `.ent` research notes

Status as of this writing: **genuinely promising, not cracked into a parser yet.** This is a
much richer vein than `.anim` turned out to be — real, verified level-entity transform data
(position/rotation/scale) has been found, plus a fully-decoded texture-list section, plus a
major structural insight about how the manifest's sub-files actually relate to each other. No
Go code exists for any of this yet. Everything below is pure research: real bytes from a real
sample (`h_hellbase.pc_entdata`), inspected with throwaway Python scripts (never committed).

## The headline structural discovery: `.cut` and `.ent` are one continuous stream

`package.go`'s `Package.Entries` already extracts `.snd`, `.nav`, `.cut`, `.ent` as four
separate `PackageEntry` byte slices, sliced by the manifest's own declared offset/size per
entry — and that extraction is **confirmed correct**, verified directly against
`asura.ParsePackage`'s own output, not just assumed. But those four declared boundaries do
**not** correspond to independent, self-contained file formats. Concretely: a texture-path
string (`\specialfx\lens_glare\Combined_01.tga`) is split exactly in half by the `.cut`/`.ent`
boundary — `.cut`'s last 12 bytes are `\specialfx\l`, and `.ent`'s first bytes are
`ens_glare\Combined_01.tga\x00\x00\x00\specialfx\...`. This isn't a bug in this project's own
extraction (double-checked with a standalone Go program calling `asura.ParsePackage` directly);
it's how the underlying data is genuinely laid out. A `"TEXT"`-tagged section starting near the
end of `.cut` declares a size that lands its end **exactly** 324 bytes into `.ent`, and at that
exact predicted byte, a brand new tag (`"SMSG"`) begins cleanly — a precise, quantitatively
confirmed boundary crossing, not a coincidence.

**Practical implication: a correct parser for this data must treat `.cut` + `.ent` (at minimum;
`.snd`/`.nav` weren't checked for the same thing, see "Open questions") as one concatenated byte
stream, not four independent sub-files.** Walking generic tagged sections from a point partway
through `.cut` all the way through `.ent`'s end works cleanly; walking `.ent` alone from its own
offset-zero does not (its very first bytes are mid-string, not a section start).

## The tag catalog: this data is *almost entirely* walkable with existing project machinery

A brute-force scan of the concatenated `cut+ent` stream (941,186 bytes total) for the same
generic "4 uppercase ASCII letters + `uint32` size" framing this project already uses elsewhere
(`skipTaggedSection` in `pkg/asura/package.go`, already relied on for `PBRV`/`SDPH`/`HSKN`/etc.
inside `.pc` packages) finds:

- **1927 valid tag+size sections**, covering nearly the entire file.
- Only **1986 individual byte positions** had to be skipped one-at-a-time because they didn't
  start a recognizable section (out of 941,186 total bytes) — the walk is almost never "lost."

Tag frequency found:

| tag | count | first seen at (offset in `cut+ent`) | first size |
|---|---|---|---|
| `ENTI` | 1808 | 26321 | 113 |
| `CTEV` | 55 | 5933 | 118 |
| `CTTR` | 28 | 2118 | 293 |
| `CTAC` | 25 | 1962 | 156 |
| `CTAT` | 7 | 2731 | 162 |
| `CUTS` | 2 | 8427 | 5618 |
| `TEXT` | 1 | 26676 | 356 |
| `SMSG` | 1 | 27032 | 57637 |

Reading the names: `CT*` tags (`CTEV`, `CTTR`, `CTAC`, `CTAT`) are concentrated early in `.cut`
(offsets 1962–8427) and are presumably Cutscene EVent / TRack / ACtion / (unknown) records —
not investigated beyond noticing where they live. `CUTS` appears twice, once small (in
`.nav`, see below) and once large (5618 bytes, inside `.cut` itself) — likely the same concept
(a cutscene reference/definition) at different scopes. `TEXT` and `SMSG` are singular, large
blocks (see below). **`ENTI` — "Entity" — dominates by far, and is the main subject of this
document.**

## The `TEXT` section: fully cracked

Structure (all offsets relative to the section's own `"TEXT"` tag):

```
offset 0:  "TEXT"           (4 bytes, tag)
offset 4:  u32 = 356         (total section size, tag-relative — standard framing)
offset 8:  u32 = 3           (meaning unknown)
offset 12: u32 = 0           (meaning unknown)
offset 16: u32 = 7           (STRING COUNT — matches exactly: 7 strings follow)
offset 20: 7 × NUL-terminated ASCII strings, each padded so the next one starts cleanly
```

Confirmed by direct inspection: exactly 7 backslash-prefixed asset paths follow (all texture
references — `.tga`/`.dds`/`.bmp` files under `\specialfx\...`), and the section's declared
size lands exactly on the next tag (`SMSG`) with no slack. This is about as solid a crack as
this investigation has produced for any *non*-`ENTI` section — small enough to fully verify by
inspection, and it directly matches this project's already-established generic
tag+size+count+strings convention (the same shape used for the `RSFL` manifest in
`pkg/asura/package.go`).

## `ENTI` records: confirmed shared header, type-specific tail — and real position data found

### Two distinct populations of `ENTI`

The **first 3** `ENTI` records found (offsets 26321, 26434, 26547 — inside `.cut`, before the
`TEXT`/boundary-crossing region) are **cutscene trigger markers**, not level objects — each one
ends in a short, human-readable name: `"Outro"`, `"Intro"`, `"Cutscene_Exit_Available"`. These
are presumably named checkpoints/events within the cutscene system, not placed world objects.

The remaining **1805** `ENTI` records, starting immediately after the big `SMSG` block ends (at
absolute offset 84669), are the real subject of interest — level object instances.

### The shared header (first ~44 bytes of every `ENTI` record examined)

Every `ENTI` record inspected so far (light entities and the "generic positioned object" record
described below) opens with a structurally similar prefix. Example (hex, from the record at
offset 86128):

```
45 4e 54 49 96 01 00 00 01 00 00 00 00 00 00 00 d4 d9 6a 00 4b 00 00 00
08 00 00 00 0c 00 00 80 00 38 00 00 00 00 00 00 00 00 00 00 00 01 01 01
00 00 00 00 20 00 00 00 00 00 00 00 01 00 00 00
22 69 2e da 66 18 65 ab ed 2b 7b 0f ac 98 e9 11
02 00 00 00 00 00 00 00 00 00 00 00 00
```

Notable pieces, **none of them decoded yet**, just described:

- `"ENTI"` (4 bytes) + `u32` total size (standard framing, confirmed — every record's declared
  size correctly lands on the next tag).
- A 16-byte sequence (`22 69 2e da 66 18 65 ab ?? 2c 7b 0f ac 98 e9 11`) that recurs across
  *consecutive* records with only 1–2 bytes differing (e.g. `3d`, `3c`, `3b` at one position
  across three adjacent records seen). Strongly suggestive of a creation-time GUID or an
  incrementing per-object ID, where most bytes are shared (same authoring session/level) and a
  couple of low-order bytes vary per object — not confirmed, just the obvious shape.
- A repeated `e5 24 ab ab` value seen in multiple records at what looks like the same relative
  position — candidate: a hash of a shared type/class name (e.g. a "Light" or "SpotLight"
  entity class), shared across several similar objects, analogous to how `MeshGroup.Hash` in
  `pkg/asura/mesh.go` is a material identifier with an unknown hash algorithm. Not resolved
  here either.
- No readable name/string appears in this shared header region for the "positioned object"
  record examined in detail below — if entities are named at all (beyond the 3 cutscene-marker
  `ENTI`s which *do* have plain names), the name is likely a hash too, or lives outside the
  `ENTI` record itself (see "Open questions").

### A confirmed, verified TRS transform, in at least one `ENTI` record type

The record at absolute offset 86128 (406 bytes total) has, starting at **byte offset 93 from
the record's own `"ENTI"` tag**, twelve consecutive `float32` values that decompose cleanly and
convincingly as:

```
offset 93:  position.x = -81.7621
offset 97:  position.y = -70.9749
offset 101: position.z = 159.7562
offset 105: quat1.x = -0.712579
offset 109: quat1.y = -0.002390
offset 113: quat1.z = -0.701588
offset 117: quat1.w =  0.002390
offset 121: scale?   =  0.999980   (i.e. 1.0 within float32 noise)
offset 125: quat2.x = -0.005835
offset 129: quat2.y =  0.701588
offset 133: quat2.z = -0.005835
offset 137: quat2.w = -0.712559
```

**Verified, not eyeballed:** `quat1`'s magnitude is `1.000003` and `quat2`'s magnitude is
`1.000017` — both within float32 rounding of exactly `1.0`. Two independent 4-tuples, both
genuine unit quaternions, sitting either side of a value that's `1.0` within the same
tolerance. This is about as strong a confirmation as a single record can give without a second
independent sample to cross-check against (see "Open questions" — this hasn't been validated
against a *second* record of the same type/offset yet, which would be the obvious next step,
the same way the `.anim` investigation always sought a second confirming sample before trusting
a formula).

What the second quaternion represents isn't known. Candidates, none confirmed: a "previous
frame"/delta rotation for some kind of interpolation or physics purposes; a separate rotation
for a different coordinate space (local vs. world); or something unrelated to orientation
entirely that just happens to also be unit-length. This record (and its two visually similar
siblings at offsets 84669 and 85088/85507, which also embed a texture-path string —
`\specialfx\lens_glare\Combined_01.tga` and `\specialfx\Fog\Fog_Hellbase.tga` respectively —
further into their own content) appear to be **light/effect entities** (a lens flare and a fog
volume, going by the referenced textures), not simple static mesh placements — meaning the
*true* general "place a mesh here" record type, if this project's parsing eventually needs one,
may look different again. This wasn't disambiguated further.

### A second, independently confirmed transform layout — and it's the *dominant* entity type

A follow-up pass cross-checked the size-406 finding against a second record of the same size
(only 2 exist total) — both confirmed, quaternion magnitudes `1.00000`/`1.00002` for the first
and `1.00000`/`1.00002` for the second, at the identical offsets. Real, if a small sample.

Far more valuable: the **single most common `ENTI` record size, 302 bytes (407 of 1805 records
— 22.5% of all level entities)**, has its own transform, found the same way the `.anim`
investigation's breakthroughs were found — scanning every byte offset across many records for a
window whose values are consistently a valid unit quaternion (magnitude ≈ 1, not trivial
`0`/`1` constants) — and confirmed here with a **much larger, far more convincing sample: 10
independent records, all clean.**

```
offset 101: position.x, position.y, position.z   (float32 × 3)
offset 113: quat.x, quat.y, quat.z, quat.w        (float32 × 4, verified unit length in all 10)
```

Real decoded values (10 records, `(position) → quaternion`):

```
(-17.854, -5.722, -249.492)  → (0.0,     0.9328,  0.0,    -0.3605)
(-10.009,  2.012, -335.129)  → (-0.007, -0.0694,  0.002,   0.9976)
(-13.382,  0.457, -306.900)  → (0.0067,  0.7509,  0.0058,  0.6604)
(  2.703, -3.546, -305.044)  → (-0.001,  0.1039, -0.0092,  0.9945)
( 12.529, -2.804, -289.004)  → (0.0,    -0.0045,  0.0,     1.0)
( -4.107, -1.637, -259.118)  → (0.0,     0.9994,  0.0,    -0.0349)
(-24.711, -6.790, -250.461)  → (0.0003, -0.0718, -0.0001,  0.9974)
( 18.947, -6.839, -258.223)  → (0.0,     0.2958,  0.0,     0.9553)
(-16.597,-235.357, 263.576)  → (0.0452,  0.6475,  0.0597,  0.7584)
( -4.051,-232.391, 250.829)  → (-0.0,    0.0403,  0.0,     0.9992)
```

Every position is in a sensible level-coordinate range (tens to low hundreds, both signs);
every quaternion is unit-length to 3+ decimal places and genuinely varies between records —
this is unambiguously real per-object placement data, not noise or a coincidental match. Note
the transform starts at a **different absolute offset than the 406-byte type** (101 vs. 93) —
the 12-byte position-then-quaternion shape is consistent, but the fixed header in front of it
is 8 bytes longer for this type. This confirms the earlier suspicion that header length (and
therefore transform offset) is entity-type-dependent, not a single fixed constant across all
`ENTI` records — any future parser needs to locate the transform relative to a type-specific
header length, not a single hardcoded offset.

Also found for this record type (not yet understood, noted for completeness):

- Two more 4-float slots later in the record that are **constant across every record checked**
  — `(0, 0, -1, 0)` and `(0, 0, 0, 1)` — clearly fixed/default values, not per-object data (the
  second one is genuine identity in the `w=1` convention). Not investigated further.
- A `float32` at offset 68, value `112.5959`, identical across every record of this type
  checked — candidate: a per-*type* constant (e.g. a shared default radius/range), not a
  per-object value, since it never varies while position/rotation do.
- **No plain string anywhere in this record type** (confirmed: a printable-ASCII-run search
  found only the `"ENTI"` tag itself) — this type references whatever object it represents
  purely by hash/ID, unlike the three cutscene-marker `ENTI` records which do carry plain
  names. Consistent with "entity name/type is a hash looked up elsewhere" being the norm, not
  the light-entity/cutscene-marker cases being the exception.
- The exact same 12 bytes making up the confirmed position reappear **twice more**, verbatim,
  later in the same record (once around offset 145, once around offset 165, based on visual
  inspection of the record's raw hex). Candidate explanations, none confirmed: a bounding-box
  min/max pair that happens to collapse to a single point for this object, or the same position
  serving two different purposes (e.g. render vs. physics) that happen to coincide.

## Open questions

1. **Entity type dispatch.** Nothing found so far explains, from the shared header alone, which
   "shape" of trailing content a given `ENTI` record has before parsing it (light entity with
   embedded texture reference vs. the confirmed TRS-transform layout vs. potentially other
   shapes not seen yet, e.g. plain mesh placements or triggers). The repeated `e5 24 ab ab`
   hash-like value is the most promising lead — if it's genuinely a type/class hash, a survey
   of its distinct values across all 1805 records, cross-referenced with each record's actual
   observed layout, would likely reveal the dispatch mechanism.

2. ~~Only one record's transform has been checked.~~ **Resolved.** Both the size-406 type (2/2
   records) and, far more convincingly, the size-302 type (10/10 records, the single most
   common `ENTI` size at 407 occurrences) now have independently confirmed transform offsets
   with consistently valid unit quaternions and sensible varying positions. See "A second,
   independently confirmed transform layout" above. The offset is confirmed *type-specific*
   (93 for the 406-byte type, 101 for the 302-byte type) — a future parser needs to locate it
   relative to each type's own header length, not assume one global offset.

3. **No object/mesh name or hash-reference found yet in *either* confirmed transform-bearing
   record type.** Both the size-406 and size-302 types were checked for embedded plain strings
   — neither has one anywhere in the record (confirmed via a printable-ASCII-run search over
   the full record, not just the transform region). If these entities are meant to place actual
   game objects (with meshes, like the `carcano` rifle or the various props this project
   already extracts via `package.go`'s `RSCF` walk), the object reference is presumably a hash,
   not a string — and this project doesn't have a hash-reversal capability, since
   `MeshGroup.Hash`'s own algorithm is itself unknown (see `pkg/asura/mesh.go`). Until either
   that hash algorithm is found, or a record type turns up that references its object by plain
   string instead (the way the light-entity records reference their *textures* by string), it
   won't be possible to confirm which specific game object a given transform actually
   positions — meaning even a fully-parsed transform can't yet be tied back to "this is where
   the carcano rifle goes."

4. **`SMSG` (57,637 bytes, one single block) is completely unexplored.** Treated as opaque
   (skipped via its own declared size) in the tag-walking survey above. Given its size — larger
   than everything else in the file combined except the 1808 `ENTI` records — it's plausibly
   either a big flat data table (not sub-tagged, or sub-tagged with a scheme the brute-force
   tag scanner didn't recognize) or something in a genuinely different format nested inside.
   Worth a dedicated look.

5. **`CTEV`/`CTTR`/`CTAC`/`CTAT` (cutscene event/track/action/? records) not examined at all**
   beyond noting where they live and how many there are. Likely valuable if cutscene/trigger
   data ever becomes a target, but wasn't prioritized here since `ENTI` (level object placement)
   seemed the higher-value target given this project's existing mesh/skeleton/texture
   extraction work.

6. **`.snd` was not checked for the same cross-boundary-stream behavior found between `.cut`
   and `.ent`.** Given `.snd` is the manifest entry immediately *before* `.nav` (order:
   `.snd`, `.nav`, `.cut`, `.ent`), and `.nav` was found to be small and fully self-contained
   (see below) rather than spilling into `.cut`, the cross-boundary behavior may be specific to
   the `.cut`/`.ent` pair rather than universal across all four sub-files — not confirmed
   either way for `.snd`.

## A smaller, fully-understood file: `.nav` (91 bytes)

Not part of the `.cut`+`.ent` stream (checked: `.nav`'s own `"CUTS"` section is fully
self-contained, 32 bytes, ending cleanly at the file's own declared end — no evidence it spills
into or out of `.cut`). Worth recording since it was fully decoded:

```
offset 0:  5 bytes, all zero in this sample — unexplained prefix, meaning unknown
offset 5:  "WPSG" tag
offset 9:  u32 = 54 (section size, tag-relative — lands exactly on the next tag, confirming
           standard framing)
offset 13: u16 = 4 — meaning unknown (a count? a type?)
offset 15: 11 × float32 — clean, plausible-looking values:
           0.0, 0.5, 3.0, 0.5, 1.5, 0.5, 6.1, 4.0, 5.0, 10.0
           (not further decomposed into a meaningful per-waypoint record; "WPSG" plausibly
           stands for "WayPoint (Segment/Graph)" given the name, but this is a guess)
offset 59: "CUTS" tag
offset 63: u32 = 1994 (0x7ca) — candidate: a hash, possibly of the cutscene name that follows
offset 67: u32 = 34 — meaning unknown
offset 71: u32 = 0 — meaning unknown
offset 75: NUL-terminated ASCII string: "Outro" (a cutscene name — presumably the same "Outro"
           cutscene-marker `ENTI` record found in `.cut`, see above)
offset ~86: 4 trailing bytes, meaning unknown
```

This file is low-value on its own (91 bytes, one waypoint-ish section and one cutscene
reference) but was useful as a small, fully-tractable confirmation that the generic tag+size
framing genuinely applies here too, before diving into the much bigger and more valuable
`.cut`+`.ent` stream.

## Suggested next steps, roughly in order of expected value

1. ~~Cross-check the TRS transform against more `ENTI` records.~~ **Done for two types** (406-
   byte: 2/2 records; 302-byte, the dominant type: 10/10 records) — see "A second, independently
   confirmed transform layout" above. Worth doing the same for a few more of the size buckets
   from the size-distribution table (366, 267, 182, 165, 153, 511, 113, ...) to see how many
   distinct transform "shapes" actually exist, before assuming the two found so far are the only
   ones.
2. **Find the entity-type dispatch mechanism** (see Open Questions #1) — still the single
   highest-value next step. Now that two concrete transform-bearing types are confirmed (406 and
   302 bytes), a promising concrete test: check whether the `float32` constant found for each
   type (`112.5959` for the 302-byte type; an equivalent per-type constant likely exists for the
   406-byte type but wasn't specifically checked) or the repeated `e5 24 ab ab`-style hash
   differs consistently *by size bucket* — if a single small header field reliably predicts
   record size (and therefore shape), that's the dispatch mechanism.
3. **Find a way to tie a transform to a specific game object** (see Open Questions #3) — now the
   real remaining blocker for anything level-layout-reconstruction-shaped, since both confirmed
   transform-bearing types reference their object purely by hash, and this project has no way to
   reverse a hash back to a name. The `MeshGroup.Hash` question in `pkg/asura/mesh.go` and this
   one may turn out to be the same unsolved problem, or may be unrelated — worth checking whether
   the same hash algorithm is even plausible (e.g. do the same input strings, hashed, ever
   produce values seen in both contexts?) before assuming they're independent.
4. **Investigate `SMSG`'s 57KB of content** — currently completely opaque.
5. Once entity parsing is trustworthy: a Go implementation in `pkg/asura`, following this
   project's usual practice of verifying against real sample data with numeric
   self-consistency checks before trusting it (matching how `rscf.go`/`mesh.go`/`skeleton.go`
   were each built).
