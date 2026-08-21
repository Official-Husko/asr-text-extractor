# asr-text-extractor Wiki

Feature-by-feature documentation for using `asr-text-extractor`. The [README](../README.md) is
a short pointer; this is where the actual detail lives.

## Entries

- [Text & Voice Extraction](Text-and-Voice-Extraction.md) — unpacking, translating, and
  repacking `HTXT` (menu/UI) and `DLLN` (voice line) strings; the `--format`/`--encoding`
  interchange system (JSON, YAML, XML, CSV, txt).

## Planned (not yet implemented)

- Texture extraction
- Model extraction
- Sound extraction — an `ASTS` chunk (readable `.wav` asset paths, looks like a sound-bank
  manifest) has been seen in a sample `.streamsounds` file but isn't reverse-engineered yet
- Recompiling/repacking beyond text overrides (phase 2)

Each of these gets its own wiki entry once it's implemented.
