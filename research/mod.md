# `.mod` research notes

Status as of this writing: **a real, substantially-cracked format in two of the three Asura
titles checked, plus a major structural discovery that generalizes beyond `.mod` itself.** Found
while surveying `za4-full-file-list` for undecoded formats; picked back up at the user's request
("`.mod` files next"). Zombie Army 4 turned out to have exactly one `.mod` file in the entire
install — a tiny, opaque 24-byte stub — which looked like a dead end until the user's own other
installed Asura-engine titles (Sniper Elite 5, Sniper Elite Resistance) turned out to have a much
richer `.mod` variant in the equivalent file, ~500x larger. No Go code exists for any of this
yet. Everything below is pure research: real bytes from real samples across three different
games, inspected with throwaway Python scripts (never committed).

## Two very different `.mod` shapes across three games

`.mod` is a manifest sub-file (like `.anim`/`.pfx`/`.snd`/`.nav`/`.cut`/`.ent`/`.sky`/`.gi`/
`.fsx`), bundled inside a level's `.pc` package — not a standalone file on disk.

**Zombie Army 4**: exhaustively confirmed to have exactly **one** `.mod` file in the entire
game install (checked all 56 real `.pc` files under `Envs/`, only `Envs/3D_Frontend/
3d_frontend.pc` has a `.mod` manifest entry). That one file is 24 bytes: six `u32` fields —
`10, 0, 13, 100000, 0x6cad8cee, 66`. No magic, no tag, no ASCII anywhere in it (unlike `.ssm`,
which turned out to share the `FNFO` stub shape — this one really is a bare, structured blob).
Tested the hash-looking field (`0x6cad8cee`) against several common hash functions (CRC32,
FNV1a-32, DJB2, SDBM) across a dozen candidate strings derived from the file's own name and
path — no match. Checked the package's own known contents (29 manifest entries, 271 textures,
46 meshes) against the file's other fields (10, 13, 100000, 66) — no match either. With only one
real sample and no crack so far, this variant is a dead end pending either a hash-algorithm
breakthrough elsewhere in the project or a second real ZA4 sample turning up.

