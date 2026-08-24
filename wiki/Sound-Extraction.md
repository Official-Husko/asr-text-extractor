# Sound Extraction

Covers the `sound` command: extracting embedded WAV audio assets from a streamsounds (`ASTS`)
file or an RSCF audio archive (`.pc.sounds`).

## Background

`.streamsounds` files (e.g. `ss_cut_dlc_horde.asr.pc.streamsounds`) hold an `ASTS` chunk: a
small manifest listing the game's own asset paths for a handful of sound effects, followed
immediately by the actual audio data — one complete, valid RIFF/WAVE file per manifest entry,
back-to-back, no gaps. This was confirmed against a real Zombie Army 4 sample: every extracted
slice is a well-formed WAV (correct declared RIFF size, proper `fmt `/`data`/`smpl`
subchunks), and the three entries accounted for the entire file except a 4-byte zero footer at
the very end — the same footer convention the `HTXT` symbol-name table uses (see
[Text & Voice Extraction](Text-and-Voice-Extraction.md#symbol-names)).

`.pc.sounds` files (e.g. `Chars/mp.pc.pc.sounds`, one per matching `.pc`/`.gui` asset — very
common across a real install) are a *different* container for the same kind of content: a
plain, uncompressed `"Asura   "`-signed file whose very first section is `RSCF` — the exact
same per-entry tag+size+resource-type framing already used for embedded textures/meshes (see
[Texture Extraction](Texture-Extraction.md) and [Package Extraction](Package-Extraction.md)),
just with resource-type code `3` (audio) instead of `2` (texture), and a payload that's a
complete RIFF/WAVE file instead of a DDS. Confirmed against two independent real samples
(`Chars/mp.pc.pc.sounds`, `GUIMenu/ingame_common.gui.pc.sounds`): both extract to valid,
playable WAV files (Microsoft ADPCM, per-entry paths like
`sounds\hud\duty_roster_collected\hud_duty_roster_collected_01.wav`) with the resource-type
field reading exactly `3` on every entry, resolving what had previously been documented as an
unimplemented, unconfirmed type code (see `rscfResourceTypeAudio` in `pkg/asura/rscf.go`).

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
needed. If `output-dir` is omitted, it defaults to the input's base name. The container type
(`ASTS` or `RSCF`) is auto-detected from the chunk tag right after the Asura magic — the same
file, folder, or invocation works for either.

```sh
asr-text-extractor sound unpack ss_cut_dlc_horde.asr.pc.streamsounds
# -> ss_cut_dlc_horde.asr.pc/Sounds/Cutscenes/Flix/DLC_HORDE_Railyard/CUT_DLC_HORDE_Railyard_Intro_SFX.wav
# -> .../CUT_DLC_HORDE_Railyard_Outro_SFX.wav
# -> .../CUT_DLC_HORDE_Railyard_Middle_SFX.wav

asr-text-extractor sound unpack Chars/mp.pc.pc.sounds
# -> mp.pc.pc/sounds/hud/duty_roster_collected/hud_duty_roster_collected_01.wav
```

It also prints a one-line diagnostic to stderr (`Version`/`Entries` for ASTS, `Entries` for
RSCF) before extracting.

Unlike `text`/`voice`, there is no `--format`/`--encoding` (extracted files are the audio
itself, not a translatable interchange format) and no `override`/repack — this is
extract-only, matching where the project's sound support currently stands.

`package unpack` also extracts any RSCF audio entries found embedded directly inside a `.pc`
level package (written to an `audio/` subdirectory alongside `textures/`), the same way it
already handles embedded RSCF textures — not yet seen in a real sample (every real audio RSCF
section found so far has been a standalone `.pc.sounds` file, not nested inside a `.pc`), but
wired in for when it turns up, following the same resource-type dispatch `pkg/asura/rscf.go`
already uses.

## Known limitations

See [games/](games/Zombie-Army-4.md) for per-game verification status — streamsounds (`ASTS`)
extraction is confirmed **broken** on Sniper Elite 5 (see
[games/Sniper-Elite-5.md](games/Sniper-Elite-5.md#why-streamsounds-asts-extraction-fails)), a
real incompatibility this feature-level page doesn't otherwise call out.

- Two RSCF/ASTS-family container shapes are understood (see above). A different, much smaller
  sound-adjacent chunk (`FNFO`, exactly 36 bytes, `.ssm` files like `sounds/ss_cut_dlc01.ssm`,
  and previously a sample `gmsnd.asr_wav_en` file) is now understood at the byte level even
  though it carries no extractable content: every `.ssm` file in a real install — all 49 of
  them, spanning all four naming families found (`ss_cut_*`, one per level/cutscene; `ss_env_*`,
  one per level/DLC; `ss_mus_*`, one per music track; and the singular `streamingsounds.ssm`,
  whose name alone suggested it might be a real manifest into the game's separate audio
  streaming pool — it isn't) — has the exact same MD5 checksum. A constant `FNFO`-tagged stub
  whose body is `{1, 8, 32, 8}` (32 = its own 36-byte total length minus the trailing 4-byte zero
  footer, the same formula confirmed for the `FNFO`-without-`RSFL` package variant — see
  [Package Extraction](Package-Extraction.md)). Since every sample is identical, there is no
  per-level data to extract — it's an inert placeholder, not an unsolved format.
- Extract-only: there's no way to repack edited/replacement audio back into either container
  type yet (this is true of every asset type except text/voice strings — recompiling is a
  planned phase 2, see [Home](Home.md#planned-not-yet-implemented)).
- The exact zero-padding rule between an ASTS manifest entry's path and its size/offset fields
  isn't fully pinned down (only one sample file, with very little path-length variation, has
  been examined) — parsing tolerates any padding length by scanning for the next non-zero
  byte, so this doesn't affect extraction, but it means the precise on-disk convention isn't
  documented with full confidence yet.
