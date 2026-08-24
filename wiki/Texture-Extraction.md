# Texture Extraction

Covers the `texture` command: extracting embedded DDS textures from an RSCF archive.

## Background

`.pc_textures` files (e.g. `dlc08.pc_textures`) hold a sequence of `RSCF` entries — unlike
every other chunk type this tool understands, there's no single chunk-level header; each
entry repeats its own `"RSCF"` tag, one after another, until a 4-byte zero footer at the end
of the file (the same footer convention as the `HTXT` symbol-name table and `ASTS` streamsounds
manifest).

Each entry is: the `RSCF` tag, a `uint32` giving that entry's **total byte length** (tag
through the end of its payload), 2 more `uint32` fields of unconfirmed meaning, a `uint32`
**resource-type code**, a `uint32` flags field of unconfirmed meaning, a `uint32` giving the
payload's **exact byte length**, a source-asset path in a 4-byte-chunk-aligned string encoding
(e.g. `\graphics\characters\...\rs16_clothes_ar.tga` — the extension reflects the *original*
art source, not what gets extracted), and finally the payload itself, read directly via the
declared length. The resource-type code was cross-checked against independent reference
implementations of the format: `2` is a texture (the only type this command decodes — see
[Sound Extraction](Sound-Extraction.md) for type `3`, audio, decoded by the `sound` command
instead), `0` is a large category this tool doesn't touch here (see Known limitations), and `6`
is a bare reference to another package with no embedded payload.
Only type-2 entries whose payload actually starts with the DDS magic are decoded as textures;
type mismatches and malformed payloads are skipped, not treated as errors, since the declared
total length keeps the walk in sync with the file regardless — confirmed by walking every one
of 217 entries in a real 763MB Zombie Army 4 sample to an exact, error-free end of file, and by
decoding extracted textures with ImageMagick and `file` (correct dimensions, correct pixel
format, correct pixel content — verified visually, not just structurally).

Both legacy FourCC-tagged DDS (`ATI1`/`ATI2`, i.e. BC4/BC5 — common for normal maps) and the
newer DX10-extended header (BC7, seen for albedo/roughness textures) show up in the sample
file. An earlier version of this parser instead searched for the entry's own `"DDS "` magic
within its declared span rather than trusting the payload-length field directly — that worked
(and, before it, computing a DDS mip-chain size itself worked for the first 165 entries and
then broke on a texture-array-shaped surprise), but was less precise and blind to non-texture
resource types.

## Commands

```text
asr-text-extractor texture unpack <file> [output-dir] [--convert dds|png]
```

Extracts every embedded texture from `<file>`, writing each one to `<output-dir>` at the
relative path recorded in its entry (backslashes normalized, any `.`/`..` component dropped).
Creates subdirectories as needed. If `output-dir` is omitted, it defaults to the input's base
name.

```sh
asr-text-extractor texture unpack dlc08.pc_textures
# -> dlc08/graphics/characters/zap_characters/humans/nza4_partisans_rs16/rs16_clothes_ar.dds
# -> ... (217 files, mirroring the game's own asset paths)
```

It also prints a one-line diagnostic to stderr (`Entries: N`) before extracting.

Like `sound`, this is extract-only: there's no `--format`/`--encoding` (textures aren't a
translatable interchange format) and no repack path yet.

### `--convert dds` (default) vs `--convert png`

`--convert dds` (the default) writes the raw embedded DDS bytes verbatim, with the extension
forced to `.dds` regardless of the manifest path's original source extension.

`--convert png` decodes each texture's pixels (see [pkg/dds](#the-dds-decoder) below) and
writes a lossless PNG instead — no external tools, no plugins, guaranteed to open correctly
anywhere. This is what you want for actually editing a texture in Blender/GIMP/Photoshop
without worrying about DDS codec support. An entry whose pixel format isn't understood is
skipped with a warning (see Known limitations) rather than aborting the whole extraction; the
run ends with a summary line if anything was skipped.

```sh
asr-text-extractor texture unpack --convert png dlc08.pc_textures
# -> dlc08/graphics/.../rs16_clothes_ar.png (decoded, lossless, ready to edit)
```

