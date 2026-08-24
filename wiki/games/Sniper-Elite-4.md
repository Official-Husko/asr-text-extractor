# Sniper Elite 4

**No installation of this game has been available to test against during this project's
development.** Support for it is inherited, not verified: this project's text and voice format
understanding (`HTXT`/`DLLN` chunks) is itself based on an earlier tool built specifically for
Sniper Elite 4's own file formats (see `CREDITS.md`), so those two features are *expected* to
work correctly — but every other feature (sound, texture, package/mesh extraction, `scan`), and
even text/voice themselves, have never been run against a real Sniper Elite 4 file by this
project.

## Status

| Feature | Status |
|---|---|
| `text` (unpack/override/compare) | ❓ Presumed working — the format understanding this project's implementation is based on originates from Sniper Elite 4 tooling — but never independently confirmed against a real file. |
| `voice` (unpack/override) | ❓ Presumed working, same basis and same caveat as `text` above. |
| `sound` | ❓ Untested. No reason to assume it doesn't work (the container format is shared across titles), but unlike [Sniper Elite 5](Sniper-Elite-5.md), where testing turned up a real, confirmed incompatibility in the streamsounds manifest, Sniper Elite 4 hasn't been checked at all. |
| `texture` | ❓ Untested. |
| `package` — sub-file/texture extraction | ❓ Untested. |
| `package` — mesh decoding | ❓ Untested. Given [Sniper Elite 5](Sniper-Elite-5.md#why-meshes-dont-decode) — a *later* title than Sniper Elite 4 — has a mesh format this tool can't decode at all, there's no safe assumption to make here either way: Sniper Elite 4's mesh format could match Zombie Army 4's, match Sniper Elite 5's, or be a third, distinct shape. |
| `scan` | ❓ Untested. |

## If you have Sniper Elite 4 files

This is the single biggest gap in this project's real-world verification. Running `scan` over a
real installation and comparing the result against what's documented for [Zombie Army
4](Zombie-Army-4.md) and [Sniper Elite 5](Sniper-Elite-5.md) would be the most direct way to
close it — particularly whether mesh decoding works (see the open question above) and whether
the streamsounds bug found in Sniper Elite 5 is present here too.
