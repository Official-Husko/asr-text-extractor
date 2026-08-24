# Scan

Covers the `scan` command: walking a whole folder and listing what's inside every recognized
Asura file by name, without extracting any data.

## Background

A real game installation mixes many thousands of ordinary files (executables, videos, shader
caches, ...) with the handful of container formats this tool understands (`HTXT` text, `DLLN`
voice, `ASTS` sound manifests, `RSCF` texture archives, and `AsuraZbb`-compressed level
packages). Fully unpacking everything just to see what's *in* it — before deciding what's
actually worth extracting — would take far more time and disk space than needed: a single real
level package alone decompresses to several hundred megabytes and contains thousands of named
entries.

`scan` instead walks a folder recursively and writes a plain-text tree: real subdirectories and
files as usual, but for every file it recognizes, an extra nested list of that file's own entry
*names* — manifest sub-file paths, texture/mesh paths, `HTXT` string-table hashes, `DLLN` voice
command names, `ASTS` sound asset paths. No entry bytes are ever read out or written anywhere,
only the names — cheap enough to run across an entire install in one pass, and useful for
grepping through a game's structure before committing to a full unpack of any one file.

Detection reads only the first 12 bytes of a file (magic plus, for a plain `"Asura   "`-signed
file, its first chunk tag) — a file that doesn't match is left as a plain, childless entry
without ever reading the rest of it, so a mixed install's many-gigabyte video/executable files
don't slow the scan down. `AsuraZbb`-wrapped packages (`.pc`, `.pc_entdata`) are the one case that
needs the whole file decompressed to see inside; that buffer is local to each file in turn and
freed once its names are collected, so scanning many large packages stays bounded to roughly the
size of whichever single one is currently being read, not the whole install.

## Performance

Decompressing and parsing an `AsuraZbb` package to see its entry names is genuinely
CPU-intensive — a real profile of a full scan found over 90% of total CPU time inside
`compress/zlib`/`compress/flate` decompression, and each package's decompression is completely
independent of every other file's. `scan` runs this decompress-and-parse step across every
available CPU core at once (bounded to `GOMAXPROCS`), instead of one file at a time.

**File reads themselves stay sequential, deliberately** — an earlier version of this also
parallelized the raw file reads, and measured almost no real improvement on a real ~20GB
install: on a rotational hard disk (the common case for a large game library — an SSD-only
install would likely see a bigger win), several goroutines reading different large files at
once forces the drive's single read head to keep jumping between unrelated locations, which
can erase most of the benefit parallel CPU work would otherwise provide. Reading files
one-at-a-time, in directory order, keeps disk access close to the sequential pattern spinning
media performs best at, while still handing each file's already-in-memory bytes off to a
worker-pool for decompression/parsing — overlapping *that* CPU work with the *next* file's
read. Measured on a real ~20GB sample on rotational storage: this cut wall-clock scan time by
about 20% over the naive fully-sequential version (and further over the
also-parallelize-reads version, which measured *worse* than sequential due to the seek
contention above), landing within about 12% of that drive's own raw sequential-read ceiling
(confirmed independently via `dd`) — there isn't much room left to improve without reading
fewer bytes or faster storage. Output is verified byte-for-byte identical to the original
fully-sequential implementation on the same real data.

## Commands

```text
asr-text-extractor scan <folder> [output-file]
```

Walks `<folder>` and writes the resulting tree to `<output-file>` (default: the folder's own
base name plus `.txt`, e.g. scanning `Envs/Hellbase` writes `Hellbase.txt`).

```sh
asr-text-extractor scan "Envs/Hellbase" hellbase-scan.txt
```

```text
Hellbase
├── h_hellbase.pc
│   ├── files (282)
│   │   ├── Objects\VFX\Destruction\...\Explo_Box_Sm_Chunk_13.anim
│   │   └── ...
│   ├── textures (2502)
│   │   ├── \graphics\weapons\rifles\carcano\carcano_body_a.tga
│   │   └── ...
│   └── meshes (550)
│       ├── carcano [skeleton: Carcano]
│       ├── l1#carcano [skeleton: Carcano]
│       └── ...
├── h_hellbase.pc_entdata
│   └── files (4)
│       ├── LevelExportTemp0/HellBase.snd
│       └── ...
├── h_hellbase.pc_textures
│   └── textures (...)
│       └── ...
├── h_hellbase.pc_wav_en.pc.streamsounds
│   └── sounds (...)
│       └── ...
└── h_hellbase.ts
```

(`h_hellbase.ts` above has no children — it's a chunk type this tool doesn't understand yet, so
it's left as a plain, unexpanded entry, which is itself useful information: it's immediately
visible which files in an install are and aren't understood.)

A mesh's entry gets a `[skeleton: <name>]` suffix when a matching `HSKN` skeleton was found for
it (see [Package Extraction](Package-Extraction.md#skinning-multi-part-meshes)) — the same
matching `package unpack` itself uses, so this reflects exactly which meshes will come out
rigged versus static without needing to actually unpack anything. An `HTXT` string entry gets a
`(SymbolName)` suffix the same way when the file's optional secondary symbol-name table names
that hash (see [Text & Voice Extraction](Text-and-Voice-Extraction.md#symbol-names)).

A subdirectory or file that can't be read (permissions, a broken symlink, ...) gets a single
inline `(error reading ...)` note instead of aborting the whole scan — an install with hundreds
of thousands of files is exactly the situation where one bad entry shouldn't lose everything
else. Likewise, a file whose magic matches but whose content fails to parse gets a
`(failed to parse: ...)` note rather than stopping the scan.

## Known limitations

See [games/](games/Zombie-Army-4.md) for per-game verification status — `scan` runs correctly
on every title tested so far, and its reported counts for a given file are only as accurate as
that file type's own extraction support (see
[games/Sniper-Elite-5.md](games/Sniper-Elite-5.md#mesh-decoding-the-fix) for one case where a
real, confirmed extraction limitation was found — and later fixed — this way).

- Detects HTXT/ASTS/RSCF by their very first chunk tag, and otherwise falls back to a DLLN scan
  (since voice entries are scattered through otherwise-unknown binary data — see
  [Text & Voice Extraction](Text-and-Voice-Extraction.md)) — a plain `"Asura   "`-signed file
  that's none of those (like the `.ts` files seen in a real sample, of unknown chunk type) is
  left unexpanded, not misidentified.
- No `--format`/`--encoding` and nothing is written back — this command only ever reads and
  reports; see the other commands' own pages for actual extraction/repacking.
- Entry order within a recognized file matches decode order (manifest/string-table/entry order
  as stored), not re-sorted — real directories are already returned name-sorted by the
  filesystem, so only the synthetic per-file entry lists follow this rule.
- A section type inside an `AsuraZbb` package that this tool doesn't specifically decode (e.g.
  the small `TTXT` section found in a real DLC texture-override file — see
  [Package Extraction](Package-Extraction.md)) is silently skipped over, not listed by name —
  only `RSCF` (texture/mesh) and `HSKN` (skeleton) entries are enumerated.
