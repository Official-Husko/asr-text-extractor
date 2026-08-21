# asr-text-extractor Wiki

Feature-by-feature documentation for using `asr-text-extractor`. The [README](../README.md) is
a short pointer; this is where the actual detail lives.

## Entries

- [Text & Voice Extraction](Text-and-Voice-Extraction.md) — unpacking, translating, and
  repacking `HTXT` (menu/UI) and `DLLN` (voice line) strings; the `--format`/`--encoding`
  interchange system (JSON, YAML, XML, CSV, txt).
- [Sound Extraction](Sound-Extraction.md) — extracting embedded WAV assets from `ASTS`
  streamsounds manifests (extract-only, no repacking yet).

## Planned (not yet implemented)

- Texture extraction
- Model extraction
- A second, smaller sound chunk type (`FNFO`) seen in a sample `gmsnd.asr_wav_en` file —
  not yet reverse-engineered (see [Sound Extraction](Sound-Extraction.md#known-limitations))
- Recompiling/repacking beyond text overrides (phase 2) — this includes repacking sound,
  texture, and model assets once their formats are understood

Each of these gets its own wiki entry once it's implemented.
