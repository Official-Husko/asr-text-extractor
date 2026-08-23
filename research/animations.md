# `.anim` format research notes

Status as of this writing: **partially cracked, not usable yet.** One numeric component of
the rotation encoding is solidly confirmed (multiple independent matches, including an exact
one). The axis-direction component, the full per-keyframe record layout, and per-bone
attribution are still open. This document exists so a future session (or a different approach
entirely) can pick this up without re-deriving everything from scratch — it's deliberately more
detailed and slower-paced than the summary in `CLAUDE.md`.

No Go code exists for any of this yet. Everything below is pure research: real bytes from real
game files, inspected with throwaway Python scripts (never committed — write your own from the
snippets here if you want to reproduce a step).

## Why this matters / what "done" looks like

The end goal (stated by the user) is: get the game's own `.anim` animation clips playing back on
the armatures this project already exports to glTF (`internal/cli/gltf.go`, see
`addMeshNodes`/`writeGLB`). That needs, in order:

1. A cracked byte format for `.anim` (**in progress, this document**).
2. A Go parser producing, per bone, a list of `(time, rotation, ...)` keyframes.
3. A way to resolve each keyframe track to the *named* bone it belongs to (the per-bone table's
   semantics — see below — not yet understood well enough for this).
4. A pose-composition function for **playback**, which is *not* the same as
   `pkg/asura/skeleton.go`'s `Skeleton.Skin()`. `Skin()` is deliberately flat (no
   `Bone.ParentIndex` composition) and is only used to reconstruct the mesh's *static bind
   pose*. Real animation playback, per the AVP reference (see below), composes hierarchically:
   each bone's pose is computed relative to its already-posed parent, recursively. Reusing
   `Skin()` for playback would be wrong and would need a new function.
5. Wiring the result into glTF as actual `animations` (an `AnimationSampler` per bone/property
   plus an `AnimationChannel` targeting the same joint nodes `addMeshNodes` already creates) —
   glTF has first-class keyframed-animation support built exactly for this, so this step is
   mechanical once steps 1–4 exist, but it doesn't exist yet either.

This document only covers step 1, and only partially.

## Reference material

- **`AVP2010ModelViewer-main/`** (project root, gitignored, read-only, MIT-ish third-party
  source) — an open-source C++ model/animation viewer for *Aliens vs. Predator* (2010), an
  older Rebellion Asura-engine title. Its `Models.h`/`Models.cpp` (`Read_HANM`,
  `Model_anim_update`, `Anim_search_for_nearest_t`, `Dump_anim`) is the *only* reference this
  project has ever found for any `.anim`-shaped data. **Important caveat, confirmed by direct
  byte inspection, not assumed:** AVP is a genuinely different, older engine generation. Its
  on-disk chunk starts with an explicit 16-byte header whose magic literally spells `"HANM"`
  (`asura16b_hdr{magic:'MNAH', chunk_size, type1, type2}` — the C multi-char literal `'MNAH'`
  reads as on-disk bytes `H`,`A`,`N`,`M` in that order on a little-endian read), and it stores
  each keyframe as a **raw `float32` quaternion** (`XMFLOAT4`, 16 bytes) plus a **raw `float32`
  `t_perc`** (4 bytes) = 20 bytes/keyframe, uncompressed. Real Zombie Army 4 `.anim` samples
  have **no** `"HANM"`/`"MNAH"` bytes anywhere in them (checked directly, byte-search, both
  found nothing), and their keyframe data is `int16`-quantized and — as of this document —
  *delta-encoded*, not raw. So AVP's exact byte layout does **not** carry over. What *does*
  carry over, confirmed piece by piece below, is the conceptual shape:
  - `model_skeleton_anim{type, anim_name, anim_hash, amount_bones, have_extra_bone_anim,
    bone_anims[], total_time}` — a per-clip header with a name, a bone count, and a total
    duration.
  - `model_bone_anim{frames, not_change_lenght, rots[], lens[]}` — per bone: a keyframe count, a
    "does this bone's length change" flag, an array of rotation keyframes, and a *separate*
    array of translation/"length" keyframes (1 entry if length doesn't change, `frames` entries
    if it does).
  - `model_bone_rot{rot: quat, t_perc: float}` — each rotation keyframe is a quaternion plus a
    **time normalized to `[0,1]`** (actual time = `t_perc × total_time`).
  - Playback (`Anim_search_for_nearest_t` + `Model_anim_update`): binary-search for the
    bracketing keyframe pair by `t_perc × total_time`, `SLERP` the rotations and `LERP` the
    length/translation vectors by the same blend factor, compose as **rotate-then-translate**,
    and — the important part — compose that result **hierarchically through
    `Bone.parent_bone_id`**, recursively passing the parent's already-computed matrix down to
    its children (`Model_anim_update` calls itself once per child, `M_prev` threaded through).

