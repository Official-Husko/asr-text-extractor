# asr-text-extractor Wiki

Feature-by-feature documentation for using `asr-text-extractor`. The [README](../README.md) is
a short pointer; this is where the actual detail lives.

## Entries

- [Text & Voice Extraction](Text-and-Voice-Extraction.md) — unpacking, translating, and
  repacking `HTXT` (menu/UI) and `DLLN` (voice line) strings; the `--format`/`--encoding`
  interchange system (JSON, YAML, XML, CSV, txt).
- [Sound Extraction](Sound-Extraction.md) — extracting embedded WAV assets from `ASTS`
  streamsounds manifests (extract-only, no repacking yet).
- [Texture Extraction](Texture-Extraction.md) — extracting embedded DDS textures from `RSCF`
  archives (extract-only, no repacking yet).
- [Package Extraction](Package-Extraction.md) — extracting manifest sub-files, embedded
  textures, and meshes (as Wavefront OBJ) from `AsuraZbb`-compressed level packages (`.pc`,
  `.pc_entdata`) (extract-only, no repacking yet).

## Planned (not yet implemented)

- Skinned/rigged mesh support — mesh vertex data beyond position and UV (normals, skin
  weights, bone indices) isn't decoded yet, and there's no skeleton (`HSKN`) importer, so
  meshes always export as static geometry even when the source data is rigged. See
  [Package Extraction](Package-Extraction.md#meshes). The unrelated `PBRV`
  geometry/spatial-data section found inside level packages isn't reverse-engineered at all.
- A second, smaller sound chunk type (`FNFO`) seen in a sample `gmsnd.asr_wav_en` file —
  not yet reverse-engineered (see [Sound Extraction](Sound-Extraction.md#known-limitations))
- Recompiling/repacking beyond text overrides (phase 2) — this includes repacking sound,
  texture, and model assets once their formats are understood

Each of these gets its own wiki entry once it's implemented.
