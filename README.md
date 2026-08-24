# asr-text-extractor

A cross-platform Go tool for unpacking, translating, and repacking data in Rebellion's
**Asura Engine** games — Sniper Elite 4, Zombie Army 4, and (in progress) other Asura-engine
titles such as Sniper Elite 5 and Sniper Elite Resistance.

It reads and writes the `"Asura   "`-signed container format's chunk types:

- **`text`** — `HTXT` chunks: a single menu/UI string table per file (e.g. `Menu.asr_en`) —
  unpack, translate, and repack
- **`voice`** — `DLLN` chunks: many voice-line entries per file (e.g. `MP.pc_en`) — unpack,
  translate, and repack
- **`sound`** — embedded WAV assets, from either an `ASTS` `.streamsounds` manifest or an
  RSCF-based `.pc.sounds` audio archive (auto-detected) — extract only
- **`texture`** — `RSCF` chunks: embedded textures in a `.pc_textures` archive — extract only,
  as raw `.dds` or, with `--convert png`, as decoded lossless PNG
- **`package`** — `AsuraZbb`-compressed level packages (`.pc`, `.pc_entdata`): manifest
  sub-files, embedded textures, and meshes — extract only, as binary glTF by default (a real,
  riggable armature when a mesh has a matching skeleton — e.g. a rifle's bolt assembly, posable
  bone-by-bone in Blender — with a matching diffuse/normal material embedded directly in the
  file) or Wavefront `.obj` via `--mesh-format obj`; LOD and destroyed-state variants of the
  same object are combined into one file by default (`--separate-lods` to opt out)

It also has a **`scan`** command: walk a whole folder and write a text tree listing every
recognized file's own entry names (sub-files, textures, meshes, strings, voice lines, ...) with
no data extracted — for browsing a full game install's structure before committing to a real
unpack of any one file.

Mesh normals aren't decoded yet — the container/chunk reader (`pkg/asura`) is built to be
extended as more of these formats are reverse-engineered. Recompiling/repacking beyond
text/voice overrides is also planned (phase 2).

## Game support

The container format is shared across Rebellion's Asura-engine titles, but not every chunk type
has stayed byte-identical between them — some features needed a real, engine-revision-specific
fix (see the footnotes) before they worked on every tested title.

| Game | Text / Voice | Sound | Texture | Package / Mesh |
|---|---|---|---|---|
| Zombie Army 4 | ✅ | ✅ | ✅ | ✅ |
| Sniper Elite 5 | ✅ | ✅ | ✅ | ✅ ¹ |
| Sniper Elite Resistance | ✅ | ✅ | ✅ | ✅ ¹ |
| Sniper Elite 4 | ❔ ² | ❔ ² | ❔ ² | ❔ ² |

¹ Needed a real, confirmed engine-revision-specific fix to work: this title's mesh format uses a
2-float position offset instead of Zombie Army 4's 3-float one. `ParseMesh` now detects and
handles both automatically — see the wiki for the byte-level derivation.
² No installation has been available to test against during development. Text/voice support is
inherited from this project's original format sources (themselves built for Sniper Elite 4) but
never independently confirmed; every other feature is completely untested for this title.

See the wiki's **[per-game pages](wiki/Home.md#games)** for the full detail behind every ✅/❔
above — what was actually tested, real error messages, and sample files used.

## Quick start

```sh
go build -o asr-text-extractor ./cmd/asr-text-extractor

./asr-text-extractor text unpack Menu.asr_en           # -> Menu.json
# ...edit the "override" field of the entries you want to translate...
./asr-text-extractor text override Menu.asr_en Menu.json
```

Output defaults to JSON; `--format` also accepts `yaml`, `xml`, `csv`, and the original tool's
tab-separated `txt`. See the **[wiki](wiki/Home.md)** for the full command reference, the
interchange format details, and the `<TAG>` placeholder reference.

## Building

```sh
go build ./cmd/asr-text-extractor
```

Run `go test ./...` to run the test suite.

## Credit

Every format in this repository is an original Go implementation, verified against real game
files. See [CREDITS.md](CREDITS.md) for the prior work that informed some of that
understanding.