## The `.anim` sub-file's outer shape (confirmed)

Every sample checked has this structure, in order:

```
[u32 field0]  [u32 zero]  [filename, NUL-terminated ASCII, padded to 4 bytes]
[per-bone table: N × 12 bytes]
[keyframe blob: variable length]
[footer: "HCAN" tag + 28 bytes, OR (rarely) "SDDC" tag + different content]
```

### The wrapper (`field0`, filename)

`field0` is a `uint32`. **It is not the byte length of the name block**, despite looking like it
in the very first sample examined — this was an early wrong hypothesis, caught and corrected,
worth recording explicitly so it isn't re-derived and re-believed:

- `HellBase_Cannon_Fire_Right.anim`: `field0 = 36`, and the padded name block *also* happens to
  end at byte 36 (26-char name). Looked like a match.
- `Null_Gizmo01_Rot_90x_30f.anim`: `field0 = 1`, but the padded name block ends at byte 36
  too (24-char name, same padding math). `1 ≠ 36`. The first match was coincidence.

`field0`'s real meaning is still unknown. Do not assume it's a byte length or an offset.
Whatever it is, it's small (seen values: `36`, `1`, others not yet systematically logged).

The 4 zero bytes after `field0` and the filename itself are solid — the filename is the exact
same string as the manifest path this `.anim` sub-file was extracted under (this is literally
how the manifest-offset-base bug was originally diagnosed, see `CLAUDE.md`'s `package.go`
notes), NUL-terminated, then zero-padded so the *next* field starts on a 4-byte boundary.

**To find the true start of the per-bone table reliably: find the filename's NUL terminator,
then round up to the next multiple of 4 (if already aligned, add 4 more — there must be at
least one padding slot after the NUL).** Do not use `field0` for this. Example in Python:

```python
name_end = data.index(0, 8)          # 8 = skip field0 + zero
table_start = name_end
while table_start % 4 != 0:
    table_start += 1
if table_start == name_end:          # name already landed on a 4-byte boundary
    table_start += 4
```

### The per-bone table

Right after the wrapper sits a table of `N × 12` bytes, where `N` is presumed to be the bone
count (not independently confirmed against a ground-truth skeleton yet — see "Open questions"
below). Two real samples with **identical** rotations but different world axes
(`Null_Gizmo01_Rot_90x_30f.anim` vs `Null_Gizmo02_Rot_90y_30f.anim`, both single-bone `Null`
test objects) have a **byte-for-byte identical** 12-byte table:

```
02 00 01 00 00 00 00 00 00 00 00 00
```

Two more real samples, unrelated objects (`searchlight_150_Body_Tilt_45_Sabotaged.anim` and
`fortress_ventilation_fan_Blades_Big_360.anim`), also share an identical table with each other:

```
06 00 01 00 00 00 00 00 00 00 00 00
```

**The leading `uint16` of this table is a genuine keyframe count — confirmed by exact linear
regression, not guessed.** Treating it as `frames` and testing `blob_length = A + B × frames`
against 4 independent real samples:

| file | `frames` (leading table `u16`) | blob length (bytes, wrapper-end to footer-start) |
|---|---|---|
| `Null_Gizmo01_Rot_90x_30f.anim` | 2 | 90 |
| `searchlight_150_Body_Tilt_45_Sabotaged.anim` | 6 | 130 |
| `fortress_ventilation_fan_Blades_Big_360.anim` | 6 | 130 |
| `fortress_ventilation_fan_Blades_Medium_360.anim` | 6 | 130 |

This fits **exactly** (zero error) as `blob_length = 70 + 10 × frames`. That means: **each
keyframe costs exactly 10 bytes**, and there's a fixed 70-byte block of overhead per bone/clip
(this includes the 12-byte table itself, leaving 58 bytes of further fixed-size structure not
yet identified — plausibly analogous to AVP's post-per-bone-loop fixed fields, e.g. its
`float unk1; DWORD unk2;` plus several conditional blocks gated by header flag bits, none of
which are understood for ZA4 yet).

10 bytes per keyframe is a real, useful constraint on what the rest of this document is trying
to find inside it (see "Open questions").

### The footer

