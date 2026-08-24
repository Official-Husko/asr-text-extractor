# Sniper Elite 5

An in-progress secondary target. The underlying `"Asura   "`/`"AsuraZbb"` container format is
shared with Zombie Army 4, and every feature now carries over correctly, including mesh
decoding after a real, confirmed engine-revision difference was found and fixed (see below).
Everything below reflects direct testing against a real Sniper Elite 5 installation, not
assumption.

## Status

| Feature | Status |
|---|---|
| `text` (unpack) | ✅ Verified working against real `HTXT` files (e.g. `text/PC/ACHIEVEMENTS/achievements.asr_en`, 156 entries) |
| `voice` (unpack) | ⚠️ Not exhaustively verified — the one real file tried had zero `DLLN` entries, which may just mean that particular file carries no voice data rather than a format problem. The scanning logic is identical to Zombie Army 4's; no reason to expect it doesn't work, but it hasn't been confirmed against a sample known to contain voice lines. |
| `sound` — streamsounds manifest (`ASTS`) | ✅ **Fixed.** Was confirmed broken (a real sample failed with an out-of-range audio offset at entry 84), then fixed — see below. All 15,509 entries in that same real sample (`misc/common.asr_wav_en.pc.streamsounds`) now extract to valid, playable WAV files. Double-checked against a real Sniper Elite Resistance install too (a separate title): 425/448 real files extract directly, and the remaining 23 (reference-only `.ssm` manifests) each have a working sibling file with the same content — 100% of real embedded audio reachable. |
| `sound` — RSCF audio archive (`.pc.sounds`) | ✅ Verified working — 266 real WAV entries extracted from `chars/mp.pc.pc.sounds`. |
| `texture` (`.pc_textures` archives, and via `package unpack`) | ✅ Verified working — 4,077 real textures extracted from a DLC level package, 4,076 successfully converted to PNG. |
| `package` — sub-file/texture extraction | ✅ Sub-file and texture extraction both work correctly. |
| `package` — mesh decoding | ✅ **Fixed.** Was confirmed broken (0 of 1,613 real mesh-candidate entries decoded), then fixed — see below. A real DLC level package now decodes 1,565 of those 1,613 entries into real geometry; the remaining 48 were individually checked and are provably not meshes at all: 45 are all-zero 16-byte placement/marker/proxy stubs (verified by hex dump — `null_searchlight_emplacement`, `null_guipromptmarker_01`, `*_compoundproxy`, and similar), and 3 are unrelated bulk data blobs (`"inst (dynamic)"`/`"inst (static)"`, the same category Zombie Army 4 has, plus one Sniper-Elite-5-specific `"Env"` blob) whose header fields don't remotely reconcile with their real payload size. |
| `package` — embedded mesh textures/materials | ✅ **Fixed.** Was confirmed broken (every exported `.glb` had 0 embedded materials, despite geometry and skinning both working) — see below. A real DLC level package now embeds a real material into 51.1% of exported `.glb` files (up from 3.2% before the fix), with the remainder a genuine, expected gap (see below), not a bug. |
| `scan` | ✅ Runs correctly and reports accurate counts for everything above. |
| `.mod` sub-file | 🔍 Structurally different from — and far richer than — Zombie Army 4's. See `research/mod.md` for a substantial partial crack: a real mesh format (vertex + quad-index sections) plus a named `"FXPT"` effect-point marker. Not implemented in Go yet. |

## Mesh decoding: the fix

`pkg/asura/mesh.go`'s vertex/group/index layout was originally reverse-engineered specifically
against Zombie Army 4 samples, and Sniper Elite 5's real resource-type-0 `RSCF` entries — present
in large numbers in real level packages — didn't match it, so `ParseMesh`'s size-reconciliation
check correctly rejected all of them rather than producing garbled geometry silently.