**Sniper Elite 5 and Sniper Elite Resistance**: the equivalent file
(`envs/3D_FrontEnd/m_3d_frontend.pc`'s own `LevelExportTemp0\3D_Frontend.mod` entry) is
**11,788 bytes (SE5) / 11,976 bytes (Resistance)** — nearly 500x larger than the ZA4 sample, and
a completely different, genuinely structured format: dense binary content from byte 0, no small
integer header at all. The first ~176 bytes of both games' files are byte-for-byte identical,
diverging only slightly further in (expected — Resistance reuses and lightly edits SE5's
frontend scene assets, consistent with how the project's own reference material describes the
two titles' relationship).

## The SE5/Resistance shape: repeating vertex + quad-index mesh chunks

Decoding the file as `float32` from byte 0 immediately shows real, physically-plausible,
tightly-clustered coordinate data (e.g. `175.04, -265.13, 3.56`, `175.04, -265.13, -12.09`, ...)
— genuine 3D positions, not noise. Scanning the whole file for runs of "plausible" floats
(`0.5 < |v| < 5000`, the same kind of heuristic filter used successfully elsewhere in this
project's research) versus runs that don't decode as plausible floats at all reveals a clean,
consistent **alternating structure**: a run of `float32` XYZ triples (vertex positions),
followed immediately by a run of small, non-float-looking values that decode cleanly as
`uint16` indices.

The very first pair confirms the shape precisely: **1500 floats (500 vertices × 3) followed by
676 `uint16` values, which is exactly 169 × 4** — a **quad index buffer** (4 indices per face),
not triangles. (The measured coordinate run was 1501 floats, one more than the clean 500×3 —
there's a single extra plausible-looking value at the boundary not yet accounted for; likely a
minor boundary-detection artifact of the heuristic, not evidence the vertex count itself is
wrong, given 500×3 is exact and 500 is a clean round number.)

This vertex+index pair pattern **repeats 130 times across the whole file**, in a clear size
distribution:

| vertices per section | occurrences | typical index (`u16`) count |
|---|---|---|
| 500 | 1 (the "hero" mesh) | 676 (169 quads) |
| 8 | 9 | 16–20 |
| 1 | 60 | 2 |
| 0 (fewer than 3 floats — likely 1-2 raw values, not a full XYZ triple) | 60 | 2–4 |

The most natural reading: **one large decorative "hero" mesh** (plausibly the animated 3D scene
geometry visible behind Sniper Elite 5's/Resistance's main menu — camera-facing panels, screens,
or similar dressing, built from quads) **plus many much smaller placement points** — the
1-vertex and 2-value "mesh" entries are far too small to be real geometry and are much more
likely simple 3D position markers (particle-effect spawn points, light positions, or similar),
consistent with what's found immediately after the last one (see below). The 9 medium (8-vertex)
sections are plausibly small decorative geometry — individual quad-based props.

## The `FXPT` marker and a second confirmed cross-manifest-boundary stream

Right after the last vertex/index section, both real samples end with a distinct 4-byte ASCII
tag never seen elsewhere in this project: **`"FXPT"`** (almost certainly "FX Point" — an effect
attachment point). Its shape: `"FXPT"` + `u32` (a size-shaped field, `1936` in the SE5 sample,
`1764` in Resistance — see below) + `u32 = 37` (**identical in both independent samples** — a
real constant) + `u32 = 0` + a `u32` hash-like value (`0x085e50da` in SE5, `0x38e93cc8` in
Resistance — different per sample, presumably per-effect) + a plain ASCII path string.

**In both real samples, that trailing string is truncated exactly at the `.mod` file's own end**
— `"SE5\FireEffe"` (SE5) / `"SE5\WeaponEf"` (Resistance), cut off mid-word with no null
terminator anywhere in the file. This is not corruption or a bug in this project's own
extraction (double-checked: the manifest-declared offset/size for the `.mod` entry is used
exactly as-is, the same trusted mechanism `package.go`'s `PackageEntry` extraction already uses
elsewhere). **It's the same phenomenon `research/entdata.md` already documented for `.cut`/
`.ent`**: the manifest's declared per-entry boundary doesn't correspond to independent content —
it's an arbitrary split of one continuous byte stream. Checking the very next manifest entry
after `.mod` in each package confirms this exactly, in both independent games:

- SE5: next entry is `LevelExportTemp0\SE5\FireEffects\SE5_Fire_Smoke_Plume.pfx`. That `.pfx`
  file's own first extracted bytes are `"cts\SE5_Fire_Smoke_Plume:Smoke123..."`. Concatenated:
  `"SE5\FireEffe" + "cts\SE5_Fire_Smoke_Plume:Smoke123..." = "SE5\FireEffects\
  SE5_Fire_Smoke_Plume:Smoke123..."` — an **exact** match to the manifest's own declared path
  for that entry (`SE5\FireEffects\SE5_Fire_Smoke_Plume.pfx`), continuing straight into that
  `.pfx` file's own already-documented internal `<name>:<substream>` identifier format (see the
  `.pfx` findings in `CLAUDE.md`) rather than stopping at a `.pfx` extension.
- Resistance: next entry is `LevelExportTemp0\SE5\WeaponEffects\
  SE5_AA_Artillery_Gun_Tracer_Looped.pfx`. Its own first bytes:
  `"fects\SE5_AA_Artillery_Gun_Tracer_Looped:MuzSmoke..."`. Concatenated: `"SE5\WeaponEf" +
  "fects\SE5_AA_Artillery_Gun_Tracer_Looped:MuzSmoke..." = "SE5\WeaponEffects\
  SE5_AA_Artillery_Gun_Tracer_Looped:MuzSmoke..."` — again an exact match.

**This is a second, independently-confirmed instance of the cross-manifest-boundary-stream
phenomenon, in two different games, for a completely different pair of sub-file types than the
first one (`.cut`+`.ent`, a single relationship in one game).** That raises the real
possibility that this isn't a one-off relationship between two specific sub-file types, but a
general property of how this whole manifest format packs sub-files: consecutive manifest
entries may routinely be slices of one continuous stream rather than independent content, and
`.cut`/`.ent` and `.mod`/`.pfx` are just the two pairs caught so far. Worth testing directly
against other adjacent sub-file pairs in a future round (see Suggested next steps).

The `FXPT` size field (`1936` SE5, `1764` Resistance) is almost certainly the *true* remaining
byte length of the `FXPT` record, measured from a point that extends *past* `.mod`'s own
declared end and into the next sub-file(s) — consistent with the cross-boundary-stream finding,
and analogous to how `research/entdata.md`'s `TEXT` section's own declared size was what first
proved the `.cut`/`.ent` boundary crossing. Not yet verified precisely (would need reading
`1936`/`1764` bytes forward from the `FXPT` tag across the sub-file boundary and checking
whether that lands on a sensible next-record boundary), but a strong, well-motivated next check.

## Open questions

1. **The ZA4 24-byte `.mod` stub remains completely uncracked** — no hash match, no
   cross-reference to its own package's contents, and only one real sample exists in the whole
   game (confirmed exhaustively). Whether it's a genuinely different, much simpler format, or a
   degenerate/near-empty case of the same SE5/Resistance shape (which would need at least a
   `FXPT` tag or *something* recognizable — it has neither), is unresolved.
2. **The exact byte layout of a single vertex+index section's header/framing** — this research
   found the pattern by scanning for plausible-vs-implausible float runs, not by locating a
   real per-section tag or length field the way `VAND`/`ENTI`/other records in this project have
   been cracked. There's presumably a real count-based framing (a vertex count, an index count)
   preceding each section rather than this research's after-the-fact heuristic boundary
   detection — not yet found.
3. **What the 60 "0-vertex" (fewer than 3 floats) tiny sections actually hold** — 2 floats each
   in most cases, too small to be a 3D position. Possibly a 2D UV coordinate, a min/max pair, or
   something else entirely.
4. **Whether the cross-manifest-boundary-stream phenomenon generalizes beyond `.cut`/`.ent` and
   `.mod`/`.pfx`** — worth directly testing a few more adjacent sub-file pairs in a real
   package's manifest (e.g. whatever immediately follows a `.pfx` or `.snd` entry) rather than
   assuming either confirmed pair is representative.
5. **The `FXPT` size field's exact meaning and the full extent of the real record it
   describes** — plausible as "bytes remaining, crossing sub-file boundaries," not yet verified
   byte-for-byte.
6. **Whether SE5's and Resistance's own manifests order their `.pc` sub-files consistently
   enough that this cross-boundary reconstruction could be automated** (i.e. always concatenate
   entry *N* with entry *N+1* before parsing either) — both real samples checked here happened
   to have `.mod` immediately followed by the referenced `.pfx`, but that adjacency itself isn't
   proven to be guaranteed by the format rather than coincidental level-authoring order.

## Suggested next steps

- Try to locate the real per-vertex-section length/count framing (rather than relying on the
  plausible-float heuristic) by checking whether a small header value near the start of each
  section (or the tail of the previous one) matches that section's own vertex or index count
  exactly — the same kind of directly-measured verification that cracked `.navmesh`'s `3N+7`
  formula.
- Test the cross-manifest-boundary-stream hypothesis against a few more adjacent sub-file pairs
  in a real package (not just `.cut`/`.ent` and `.mod`/`.pfx`) to see how general it really is —
  this could be a significant, broadly-applicable correction to how `package.go` should
  eventually expose `PackageEntry` data, if boundaries turn out to be routinely unreliable.
- If a Sniper Elite 4 (this project's original reference title) or further Sniper Elite 5/
  Resistance level package turns up with its own `.mod` file, use it for a genuine 3-way
  differential comparison, the technique that's repeatedly cracked format details elsewhere in
  this project's research.
- Decode the `FXPT` record's own hash field against the same kind of hash-cracking attempt tried
  (unsuccessfully) for the ZA4 stub's field — a working algorithm found here would likely also
  resolve the long-standing `MeshGroup.Hash` and `ENTI`-hash open problems elsewhere in this
  project.
