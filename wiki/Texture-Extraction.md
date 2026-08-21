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

## Known limitations

- The 20 bytes of per-entry fields between the total-size field and the path aren't fully
  understood. One of them was hypothesized to flag the asset's original source format
  (`.tga` vs `.dds`) but that didn't hold up against the real sample (109/217 mismatches) —
  it's left unparsed rather than documented with false confidence.
- Extract-only: no way to repack an edited/replacement DDS back into an archive yet (true of
  every asset type except text/voice strings — see [Home](Home.md#planned-not-yet-implemented)).
- Only tested against one real archive (217 entries, BC4/BC5/BC7 formats, no texture arrays or
  cubemaps observed). The size-field-driven approach should generalize, but array/cubemap
  textures (`arraySize > 1` in a DX10 header) haven't been seen in a real sample yet.