Byte-level investigation against 5 real samples (`weldingkit_cylinder_tall_red` and its 3 LOD
variants, plus `machine_coldblast_engine_trunnion_little`) found the payload size was always off
by exactly 4 bytes from the Zombie Army 4 prediction — and tracking that down found the real
difference: **Sniper Elite 5's trailing position-offset field is 2 float32 values (X, Y) instead
of 3 (X, Y, Z)** — Z has no stored offset at all, dequantizing to exactly 0. Everything else
(the 5-field header, 24-byte group records, 48-byte vertex stride, `uint16` index buffer) is
identical to Zombie Army 4's. Confirmed exactly (to the byte, including the triangle-index
buffer landing precisely at each payload's own end) across all 5 samples.

`ParseMesh` now tries the Zombie Army 4 (3-float offset) layout first and falls back to the
Sniper Elite 5 (2-float offset) layout only if the first doesn't reconcile the payload's exact
declared size — the same size-reconciliation check that originally caught the incompatibility is
what now safely picks the right one, with no separate "which game is this" flag needed anywhere.
Validated two ways so far: the exact-byte size match across all 5 hand-picked samples, and the
same project-established numeric check used for the original Zombie Army 4 mesh crack
(bounding-box plausibility and triangle-edge-length-vs-bounding-box ratio, checked across many
real decoded objects — LOD variants of the same object come out with near-identical bounding
boxes, and fracture/destruction "chunk" variants of the same object share identical edge-length
statistics). A real exported `.glb` (`weldingkit_cylinder_tall_red`) has been sent to the user
for the same kind of direct visual check in Blender that confirmed the original Zombie Army 4
mesh/skinning work — not yet confirmed one way or the other as of this writing.

## Embedded textures/materials: the fix