A 32-byte tail, tag + 28 bytes, present in **145 of 146** files surveyed (a whole-corpus check,
every `.anim` extracted from `h_hellbase.pc`'s manifest). Decode the 28 bytes after the tag as
7×`uint32` (or, where noted, reinterpret specific slots as `float32`):

```
u32[0]  — large, clip-specific, arbitrary-looking (candidate: a name hash, cf. AVP's `anim_hash
           = hash_from_str(0, anim_name)` — plausible but not independently confirmed for ZA4)
u32[1]  — CONSTANT = 20 in 145/146 files, zero variance. Rules out "fps" (a real per-clip frame
           rate would almost certainly vary at least once across 146 diverse clips). Most likely
           a format-version-style constant.
u32[2]  — large, clip-specific, not yet interpreted
u32[3]  — small (seen: 1, 2, 133), not yet interpreted — candidate: some kind of type/flags byte
f32[4]  — DURATION, high confidence. Matches AVP's `model_skeleton_anim.total_time`. Confirmed
           plausible across every sample decoded: 2.4667s, 0.6333s, 0.8s, 1.0s, 0.9667s, 13.3333s,
           0.3s, 1.9667s — all physically sensible clip lengths for their respective objects.
f32[5]  — a second float, sometimes 0.0, sometimes a plausible-looking small number (0.5448,
           2.014, 1.391, 1.4687 seen) — meaning not identified. Not obviously derivable from
           anything else found so far.
u32[6]  — small (seen: 1, 2, 7, 8, 9, 17), not yet interpreted
```

Tag = `"HCAN"` for a normal skeletal-animation clip. **One file in the 146-file survey**
(`Explosive_Drum_Chunk_9.anim`) has **no** `"HCAN"` tag at all — it ends in a differently-tagged
`"SDDC"` footer instead. Not investigated beyond noticing it exists; the object name suggests a
destruction/physics-simulation variant rather than a skeletal clip, so it's plausibly a
completely different sub-format sharing only the outer wrapper convention.

## The rotation encoding: what's cracked

### The headline finding: delta-from-identity encoding, not raw components

Every naive attempt to read 4 consecutive `int16` values as `(x, y, z, w) × 32767` and expect
them to match a real quaternion **failed** — the values were never anywhere near
`sin(45°) ≈ 0.7071` for a known 90° rotation. That failure, examined properly instead of
shrugged off, is itself the clue: an exhaustive scan (every `int16`-aligned offset, every
4-value permutation, scored by quaternion distance against the mathematically expected 90°
rotation) found the best matches clustering not on `sin(45°)` but on `1 − cos(45°) ≈ 0.2929`.

That's `W − 1`, not `W`. **The format stores rotation components as deltas from identity**
(reference value `1` for `W`, presumably `0` for `X`/`Y`/`Z`, though the X/Y/Z case is still
unconfirmed — see below). This is a sensible design: most keyframes in a real animation are
close to identity, and a delta encoding puts more usable precision near zero than a raw
`[-1, 1]` linear quantization would.

### Verified data points (all in file `Null_Gizmo01_Rot_90x_30f.anim`, byte offset 56, and its
### sibling `Null_Gizmo02_Rot_90y_30f.anim`, same offset)

```
gizmo_x90 (90° about X): int16[-9599, 32767, 32767, -19196] at file offset 56
gizmo_y90 (90° about Y): int16[32767, 9597, 32767, -19196] at file offset 56
```

Checked with full floating-point precision, not rounded by eye:

```
-9599 / 32767 = -0.292947
cos(45°) - 1  = -0.292893
difference    =  0.000054   (1 int16 quantization step ≈ 0.0000305 — a ~2-step match)

 9597 / 32767 =  0.292886   (same magnitude as above, opposite sign, different slot)

-19196 / 32767 = -0.585833
2 × (cos(45°) - 1) = -0.585786
difference = 0.000047   (same ~1.5-step precision)
```

The `-19196`/`-0.5858` value is **identical in both files**, axis-independent — consistent with
some kind of magnitude-only scalar related to the *size* of the rotation (not its direction),
kept separate from whichever slot carries axis-direction information. Its exact meaning (why
`2×` the delta specifically) is not understood.

**Ruled out as coincidence, not just assumed:** searched an entirely unrelated file
(`Chandelier_Drop.anim`, no reason to expect 45°/90°-rotation content) for the same raw value
(`±9599`, tolerance `±3`) across all 199 of its `int16` values. Zero matches.

### Independent cross-file confirmation, including one *exact* match

Decoding **every** `int16` in `fortress_ventilation_fan_Blades_Big_360.anim`'s keyframe blob
under `W = 1 + raw/32767` and checking which give a mathematically valid cosine (`-1 ≤ W ≤ 1`)
turns up:

- The *same* `-9599` and `-19196` values found in the isolated gizmo file, decoding to the same
  ~90°/~131° angles. Plausible, not just coincidence: a spinning fan blade's own rotation
  legitimately passes through ~45° (half of a 90° step) partway around a full spin.
- **One value hits exactly `int16`'s own minimum, `-32768`** (not an arbitrary number — the
  standard quantization convention for landing as close as possible to `-1` from an asymmetric
  signed 16-bit range: `-32768/32767 ≈ -1.00003`). This decodes to `W ≈ 0.00000` — a **precise
  180° rotation** (`cos(90°) = 0` exactly, no rounding needed to see it). This is the cleanest
  single data point found in the whole investigation.
