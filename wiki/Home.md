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
  textures, and meshes (as Wavefront OBJ, skinned against a matching skeleton when one exists —
  see [Skinning](Package-Extraction.md#skinning-multi-part-meshes)) from
  `AsuraZbb`-compressed level packages (`.pc`, `.pc_entdata`) (extract-only, no repacking yet).

## Planned (not yet implemented)

- Mesh normals — not decoded, so OBJ export relies on Blender's own recalculate-normals for
  shading. See [Package Extraction](Package-Extraction.md#known-limitations).
- Exporting a mesh's skeleton itself (only the already-skinned, bind-pose geometry is exported
  today — OBJ has no way to carry a skeleton/vertex-groups for re-posing in Blender; a richer
  format like glTF would be needed). The unrelated `PBRV` geometry/spatial-data section found
  inside level packages isn't reverse-engineered at all.
- A second, smaller sound chunk type (`FNFO`) seen in a sample `gmsnd.asr_wav_en` file —
  not yet reverse-engineered (see [Sound Extraction](Sound-Extraction.md#known-limitations))
- Recompiling/repacking beyond text overrides (phase 2) — this includes repacking sound,
  texture, and model assets once their formats are understood

Each of these gets its own wiki entry once it's implemented.
