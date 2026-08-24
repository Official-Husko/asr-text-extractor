# Credits

Every chunk parser in this repository is an original Go implementation, built by understanding
and verifying each on-disk format against real game files — not by copying source code from
elsewhere. Where that understanding was informed by prior public work, it's recorded here rather
than scattered across the README and source comments.

- **Text and voice string handling** (`HTXT`/`DLLN` chunks, `pkg/asura/htxt.go`,
  `pkg/asura/dlln.go`): behavior — including some intentionally-preserved quirks, like exact
  filesize arithmetic and auto-backup-on-override — matches an earlier Windows/.NET tool for the
  same file formats, **AsrTextExtractor**. No license was published with that tool; this
  project's Go code was written fresh from an understanding of its behavior, not by translating
  its source.
- **Mesh and skeleton parsing** (`pkg/asura/mesh.go`, `pkg/asura/skeleton.go`): an earlier
  version of this project's mesh decoder, based on a format hypothesis carried over from a
  different Rebellion Asura-engine game, produced garbled geometry once actually opened in a 3D
  viewer. The current, correct implementation matches a known-working reference decoder from an
  independently-authored Zombie Army 4 reverse-engineering project (no license published), which
  had already fully solved this format with its own working importer.
- **RSCF/RSFL manifest field layouts** (texture and package archive entries,
  `pkg/asura/rscf.go`, `pkg/asura/package.go`, `pkg/asura/container.go`): several uncertain byte
  fields were cross-checked against independent community Asura-format reference scripts for
  related titles, rather than trusted on a single sample's byte-count reconciliation alone.
- **The `AsuraZbb` compression wrapper** (`pkg/asura/zbb.go`): field layout was independently
  confirmed against a decompiled third-party tool for a related title.

None of the source material above is copied into this repository; it was consulted to verify
independently-derived format understanding, not translated line-by-line.
