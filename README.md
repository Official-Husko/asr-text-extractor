# asr-text-extractor

A cross-platform Go tool for unpacking, translating, and repacking the text data in Rebellion's
**Asura Engine** games — Sniper Elite 4, Zombie Army 4, and (in progress) other Asura-engine
titles such as Sniper Elite 5 and Sniper Elite Resistance.

It reads and writes the `"Asura   "`-signed container format's chunk types:

- **`text`** — `HTXT` chunks: a single menu/UI string table per file (e.g. `Menu.asr_en`)
- **`voice`** — `DLLN` chunks: many voice-line entries per file (e.g. `MP.pc_en`)

Texture, model, and sound unpacking are planned but not yet implemented — the container/chunk
reader (`pkg/asura`) is built to be extended with new chunk types as those formats are
reverse-engineered.

## Usage

```text
asr-text-extractor text unpack    <file> [csv]
asr-text-extractor text override  <file> <csv> [outfile] [--force]
asr-text-extractor text compare   <fileA> <fileB> [csv]

asr-text-extractor voice unpack   <file> [csv]
asr-text-extractor voice override <file> <csv> [outfile]
```

- `unpack` extracts strings to a tab-separated, UTF-16LE CSV for translation.
- `override` writes a CSV's translated strings back into a copy of the binary file. Without an
  explicit output path it edits in place and keeps a `<file>_back` backup of the original.
- `compare` builds a source/override table across two language files of the same asset.

Gamepad button glyphs and control characters in unpacked text appear as `<TAG>` placeholders
(e.g. `<INPUT_FRONTEND_A>`, `<NL>`) — edit around them, don't remove them.

## Building

```sh
go build ./cmd/asr-text-extractor
```

## Credit

Ports the file-format understanding from the original **AsrTextExtractor** C# project (no
license file was published with it) to a cross-platform Go implementation.