- A repeated identical value-pair (`17323, -15898`, appearing twice adjacently in the blob) —
  consistent with a multi-blade fan where several bones share the same rotation curve, though
  this specific pair's own meaning (see below) isn't decoded.

**Bottom line: the `W`-delta formula (`raw/32767 = W - 1`, `W = cos(halfAngle)`) is confirmed by
multiple independent matches across multiple unrelated files, including one exact match. This is
the single most solid finding in this whole investigation.**

## Open questions (not cracked, despite real effort)

### 1. The axis-direction component (the actual blocker)

A full quaternion needs `X, Y, Z` (scaled by `sin(halfAngle)` along the rotation axis) in
addition to `W`. For the 90°-about-X gizmo, that means something in the data should be close to
`sin(45°) ≈ 0.7071` (raw ≈ 23170). **Nothing in either gizmo file's blob is close to this value
at any tried offset, scale, or sign.** Two competing hypotheses were tested:

- **Hypothesis A: full quaternion, X/Y/Z stored directly (no delta) alongside delta-encoded
  W.** Predicts a value near raw ≈ ±23170 somewhere in the blob. Not found — a full scan of
  every `int16` in both gizmo files' blobs found nothing within a wide tolerance of this value.
  **Not supported by the data as currently understood.**

- **Hypothesis B: this is a single-DOF "hinge" system, not a full quaternion at all — each bone
  rotates around a single, fixed, *implicit* axis (defined externally, e.g. by the bone's own
  bind-pose orientation from `HSKN`, not stored per-frame in `.anim` at all), and the `.anim`
  data only needs a scalar angle (the `W`-delta value already cracked) per keyframe.** This
  would fully explain why no axis-direction value has ever been found: there isn't one to find.
  Circumstantial support: the per-bone table is **byte-identical** between the X-axis and
  Y-axis gizmo files (see above) — if axis choice were encoded per-clip in that table, it
  should differ and doesn't. Also, `|-9599| ≈ |9597|` — the *magnitude* of the encoded value is
  essentially identical between the X and Y rotation clips, exactly as expected if both are
  "rotate 90° around whichever axis this bone's own rig defines," with only a sign/slot
  difference (plausibly rotation *direction*, not axis *identity*).

  **This hypothesis was not fully tested before this document was written** — it was the
  in-progress leading theory at the point of writing. If picking this up: the next concrete
  test is whether `Null_Gizmo01`/`Null_Gizmo02` have distinguishable `HSKN` skeletons (most
  likely they don't — `Null` objects are typically non-renderable markers with no mesh, so this
  project's existing skeleton-to-mesh matching machinery, built around `Mesh.Skeleton`, may not
  apply directly; a `.anim`-to-`HSKN` matching path independent of mesh matching would need to
  be found or built first). If no skeleton exists for these test objects specifically, look for
  a *rigged* object with a known single-axis-hinge part (a door, a valve wheel, a fan blade —
  several exist in the sample: `Null_Door_Open.anim`, `Explosive_Valve_Tap.anim`,
  `Trap_Trigger_Spinner_rotating.anim`) whose `HSKN` bind-pose bone rotation is known, and check
  whether the *world-space* rotation reconstructed from `.anim`-scalar-angle plus
  `HSKN`-bind-pose-axis actually matches the object's real, expected motion.

