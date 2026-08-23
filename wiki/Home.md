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
  textures, and meshes (as binary glTF by default — a real, riggable armature for meshes with a
  matching skeleton, e.g. a rifle's bolt assembly, see
  [Skinning](Package-Extraction.md#exporting-a-real-riggable-armature-gltf-the-default) — or
  Wavefront OBJ via `--mesh-format obj`) from `AsuraZbb`-compressed level packages (`.pc`,
  `.pc_entdata`) (extract-only, no repacking yet).

## Planned (not yet implemented)

- Mesh normals — not decoded, so neither export format has shading data; Blender's own
  recalculate-normals fills the gap. See
  [Package Extraction](Package-Extraction.md#known-limitations).
- Importing the game's own animation data (`.anim` files aren't reverse-engineered) — the glTF
  export carries the bind-pose rig itself (individually posable bones), not any actual game
  animation. The unrelated `PBRV` geometry/spatial-data section found inside level packages
  isn't reverse-engineered at all.
- A second, smaller sound chunk type (`FNFO`) seen in a sample `gmsnd.asr_wav_en` file —
  not yet reverse-engineered (see [Sound Extraction](Sound-Extraction.md#known-limitations))
- Recompiling/repacking beyond text overrides (phase 2) — this includes repacking sound,
  texture, and model assets once their formats are understood

Each of these gets its own wiki entry once it's implemented.