Geometry and skinning both worked (see above), but every exported `.glb` from a real Sniper
Elite 5 package had **zero** embedded materials — `pkg/asura`'s `MeshGroup.Hash` (the mesh's own
declared material identifier) is uniformly `0` across every Sniper Elite 5 mesh checked, so it
carries no per-object information in this title at all, and the original texture-matching
heuristic (exact folder-segment match — see [Package
Extraction](../Package-Extraction.md#embedded-textures-gltf-the-default)) found essentially
nothing: a direct survey of 1,565 real decoded meshes against 4,077 real textures in one DLC
package found only 107 (6.8%) with an exact folder match.

Investigation found real Sniper Elite 5/Resistance samples use two texture-organization
patterns Zombie Army 4's own heuristic never needed: many unrelated objects' maps lumped into
one generically-named folder with the specific object identifiable only by filename (e.g.
`graphics\pickups\pickup_crate_explosives_ar.png`), and several sub-part meshes of one larger
object (e.g. `german_heavy_truck_door_right`, `german_heavy_truck_grill_left`) sharing one
parent-object texture set whose own filenames use an unrelated vocabulary for the specific part
(`cab`, `container`, `interior` — never `door` or `grill`). A third finding: Sniper Elite 5's
own dominant color-map suffix is `_ar`/`_albedoroughness` (packed albedo+roughness), which the
original suffix classifier didn't recognize as albedo at all — so even a perfect name match
wouldn't have embedded anything until that was fixed too.

`meshTextures` (`internal/cli/package.go`) now tries progressively shorter candidate names
(stripping one trailing `_word` at a time) against both texture folder segments and texture
filenames (role suffix removed), and `textureRole` now classifies `_ar`/`_albedoroughness` as
albedo (safe: glTF's `baseColorTexture` only ever reads a texture's RGB, never its alpha, here —
unlike the still-unmatched `_m`/metallic suffix, there's no risk of misapplying a packed
channel). Verified end-to-end on real exported `.glb` files (not just a survey script): real
material-embedding coverage went from 23/722 (3.2%) to 369/722 (51.1%) of exported files on a
real DLC package. Spot-checked several newly-matched objects for false positives — all
confirmed as genuine, sensible matches (e.g. `ammo_box_50cal` correctly finding
`ammo_box_50cal_albedoroughness.png`/`_n.png` in a generically-named `pbr_converted` folder via
the new filename-stem matching). **Zero regression on Zombie Army 4** — confirmed via a direct
before/after comparison against the same real level package — and, unexpectedly, a real
improvement there too (3.3% → 38.2% coverage on the same `h_hellbase.pc` sample): a large
fraction of Zombie Army 4's own props turned out to follow the same generic-folder-plus-filename
convention the original, narrower heuristic never checked for.

The remaining ~49% gap is real and expected, not a bug: objects whose actual texture uses a name
sharing no word at all with the mesh (e.g. `weldingkit_cylinder_tall_red`, whose real texture
folder is named `welding_tank_01`) have no way to be found by any name-based heuristic, and a
character mesh like `se5_german_kriegsmarine` has no matching texture anywhere in this specific
DLC package at all — its skin likely lives in a separate, shared character-asset package, the
same kind of split-package situation Zombie Army 4's own streaming-texture-pool architecture
already documents (see [Package
Extraction](../Package-Extraction.md#on-resolution)).

## Streamsounds (`ASTS`) extraction: the fix

`pkg/asura/asts.go`'s `ParseASTS` originally padded each manifest entry's path by skipping every
zero byte after its NUL terminator until it hit a non-zero one — the same bug class already found
and fixed once before in this codebase for `RSFL` manifest parsing (see
[Package Extraction](../Package-Extraction.md)). That greedy skip silently over-consumes into the
entry's own `size`/`offset` fields whenever one of those `uint32` values happens to start with a
zero byte itself — a roughly 1-in-256 coincidence per field, so rare enough to never trigger
across Zombie Army 4's much smaller `ASTS` sample files but effectively guaranteed to hit
somewhere in a real Sniper Elite 5 sample with 15,509 entries (it did, at entry 84, decoding a
garbage offset/size pair from a real, valid entry).

Extracting the raw manifest bytes directly and testing candidate padding rules against the file's
own internal consistency (every entry's `offset + size` must stay within the file) found the real
rule: padding is **not** "skip to the next non-zero byte" but a fixed alignment, always landing
the `size` field at the smallest position where `(pathLength + 1 + padding) ≡ 1 (mod 4)` measured
from the path's own start — i.e. `padding = (4 - (pathLength + 1) % 4) % 4 + 1`, always 1 to 4
bytes, never 0. Verified exhaustively, not just spot-checked: all 15,509 real entries in the
Sniper Elite 5 sample decode cleanly under this formula, and so do all 581 entries in an
independent Zombie Army 4 sample — confirming this was always a latent bug affecting both
titles' `ASTS` files, not a genuine per-game format difference, simply never triggered by Zombie
Army 4's smaller sample data. One formula, no per-game branching needed.

Double-checking the fix against a real Sniper Elite Resistance install (a separate title from the
sample used to derive the formula) surfaced one more real wrinkle: 23 of that install's 448 real
`.streamsounds` files still failed — but every one of them was a `.ssm`-named file (e.g.
`sounds/ss_alps.ssm.pc.streamsounds`), and none of their entries reconciled against the file's own
size *no matter what padding was tried* — a different symptom from the original bug, which always
had a padding value that worked. Investigating found a real, confirmed flag byte right after the
header (before the first entry's path): `0` in every self-contained sample (including all 425 of
Resistance's non-`.ssm` files), `1` in every one of the 23 `.ssm` files — and in the equivalent
`.ssm.*.streamsounds` files found in Sniper Elite 5 (28 of them) and Zombie Army 4 (49 of them)
too, 100 files total, no other value ever seen. A flag-1 file isn't a self-contained audio
container at all — it's a lightweight companion manifest. Every one has a sibling
`<name>.asr.*.streamsounds` file (flag 0) with the *exact same* asset paths: confirmed directly
(`ss_alps.ssm.pc.streamsounds`'s 7 paths are byte-identical to `ss_alps.asr.pc.streamsounds`'s 7
paths, and the sibling extracts to 7 real, valid WAVs) and confirmed structurally for all 100
`.ssm` files across all three titles (every one has a matching `.asr` sibling). Nothing is
actually unextractable — `ParseASTS` now detects flag 1 immediately and reports a specific,
actionable error instead of failing deep inside entry parsing with a confusing out-of-range
message. Verified end-to-end against the full real Resistance install: of all 448
`.streamsounds` files, 425 extract directly and the remaining 23 each have a working sibling that
does — 100% of real embedded audio in the install is reachable.
