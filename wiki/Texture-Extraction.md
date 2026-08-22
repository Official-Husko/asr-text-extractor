# Texture Extraction

Covers the `texture` command: extracting embedded DDS textures from an RSCF archive.

## Background

`.pc_textures` files (e.g. `dlc08.pc_textures`) hold a sequence of `RSCF` entries — unlike
every other chunk type this tool understands, there's no single chunk-level header; each
entry repeats its own `"RSCF"` tag, one after another, until a 4-byte zero footer at the end
of the file (the same footer convention as the `HTXT` symbol-name table and `ASTS` streamsounds
manifest).

Each entry is: the `RSCF` tag, a `uint32` giving that entry's **total byte length** (tag
through the end of its texture data), 20 more bytes of fields whose exact meaning isn't fully
confirmed, a NUL-terminated ASCII source-asset path (e.g.
`\graphics\characters\...\rs16_clothes_ar.tga` — the extension reflects the *original* art
source, not what gets extracted), some zero padding, and then the texture itself: a complete,
standard **DDS** (DirectDraw Surface) file, byte-for-byte identical to what any DDS-aware tool
expects — confirmed by walking every one of 217 entries in a real 763MB Zombie Army 4 sample to
an exact, error-free end of file, and by decoding extracted textures with ImageMagick and `file`
(correct dimensions, correct pixel format, correct pixel content — verified visually, not just
structurally).

Both legacy FourCC-tagged DDS (`ATI1`/`ATI2`, i.e. BC4/BC5 — common for normal maps) and the
newer DX10-extended header (BC7, seen for albedo/roughness textures) show up in the sample
file. Extraction doesn't need to understand either — it locates the entry's own `"DDS "` magic
and trusts the entry's declared total length to know where the texture data ends, rather than
computing a DDS mip-chain size itself (an earlier approach that worked for the first 165
entries and then broke on a texture-array-shaped surprise — the size field turned out to be
the right tool for the job all along).

## Commands

```text
asr-text-extractor texture unpack <file> [output-dir]
```

Extracts every embedded DDS from `<file>`, writing each one to `<output-dir>` at the relative
path recorded in its entry (backslashes normalized, any `.`/`..` component dropped, and the
extension forced to `.dds` regardless of the original source extension — the extracted bytes
are always DDS data). Creates subdirectories as needed. If `output-dir` is omitted, it
defaults to the input's base name.

```sh
asr-text-extractor texture unpack dlc08.pc_textures
# -> dlc08/graphics/characters/zap_characters/humans/nza4_partisans_rs16/rs16_clothes_ar.dds
# -> ... (217 files, mirroring the game's own asset paths)
```

It also prints a one-line diagnostic to stderr (`Entries: N`) before extracting.

Extracted `.dds` files open directly in Blender's image editor, GIMP, Photoshop (with a DDS
plugin), or any other DDS-aware tool — no conversion step needed.

Like `sound`, this is extract-only: there's no `--format`/`--encoding` (textures aren't a
translatable interchange format) and no repack path yet.

### If a texture looks like colorful noise when you view it

That's very likely your viewer, not a bad extraction — about half of a typical archive's
textures use the newer DX10-extended DDS header (BC7), and plenty of DDS viewers/plugins only
support the older plain-FourCC header (the format most normal maps use, `ATI1`/`ATI2`/BC4/BC5).
**XnView is a confirmed example**: its DDS plugin doesn't understand the DX10 extension, so it
renders BC7 textures as static while showing legacy-format ones (mostly normals) correctly —
matching "normals are fine, everything else is broken" exactly. Before assuming a bad
extraction, cross-check with a tool known to support BC7/DX10: ImageMagick
(`magick file.dds file.png`), an up-to-date GIMP DDS plugin, or Blender's own image loader
(which is what matters for actual modding use, and handles it fine).

## Known limitations

- The 20 bytes of per-entry fields between the total-size field and the path aren't fully
  understood. One of them was hypothesized to flag the asset's original source format
  (`.tga` vs `.dds`) but that didn't hold up against the real sample (109/217 mismatches) —
  it's left unparsed rather than documented with false confidence.
- One real entry in the sample archive (`graphics\specialfx\water\coastal_water_a.dds`) has a
  genuinely non-standard DDS header — different header flags and an unrecognized pixel-format
  FourCC. It still extracts (the entry's declared total length doesn't depend on understanding
  the payload), but the resulting file may not be valid DDS. This looks like a special-cased
  asset type (procedural/flow-map water shader input, going by the path), not a sign that
  other entries are at risk — it's the only one of 217 that didn't decode cleanly.
- Extract-only: no way to repack an edited/replacement DDS back into an archive yet (true of
  every asset type except text/voice strings — see [Home](Home.md#planned-not-yet-implemented)).
- Only tested against one real archive (217 entries, BC4/BC5/BC7 formats, no texture arrays or
  cubemaps observed). The size-field-driven approach should generalize, but array/cubemap
  textures (`arraySize > 1` in a DX10 header) haven't been seen in a real sample yet.
