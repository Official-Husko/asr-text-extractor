# Sound Extraction

Covers the `sound` command: extracting embedded WAV audio assets from a streamsounds file.

## Background

`.streamsounds` files (e.g. `ss_cut_dlc_horde.asr.pc.streamsounds`) hold an `ASTS` chunk: a
small manifest listing the game's own asset paths for a handful of sound effects, followed
immediately by the actual audio data — one complete, valid RIFF/WAVE file per manifest entry,
back-to-back, no gaps. This was confirmed against a real Zombie Army 4 sample: every extracted
slice is a well-formed WAV (correct declared RIFF size, proper `fmt `/`data`/`smpl`
subchunks), and the three entries accounted for the entire file except a 4-byte zero footer at
the very end — the same footer convention the `HTXT` symbol-name table uses (see
[Text & Voice Extraction](Text-and-Voice-Extraction.md#symbol-names)).

The audio itself is typically Microsoft ADPCM-compressed (`WAVE_FORMAT_ADPCM`, format tag
`2`) — standard tooling that understands ADPCM (ffmpeg, Audacity, Blender's audio import, VLC)
will open it fine; Python's stdlib `wave` module and anything expecting plain PCM won't.

## Commands

```text
asr-text-extractor sound unpack <file> [output-dir]
```

Extracts every embedded WAV from `<file>`, writing each one to `<output-dir>` at the relative
path recorded in its manifest entry (backslashes normalized, and any `.`/`..` path component
dropped so a crafted path can't write outside `<output-dir>`), creating subdirectories as
needed. If `output-dir` is omitted, it defaults to the input's base name.

```sh
asr-text-extractor sound unpack ss_cut_dlc_horde.asr.pc.streamsounds
# -> ss_cut_dlc_horde.asr.pc/Sounds/Cutscenes/Flix/DLC_HORDE_Railyard/CUT_DLC_HORDE_Railyard_Intro_SFX.wav
# -> .../CUT_DLC_HORDE_Railyard_Outro_SFX.wav
# -> .../CUT_DLC_HORDE_Railyard_Middle_SFX.wav
```

It also prints a one-line diagnostic to stderr (`Version`, `Entries`) before extracting.

Unlike `text`/`voice`, there is no `--format`/`--encoding` (extracted files are the audio
itself, not a translatable interchange format) and no `override`/repack — this is
extract-only, matching where the project's sound support currently stands.

## Known limitations

- Only the `ASTS` streamsounds manifest chunk is understood. A different, much smaller sound
  chunk type (`FNFO`, ~36 bytes, seen in a sample `gmsnd.asr_wav_en` file) has been observed
  but not reverse-engineered — it looks like a lightweight reference/pointer rather than
  embedded audio, and needs more sample data before it's worth guessing at.
- Extract-only: there's no way to repack edited/replacement audio back into a
  `.streamsounds` file yet (this is true of every asset type except text/voice strings —
  recompiling is a planned phase 2, see [Home](Home.md#planned-not-yet-implemented)).
- The exact zero-padding rule between a manifest entry's path and its size/offset fields
  isn't fully pinned down (only one sample file, with very little path-length variation, has
  been examined) — parsing tolerates any padding length by scanning for the next non-zero
  byte, so this doesn't affect extraction, but it means the precise on-disk convention isn't
  documented with full confidence yet.
