# asr-text-extractor

A cross-platform Go tool for unpacking, translating, and repacking data in Rebellion's
**Asura Engine** games — Sniper Elite 4, Zombie Army 4, and (in progress) other Asura-engine
titles such as Sniper Elite 5 and Sniper Elite Resistance.

It reads and writes the `"Asura   "`-signed container format's chunk types:

- **`text`** — `HTXT` chunks: a single menu/UI string table per file (e.g. `Menu.asr_en`) —
  unpack, translate, and repack
- **`voice`** — `DLLN` chunks: many voice-line entries per file (e.g. `MP.pc_en`) — unpack,
  translate, and repack
- **`sound`** — `ASTS` chunks: embedded WAV assets in a `.streamsounds` manifest — extract only
- **`texture`** — `RSCF` chunks: embedded textures in a `.pc_textures` archive — extract only,
  as raw `.dds` or, with `--convert png`, as decoded lossless PNG
- **`package`** — `AsuraZbb`-compressed level packages (`.pc`, `.pc_entdata`): manifest
  sub-files, embedded textures, and meshes — extract only, as binary glTF by default (a real,
  riggable armature when a mesh has a matching skeleton — e.g. a rifle's bolt assembly, posable
  bone-by-bone in Blender) or Wavefront `.obj` via `--mesh-format obj`; LOD and destroyed-state
  variants of the same object are combined into one file by default (`--separate-lods` to opt
  out)

Mesh normals aren't decoded yet — the container/chunk reader (`pkg/asura`) is built to be
extended as more of these formats are reverse-engineered. Recompiling/repacking beyond
text/voice overrides is also planned (phase 2).

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

Ports the file-format understanding from the original **AsrTextExtractor** C# project (no
license file was published with it) to a cross-platform Go implementation.