- **10-bytes-per-keyframe constraint** (see above): if a keyframe really is `W`-delta (2 bytes)
  + `t_perc` (some width) + nothing else, that leaves up to 8 more bytes unaccounted for per
  keyframe even under the "no axis needed" Hypothesis B — so *something* else is being stored
  per keyframe regardless of which axis hypothesis is right. Reconciling the 10-byte-per-frame
  math with either hypothesis is unfinished work.

### 2. A hypothesis-light scan attempt, and why it didn't work

Tried: for every pair of `int16` values in a blob, check `(1 + wdelta)² + axis² ≈ 1` (the
unit-quaternion constraint for a single-axis rotation), without assuming any particular target
angle — a more general test than checking against one specific expected rotation. **This
produced too many false positives to be useful**: most of the blob is `0`/`±32767`/`±32768`
sentinel/filler data (not yet understood structurally either — see "sentinel pattern" below),
and pairs of those values trivially satisfy the unit-circle equation by construction (e.g.
`0² + 1² = 1`, regardless of whether the two values are actually related to each other or to a
real rotation). A tighter constraint, or a much cleaner sample with less filler, would be
needed to make this approach useful.

### 3. The sentinel/filler pattern

`int16` value `32767` (max positive) and `-32768`/`-1` (extremes of the negative range) recur
constantly throughout every blob examined, clearly *not* meaningful rotation content in most
occurrences — almost certainly a "this slot is unused for this bone/keyframe" placeholder
convention. **Not fully mapped**: which exact slots get this filler, and why, isn't understood
well enough to reliably distinguish "real content coincidentally near an extreme value" (like
the genuine, confirmed `-32768 → 180°` exact match above) from "structural filler with no
numeric meaning" in general. This ambiguity is part of why the broader unit-circle scan (point 2
above) produced noise.

### 4. `t_perc` (per-keyframe time)

Not located at all yet. Per AVP, this should be a value normalized to `[0, 1]`, with actual
time = `t_perc × total_time` (the confirmed footer duration field). Whether ZA4 stores this
per-keyframe (as AVP does) or derives it implicitly (e.g. evenly-spaced keyframes, no explicit
time field needed) is unknown. If evenly-spaced, that would also help explain part of the
10-bytes-per-keyframe budget without needing a dedicated time field.

### 5. Per-bone table field semantics beyond the leading keyframe count

