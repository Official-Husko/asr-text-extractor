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
| `sound` — streamsounds manifest (`ASTS`) | ❌ **Confirmed broken.** A real sample (`misc/common.asr_wav_en.pc.streamsounds`) fails with an out-of-range audio offset (`asura: ASTS entry 84 (...): audio range [1140892985:402695658] outside file`) — the manifest's own offset/size field layout appears to have changed from Zombie Army 4's. |
| `sound` — RSCF audio archive (`.pc.sounds`) | ✅ Verified working — 266 real WAV entries extracted from `chars/mp.pc.pc.sounds`. |
| `texture` (`.pc_textures` archives, and via `package unpack`) | ✅ Verified working — 4,077 real textures extracted from a DLC level package, 4,076 successfully converted to PNG. |
| `package` — sub-file/texture extraction | ✅ Sub-file and texture extraction both work correctly. |
| `package` — mesh decoding | ✅ **Fixed.** Was confirmed broken (0 of 1,613 real mesh-candidate entries decoded), then fixed — see below. A real DLC level package now decodes 1,565 of those 1,613 entries into real geometry; the remaining 48 were individually checked and are provably not meshes at all: 45 are all-zero 16-byte placement/marker/proxy stubs (verified by hex dump — `null_searchlight_emplacement`, `null_guipromptmarker_01`, `*_compoundproxy`, and similar), and 3 are unrelated bulk data blobs (`"inst (dynamic)"`/`"inst (static)"`, the same category Zombie Army 4 has, plus one Sniper-Elite-5-specific `"Env"` blob) whose header fields don't remotely reconcile with their real payload size. |
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

## Why streamsounds (`ASTS`) extraction fails

The `ASTS` manifest's per-entry offset/size fields (see [Sound
Extraction](../Sound-Extraction.md)) decode to values wildly outside the file's own bounds on a
real Sniper Elite 5 sample. Zombie Army 4's `ASTS` files were the only ones this format's field
layout was ever confirmed against — the RSCF-based `.pc.sounds` audio container (see above)
appears to be a more reliable, working alternative for extracting audio from Sniper Elite 5
files where one is available.