PNG conversion is noticeably slower than raw extraction (real-world: ~70 seconds for a 217-entry,
~1.1GB-of-PNGs archive) since it fully decodes every texture's pixels and re-compresses them;
raw `--convert dds` extraction of the same archive takes about a second. The encoder is tuned
for speed (`png.BestSpeed`) over minimum file size, which is the right trade for a modding
workflow — about 15% larger files than default compression in exchange for roughly 4x less time.

### The DDS decoder

Texture pixel decoding lives in `pkg/dds`, a small standalone package (no dependency on the
Asura-specific parts of this project) that decodes standard DDS files — both legacy
FourCC-tagged (`ATI1`/`ATI2`, i.e. BC4/BC5 — common for normal maps) and the newer
DX10-extended header (BC7, common for albedo/roughness) — to a plain `image.NRGBA`. BC7's
eight-mode bit-packing and partition tables were ported from the public specification and
cross-checked pixel-for-pixel (100% exact match, zero-diff, across a full 2048x2048 real
texture) against an independent reference decoder before being trusted.

### If a texture looks like colorful noise when you view a raw-extracted `.dds`

That's very likely your viewer, not a bad extraction — about half of a typical archive's
textures use the newer DX10-extended DDS header (BC7), and plenty of DDS viewers/plugins only
support the older plain-FourCC header (the format most normal maps use, `ATI1`/`ATI2`/BC4/BC5).
**XnView is a confirmed example**: its DDS plugin doesn't understand the DX10 extension, so it
renders BC7 textures as static while showing legacy-format ones (mostly normals) correctly —
matching "normals are fine, everything else is broken" exactly. The simplest fix is
`--convert png` above; alternatively cross-check with a tool known to support BC7/DX10:
ImageMagick (`magick file.dds file.png`), an up-to-date GIMP DDS plugin, or Blender's own image
loader (which is what matters for actual modding use, and handles it fine).

## Known limitations

See [games/](games/Zombie-Army-4.md) for per-game verification status — this feature is fully
confirmed working on every title tested so far (Zombie Army 4, Sniper Elite 5, Sniper Elite
Resistance).

- 2 of the 5 per-entry header fields (between the total-size field and the resource-type code)
  and the flags field after it aren't fully understood — one of the 2 was hypothesized to flag
  the asset's original source format (`.tga` vs `.dds`) but that didn't hold up against the
  real sample (109/217 mismatches). They're left unparsed rather than documented with false
  confidence.
- Only resource-type 2 (texture) is decoded. A real level-package sample has 568 resource-type
  0 entries totaling 169MB with plain per-object names (no `\graphics\` path prefix) and
  `l1#`/`l2#`/`l3#`/`l4#`-prefixed variants that shrink in size together — shaped like mesh
  LOD chains, not textures — that this tool doesn't attempt to extract or interpret. See
  [Package Extraction](Package-Extraction.md#known-limitations).
- One real entry in the sample archive (`graphics\specialfx\water\coastal_water_a.dds`) has a
  genuinely non-standard DDS header — different header flags and an unrecognized pixel-format
  FourCC. It still extracts as raw `.dds` (the entry's declared total length doesn't depend on
  understanding the payload), but `--convert png` skips it (with a warning) since `pkg/dds`
  can't decode its pixel format. This looks like a special-cased asset type (procedural/
  flow-map water shader input, going by the path), not a sign that other entries are at risk —
  it's the only one of 217 that didn't decode cleanly.
- `pkg/dds` doesn't implement every DDS pixel format — BC2 (`DXT3`) and BC6H (HDR) aren't
  supported (not seen in any sample archive so far); `--convert png` skips entries in those
  formats with a warning rather than guessing. Uncompressed formats are supported only at
  32 bits/pixel.
- Extract-only: no way to repack an edited/replacement DDS back into an archive yet (true of
  every asset type except text/voice strings — see [Home](Home.md#planned-not-yet-implemented)).
- Only tested against one real archive (217 entries, BC4/BC5/BC7 formats, no texture arrays or
  cubemaps observed). The size-field-driven approach should generalize, but array/cubemap
  textures (`arraySize > 1` in a DX10 header) haven't been seen in a real sample yet.