Only the leading `uint16` (keyframe count) of the 12-byte per-bone table is understood. The
remaining 10 bytes (seen as `01 00 00 00 00 00 00 00 00 00` in every sample checked so far —
possibly itself another format constant, given it hasn't varied across any sample yet) are not
decoded.

### 6. Attributing keyframe data to specific named bones

For a multi-bone clip (e.g. `HellBase_Cannon_Fire_Right.anim`, 6 bones, cross-checked against
its real `HSKN` skeleton `HellBase_Cannon`), knowing *which* 10-byte-keyframe range belongs to
*which* named bone (`Hellbase_Cannon`, `_1`, `_2`, `Attach_Effect_1`, `_3`, `Attach_Effect_2`)
requires understanding the per-bone table well enough to compute per-bone byte offsets within
the keyframe blob. Not done — a brute-force exact-fit search (every combination of per-bone
field widths, requiring the accounted bytes to exactly consume the blob with zero leftover)
found **zero** working combinations for the 6-bone cannon file specifically, which at minimum
rules out the simplest uniform-width-per-bone layouts (see `CLAUDE.md` for the exact search
parameters, not repeated here).

## Hypotheses tried and ruled out (so they aren't retried)

- **`field0` = byte length of the name block.** Wrong — disproven by direct counterexample (see
  "The wrapper" above).
- **`raw/32767 = quaternion component` directly (no delta).** Wrong for `W` — the delta
  reframing (`raw/32767 = W - 1`) is what actually fits. Whether `X`/`Y`/`Z` are also
  delta-encoded (irrelevant if reference is `0`, since `component - 0 = component`) or something
  else entirely is still open (see "Open questions" #1).
- **Uniform-width interleaved-per-bone record** (same field widths for every bone in one file,
  laid out as `[frames][flag][keyframes...]` repeated per bone). Brute-force exact-fit search
  against the known 6-bone cannon file found zero valid combinations.
- **Header-table-then-flat-keyframe-blob** (all bones' `frames` counts up front, then one
  contiguous run of all bones' keyframes with no further per-bone framing). Also brute-forced
  against the cannon file; also zero valid combinations.
- **Blob-wide pairwise unit-quaternion self-consistency scan** (see "Open questions" #2) — not
  wrong exactly, but too noisy against real data to be a useful technique as applied; would need
  a tighter constraint or cleaner sample to be worth retrying in this form.

## Useful sample files (all under a real extracted `h_hellbase.pc`, path relative to
## `test3/files/`)

Self-documenting filenames (encode their own expected motion — useful as ground truth):

| file | known motion |
|---|---|
| `Objects/VFX/Null_Objects/anim/Null_Gizmo01_Rot_90x_30f.anim` | identity → 90° about X, ~30 frames, 1 bone |
| `Objects/VFX/Null_Objects/anim/Null_Gizmo02_Rot_90y_30f.anim` | identity → 90° about Y, ~30 frames, 1 bone — same magnitude as above, different axis, byte-identical per-bone table |
| `Objects/Common/Military/150_Searchlight/Anims/searchlight_150_Body_Tilt_45_Sabotaged.anim` | ~45° tilt (axis unknown; "Sabotaged" in the name means the *exact* angle shouldn't be trusted too precisely) |
| `Objects/Level_Specific/Fortress/fortress_ventilation_fan/Anims/fortress_ventilation_fan_Blades_Big_360.anim` | full 360° spin — contains an exact 180° point, plus values matching the gizmo's own 90°/45° content |
| `Objects/Level_Specific/Fortress/fortress_ventilation_fan/Anims/fortress_ventilation_fan_Blades_Medium_360.anim` | same as above, different fan size — same blob length/keyframe count as the other two |

Multi-bone, cross-checked against a real `HSKN` skeleton (useful once per-bone attribution is
solvable):

| file | skeleton | bones |
|---|---|---|
| `Objects/Props/Level_Specific/HellBase/Anims/HellBase_Cannon_Fire_Right.anim` | `HellBase_Cannon` (6 bones) | `Hellbase_Cannon`, `_1`, `_2`, `Attach_Effect_1`, `_3`, `Attach_Effect_2` |
| `Objects/Props/Level_Specific/HellBase/Anims/HellBase_Cannon_Fire_Left.anim` | same skeleton | same |

Other candidates worth trying next (not yet examined in this investigation, but structurally
promising by name — single-axis hinge-like motions with a plausible known/guessable target
angle):

- `Objects/VFX/ZA_Traps_Blockers/Anim/Null_Door_Open.anim` — likely ~90° about a vertical axis
- `Objects/ZA4/Pickup_Crates/Anim/Explosive_Valve_Tap.anim` — likely a small-angle valve turn
- `Objects/VFX/ZA_Traps_Blockers/Anim/Trap_Trigger_Spinner_rotating.anim` /
  `Trap_Trigger_Spinner_slowtostop.anim` — a continuous spin, similar to the fan blades

The one confirmed non-`HCAN` sample:

- `Objects/VFX/Destruction/Weaponisation/Explosive_Drum/Explosive_Drum_Chunk_9.anim` — ends in a
  `"SDDC"` footer instead of `"HCAN"`. Not investigated.

## Suggested next steps, roughly in order of expected value

1. **Resolve the axis-direction hypothesis** (Hypothesis A vs. B above) using a rigged object
   with a real `HSKN` skeleton and a guessable single-axis motion — `Explosive_Valve_Tap.anim`
   or `Null_Door_Open.anim` are good candidates, since they can be cross-checked against a real
   bind-pose bone rotation, unlike the `Null_Gizmo` test objects which likely have no mesh/
   skeleton at all.
2. **Fully decompose the 10-byte-per-keyframe budget.** Two data points confirmed
   (`W`-delta at 2 bytes, keyframe stride at 10 bytes) leave 8 bytes unaccounted; figuring out
   what fills them (axis component? `t_perc`? both, plus padding?) would likely crack the
   record shape outright.
3. **Decode the remaining 10 bytes of the per-bone table** (currently `01 00 00 00 00 00 00 00
   00 00` in every sample checked — possibly itself constant, possibly not; more samples with
   *different* bone configurations would clarify this).
4. **Investigate the `"SDDC"` footer variant** — separate sub-format, unclear how common or how
   different it is from the `"HCAN"` path documented here.
5. Once 1–3 are solid: write the actual Go parser in `pkg/asura`, following this project's usual
   practice of verifying against real sample data with numeric self-consistency checks (the same
   rigor already applied to `mesh.go`/`skeleton.go`/`rscf.go`) before trusting it.
6. Only after a working parser exists: design the hierarchical (non-`Skin()`) pose-composition
   function, then the glTF `animations` export — see "Why this matters" above for why both of
   those are real, separate pieces of work, not incidental to cracking the bytes.
