# Zombie Army 4

The primary development and test target. Every feature in this tool has been verified against
real Zombie Army 4 game files, across many samples spanning the base game and DLC.

## Status

| Feature | Status |
|---|---|
| `text` (unpack/override/compare) | ✅ Fully working |
| `voice` (unpack/override) | ✅ Fully working (Version 4 entries only for override — see [Text & Voice Extraction](../Text-and-Voice-Extraction.md)) |
| `sound` — streamsounds manifest (`ASTS`) | ✅ Fully working |
| `sound` — RSCF audio archive (`.pc.sounds`) | ✅ Fully working |
| `texture` (`.pc_textures` archives) | ✅ Fully working, both `.dds` and `--convert png` |
| `package` — sub-file/texture extraction | ✅ Fully working |
| `package` — mesh decoding | ✅ Fully working, including skeleton-driven glTF armatures |
| `scan` | ✅ Fully working |

## Notes

- Mesh and skeleton decoding (`pkg/asura/mesh.go`, `pkg/asura/skeleton.go`) were built and
  validated entirely against Zombie Army 4 samples — this is the format every other supported
  title's mesh support is compared against (see [Sniper Elite 5](Sniper-Elite-5.md) and
  [Sniper Elite Resistance](Sniper-Elite-Resistance.md), where it does **not** carry over).
- A real level package (`h_hellbase.pc`, 473MB decompressed) was used throughout development as
  the primary stress-test sample: 282 manifest sub-files, 2,502 textures, 550 meshes. See
  [Package Extraction](../Package-Extraction.md) for the full technical write-up.
- Both audio container shapes (`ASTS` streamsounds manifests, and standalone RSCF `.pc.sounds`
  archives with embedded audio) are confirmed working with real, playable WAV output.
- Several manifest sub-file types bundled inside `.pc`/`.pc_entdata` packages
  (`.anim`, `.pfx`, `.snd`, `.cut`, `.ent`, `.sky`, `.gi`, `.fsx`) extract as raw bytes only —
  their internal formats aren't understood yet. See `research/` in the repository for ongoing
  investigation notes (`.nav`'s `WPSG` section and `.cut`+`.ent`'s entity-transform data are the
  furthest along).
- `.mod` — present in exactly one file in the entire game install (`3d_frontend.pc`), a tiny
  24-byte opaque stub. Not understood; see `research/mod.md`.
