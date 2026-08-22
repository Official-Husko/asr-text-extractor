# Package Extraction

Covers the `package` command: extracting manifest-referenced sub-files and embedded textures
from an `AsuraZbb`-compressed level-package file (`.pc`, `.pc_entdata`).

## Background

A level's `.pc`/`.pc_entdata` files are wrapped in a distinct compression format signed
`"AsuraZbb"` (not the plain `"Asura   "` magic every other chunk type in this tool understands).
The wrapper is: the 8-byte magic, an unused `uint32`, a `uint32` giving the total decompressed
size, then repeating `[compressedSize uint32][decompressedSize uint32][zlib-compressed bytes]`
chunks (each 2MB decompressed except the last) until the declared total is reached. Confirmed
byte-for-byte against a real 255MB Zombie Army 4 level package (226 chunks, exact declared
total, exact end-of-file landing). Decompressing yields a normal `"Asura   "`-signed buffer.

That decompressed buffer starts with an `FNFO`-tagged header followed by an `RSFL` manifest —
a flat table of sub-files bundled into the package (`.anim`, `.pfx`, `.snd`, `.nav`, `.cut`,
`.ent`, and more), each recorded as a NUL-terminated path plus a `uint32` offset and `uint32`
size. Two things about this manifest took real sample data to pin down correctly:

- **Entry offsets are relative to the end of the `RSFL` section itself**, not the start of the
  package's decompressed content — confirmed by real entries only decoding cleanly (a small
  count field followed by the sub-file's own name repeated verbatim) once that base is added.
- **The padding between a path's NUL terminator and its offset field pads to a fixed 4-byte
  file-absolute alignment**, not "skip every zero byte present." An earlier implementation did
  the latter and silently mis-parsed 2 of 282 entries in a real sample — both had an offset
  field whose own low byte happened to be zero, which a greedy zero-byte scan mistook for one
  more byte of padding, shifting every field after it (offset, size, and the next entry's own
  path) by one byte. `pkg/asura/package_test.go`'s
  `TestParseRSFLManifestOffsetWithZeroLeadByte` regression-tests this exact scenario.

Between the end of the manifest and the manifest's own referenced sub-files sits a long run of
further tagged sections — geometry/spatial data (`PBRV`, `SDPH`), a small unidentified one
(`IRTX`), and many section types that turn out to be per-level-object records (`CONA` entity
transforms, `SDSM`/`SDEV` spatial data, `HSKN`/`HSKL`/`HSBB`/`HSKE`/`HMPT` skeleton/hitbox data,
`FAAN`/`TXAN` animation references, and more). Every one of these — including `RSFL` and `FNFO`
themselves — shares the same generic framing: a 4-byte uppercase-ASCII tag followed by a
`uint32` giving the section's total length counted from the tag's own start, which is enough to
skip over a section without understanding its contents.

**`RSCF` texture entries are interleaved one at a time throughout this run**, not packed into
their own contiguous block — confirmed against a real sample where the very first `RSCF`-tagged
section immediately after the manifest turned out to be a single bare resource reference (no
embedded texture, just a self-referential path) followed directly by ~2,200 unrelated `CONA`
entity records, with the real texture entries starting much further in and each one followed by
its own unrelated section rather than by another texture. Extraction therefore walks every
section generically and decodes each one tagged `RSCF` as a possible texture inline (same
per-entry decoder as standalone [`RSCF` texture archives](Texture-Extraction.md): search for the
embedded `"DDS "` magic within the entry's declared span). In a real 473MB decompressed sample:
3,071 `RSCF`-tagged sections, 2,502 of which embed a texture — matching an independent
whole-file search for `"DDS "` exactly.

The internal layout of `PBRV` (a geometry/mesh block, several megabytes in a typical level) has
not been reverse-engineered and isn't parsed — it's only skipped over via its declared length,
same as every other unidentified section. **Model/mesh extraction is not implemented.**

## Commands

```text
asr-text-extractor package unpack <file> [output-dir] [--convert dds|png]
```

Extracts:

- every manifest-referenced sub-file, to `<output-dir>/files/<relative-path>` (backslashes
  normalized, any `.`/`..` component dropped) — raw bytes, format not otherwise interpreted
- every embedded texture found among the package's tagged sections, to
  `<output-dir>/textures/<relative-path>.dds` (or `.png` with `--convert png`) — same decoding
  and `--convert` behavior as the standalone [`texture`](Texture-Extraction.md) command

If `output-dir` is omitted, it defaults to the input's base name. Creates subdirectories as
needed, and prints a one-line diagnostic to stderr (`Entries: N  Textures: N`) before
extracting.

```sh
asr-text-extractor package unpack h_hellbase.pc
# -> h_hellbase/files/LevelExportTemp0/ZA/Dust/ZA4_Mist_UnderLights_Small.pfx
# -> h_hellbase/textures/graphics/za4/rocks/scan_rock_cluster_01_ar.dds
# -> ... (282 files, 2502 textures)

asr-text-extractor package unpack h_hellbase.pc_entdata
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.snd
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.nav
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.cut
# -> h_hellbase.pc_entdata/files/LevelExportTemp0/HellBase.ent
# (no textures section in this file — Textures: 0)
```

Like `sound` and `texture`, this is extract-only: there's no `--format`/`--encoding` and no
repack path yet.

## Known limitations

- `PBRV` (geometry/mesh data) and every other non-`RSCF` tagged section between the manifest
  and its sub-files are skipped, not decoded — model/mesh extraction is a planned, separate
  effort (see [Home](Home.md#planned-not-yet-implemented)).
- The manifest entry's trailing `uint32` field (after offset and size) is always `1` across
  every entry in the real samples tested; its meaning beyond that isn't confirmed.
- `.snd`, `.cut`, and `.ent` sub-files extract as raw, uninterpreted bytes — no attempt is made
  to further parse their internal structure (`.nav`, by contrast, starts with a recognizable
  `WPSG`-tagged section, also unparsed). `.anim`/`.pfx` sub-files decode to a small count field
  followed by the sub-file's own name, which isn't parsed further either.
- Only tested against one real `.pc` (473MB decompressed, 282 sub-files, 2,502 textures) and one
  real `.pc_entdata` (no embedded textures) sample, both from the same Zombie Army 4 level.
- Extract-only: no way to repack an edited sub-file or texture back into a package yet (true of
  every asset type except text/voice strings — see
  [Home](Home.md#planned-not-yet-implemented)).
