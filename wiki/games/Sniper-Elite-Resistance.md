# Sniper Elite Resistance

An in-progress secondary target, and closely related to Sniper Elite 5 — the two titles share a
large amount of asset data verbatim (confirmed directly: Sniper Elite 5's and Sniper Elite
Resistance's own `3D_Frontend.mod` files are byte-identical for their first ~176 bytes, and
Resistance's `SE5\WeaponEffects\...` asset path references point at Sniper Elite 5's own asset
namespace — see `research/mod.md`). Expect its support profile to closely track [Sniper Elite
5](Sniper-Elite-5.md)'s, though not everything below has been independently spot-checked against
a Resistance sample specifically — that distinction is called out per row.

## Status

| Feature | Status |
|---|---|
| `text` (unpack) | ✅ Verified working directly — real `HTXT` files (e.g. `text/pc/collectibles_dlc/COLLECTIBLES_DLC.asr_en`, 200 entries; `text/pc/credits/credits.asr_en`, 11 entries). |
| `voice` (unpack) | ⚠️ Not independently tested against Resistance. Presumed to behave like Sniper Elite 5 (same caveat: not confirmed against a sample known to contain voice lines). |
| `sound` — streamsounds manifest (`ASTS`) | ⚠️ Not independently tested against Resistance. [Sniper Elite 5](Sniper-Elite-5.md) is confirmed broken for this container; Resistance almost certainly shares the same manifest layout, but this hasn't been directly verified with a Resistance sample. |
| `sound` — RSCF audio archive (`.pc.sounds`) | ⚠️ Not independently tested against Resistance. Works on Sniper Elite 5; presumed to work here too given the shared asset format. |
| `texture` (`.pc_textures` archives, and via `package unpack`) | ✅ Verified working directly — 1,680 real textures extracted from `envs/3d_frontend/m_3d_frontend.pc`. |
| `package` — sub-file/texture extraction | ✅ Verified working directly. |
| `package` — mesh decoding | ✅ **Fixed**, directly verified: after the fix described in [Sniper Elite 5](Sniper-Elite-5.md#mesh-decoding-the-fix) (was `Meshes: 0`, confirmed broken), a real level package (`envs/dlc_clearing/m_dlc_clearing.pc`) now decodes real geometry — including objects that are byte-identical to Sniper Elite 5's own (e.g. `german_heavy_truck_door_right`), consistent with the two titles' shared asset data. |
| `scan` | ✅ Runs correctly. |
| `.mod` sub-file | 🔍 Structurally near-identical to Sniper Elite 5's own richer format (see `research/mod.md`) — same mesh+`"FXPT"`-marker shape, not implemented in Go yet. |

## Why this page leans on Sniper Elite 5's findings

Given how closely the two titles' asset data matches (see above), most of Sniper Elite 5's
findings are expected to carry over directly rather than needing independent re-discovery. Rows
marked "not independently tested" above are exactly that — genuinely untested, not silently
assumed — flagged this way rather than merged into a blanket "same as Sniper Elite 5" claim, so
a future session knows exactly what still needs direct verification.
