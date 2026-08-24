# `.navmesh` research notes

Status as of this writing: **a genuine, deepening structural lead, not cracked into a parser
yet, but with several exact, cross-sample-confirmed formulas now in hand.** Found while
surveying `za4-full-file-list` (this project's own `scan` output over the full Zombie Army 4
game install) for extensions nothing in `pkg/asura` understands yet. Unlike most of that survey,
`.navmesh` decompresses through code this project already has and trusts
(`asura.DecompressZbb`) straight into a large, well-formed chunk with real repeating structure —
distinct from, and much larger than, the already-cracked `.nav`/`WPSG` file (see
`research/entdata.md`), which is a small waypoint-graph sub-file bundled inside `.pc_entdata`.
`.navmesh` is a real navigation-mesh *geometry* file, standalone on disk. No Go code exists for
any of this yet. Everything below is pure research: real bytes from three real samples
(`navmesh/Hellbase/h_hellbase_actor.navmesh`, 307,826 bytes on disk; `navmesh/Hellbase/
m_hellbase_actor.navmesh`, 410,606 bytes; `navmesh/ItalianCity/m_italiancity_actor.navmesh`,
a different level entirely), inspected with throwaway Python scripts (never committed).

## The container: plain `AsuraZbb`, nothing new needed to get inside

`.navmesh` files are `AsuraZbb`-compressed exactly like a `.pc`/`.pc_entdata` level package —
same 8-byte `"AsuraZbb"` magic, same chunked-zlib wrapper `zbb.go` already decodes. No new code
was needed to decompress the sample file: `asura.DecompressZbb` on the raw 307,826-byte file
produces an 858,830-byte buffer starting with the standard `"Asura   "` magic, immediately
followed by a chunk tag never seen before in this project: `"ARNM"` (presumably "Actor
NavMesh" or similar, matching the file's own `_actor.navmesh` naming — the sample used here is
`h_hellbase_actor.navmesh`).

The `ARNM` chunk itself follows the same tag+size framing used everywhere else in this project:
tag (4 bytes) + a `uint32` giving the section's remaining length. For the sample file, that
declared size is `858818`, and the buffer's own actual remaining length past the size field is
`858814` — a 4-byte discrepancy consistent with the size field counting from the tag's own
start (`858830 total - 12 bytes consumed so far (magic+tag+size) = 858818`), matching the
convention `skipTaggedSection` already assumes elsewhere in this codebase.

### The 8-field `ARNM` header: 5 of 8 fields are format constants, not content

Right after the tag+size, `ARNM` has an 8×`uint32` header before any `"VAND"` records begin.
Comparing this header across all three real samples is decisive:

| field | h_hellbase | m_hellbase | m_italian | |
|---|---|---|---|---|
| 0 | 13 | 13 | 13 | **constant** |
| 1 | 0 | 0 | 0 | **constant** |
| 2 | 0x8000000d | 0x8000000d | 0x8000000d | **constant** |
| 3 | 219851008 | 290275840 | 308596480 | varies — see below |
| 4 | 256000 | 256000 | 256000 | **constant** |
| 5 | 22528 | 84992 | 148992 | varies |
| 6 | 257024 | 257024 | 257024 | **constant** (= field 4 + 1024) |
| 7 | 17367040 | 68157440 | 164173568 | varies |

Fields 0, 1, 2, 4, 6 are **byte-for-byte identical across all three samples** despite wildly
different file sizes and levels — strong, cross-sample-confirmed evidence these are format
constants (a version marker, reserved fields, and two fixed scale/capacity parameters), not
per-file content.

**Field 3 has an exact, confirmed formula**: `field3 = 256 × (totalFileLength − 37)`, verified
to the byte across all three samples (`256×(858830−37) = 219851008`, `256×(1133927−37) =
290275840`, `256×(1205492−37) = 308596480` — all exact integer matches, no rounding). This
reads as a fixed-point (×256) encoding of the file's own total content length, minus a constant
37-byte tail (plausibly a fixed-size footer, matching the fixed-footer convention already seen
elsewhere in this project — see `parseRSFLManifest`'s and `rscf.go`'s own doc comments for the
same kind of self-describing-length field).

Fields 5 and 7 are also **always exact multiples of 256** (confirmed via integer modulo, not
float rounding) in every sample, suggesting the same ×256 fixed-point convention as field 3, but
what they count hasn't been pinned down — see Open Questions.

### `bucket` is confirmed to be a real spatial grid cell, not an arbitrary ID

Comparing the per-`bucket` record-count distribution between `h_hellbase_actor.navmesh` and
`m_hellbase_actor.navmesh` — two different navmesh variants (presumably two different actor
sizes/types) for the **same level** — is a clean, decisive cross-check: the two distributions
are **nearly identical**, cell for cell:

```
bucket:        16  17  18  19  20  21  22  23  24  25  26  27  28  29  30  31  32  33  34  35  36
h_hellbase:      8  12  13  14  25  29  46  41  38  33  23  17   9   7   1   1   2   1   2   2   1
m_hellbase:      7  12  13  14  25  29  46  41  38  33  23  17   9   7   1   1   2   1   2   2   1
```

Only `bucket 16` differs (8 vs. 7 records — matching the two files' 325-vs-324 total record
count exactly). Every other cell has the *exact* same record count in both files. This is very
strong confirmation that `bucket` really is a spatial grid cell index tied to the level's own
geometry, not a per-file arbitrary counter — two navmeshes baked for the same level naturally
produce nearly the same cell population. `m_italiancity_actor.navmesh` (an unrelated level)
has a completely different bucket range (1–34, not 16–36) and a completely different
distribution shape, as expected for a different level's geometry.

## A second, distinct record type: 22 fixed-size "edge" records early in the header

The ~33KB of `ARNM` header content between the 8-field header above and the first `"VAND"`
record isn't one undifferentiated blob. The first ~4.7KB of it (`h_hellbase` sample: bytes
188–4852) is a run of exactly **22 fixed-size, 212-byte records**, each carrying two 3D
coordinate points — almost certainly navmesh portal/boundary **edges**, distinct from `VAND`'s
per-cell polygon records. Found by first spotting physically-plausible coordinate pairs at a
suspiciously exact 212-byte stride, then locating the real per-record marker precisely:

```
[u32 = 0x??3F0000] [u32: 1 or 2, in the top byte] [u32 = 16777216] [u32 = 1004] [u32: counter]
[float×3: point A] [float×3: point B]
[u32 = 0] [u32 ≈ 0] [float = 0.5]
...further fields (offsets +56 to +212), not yet interpreted...
```

Confirmed exactly across all 22 records in the sample (`gap` between consecutive record starts
is **exactly 212 bytes, 21/21 times**, no exceptions): the marker field at `+8` is always
`16777216` (`0x01000000`), the field at `+12` is always `1004` — which is exactly `field6 / 256`
from the `ARNM` header above (`257024 / 256 = 1004`), a genuine cross-reference between the
8-field header and this record type. The field at `+16` is a small counter that increments by
`+4` almost every record (`67843 → 67847 → 67851 → 67855 → 67859 → ...`), and is suspiciously
close to `field7 / 256` from the header (`17367040 / 256 = 67840`) — off by a small, growing
amount, consistent with this being a running byte- or unit-offset counter that *starts* near
`field7/256` rather than *equaling* it exactly.

The two 3-float points per record are real, physically plausible level-space coordinates (e.g.
`(18.67, -4.56, -292.48)` and `(15.72, -1.78, -292.34)` for the first record — close together,
consistent with two ends of a short boundary edge). The trailing `0.5` constant at `+52`
matches a `0.5` constant found inside `VAND` records too (see below) — a real, if not yet
understood, cross-structure link between the two record types, not a coincidence.

The field at `+4` (top byte only: observed values `1` or `2` across the 22 records) doesn't
correlate with the fixed 212-byte record length — records with top-byte `1` and `2` are both
exactly 212 bytes — so it's a flag or sub-type value, not a size determinant.

**Where the header's remaining ~28KB (bytes 4852–33706) goes is still unknown.** It doesn't
match this 212-byte record marker at any offset (confirmed by an exhaustive scan of the whole
header region for the marker pattern — it only ever appears within the first 22-record block),
and inspecting it directly shows no smooth, coordinate-like floats — the values look closer to
hash/ID data (some exact-repeat `u32` values recur at small, regular byte offsets, e.g. a
3-value group repeating with a 12-byte period at bytes ~4956–5000) than to geometry. Not yet
understood; flagged as the single biggest remaining unknown in the header region.

## The repeating `"VAND"` record marker

A scan for the literal 4-byte ASCII sequence `"VAND"` inside the decompressed buffer found 325
occurrences — far more than random 4-uppercase-letter chance would predict for a buffer this
size (~91 expected by chance for one specific 4-letter combination in an 858KB buffer, vs. 325
actually found), a strong first signal this isn't coincidental. Inspecting the bytes immediately
before and after several occurrences confirmed a real, consistent record framing:

```
[recordLength: u32] "VAND" [const: u32 = 7] [bucket: u32] [index: u32] ...payload...
```

Six consecutive real records (raw bytes from the sample):

| record start (payload) | recordLength (field before tag) | const | bucket | index |
|---|---|---|---|---|
| 33706 | 336 | 7 | 16 | 5 |
| 34046 | 308 | 7 | 16 | 6 |
| 34366 | 312 | 7 | 16 | 9 |
| 34690 | 284 | 7 | 16 | 12 |
| 35026 | 336 | 7 | 16 | 25 |
| 35366 | 360 | 7 | 16 | 26 |

Across all 325 records in the sample: the `const` field is **always exactly 7** (a format
version or record-type marker). The `bucket` field only ever increases (observed range 16–36
across the file) and, critically, the `index` field **resets** every time `bucket` increments —
e.g. bucket 16 sees index values `5, 6, 9, 12, 25, 26, 27, 28` before bucket steps to 17 and
index restarts near `4, 5, ...`. This is the signature of a spatial grid: `bucket` behaves like
a cell/tile identifier that increases roughly monotonically with position in the file (and
presumably with position in the level), and `index` is a per-cell record counter — exactly what
you'd expect from a navmesh baked and serialized cell-by-cell.

**The `recordLength` field is a real, if slightly imperfect, framing size.** Comparing the gap
between consecutive `"VAND"` tag positions against the `recordLength` field read just before the
*current* record's tag: they match exactly in 251 of 324 gaps checked (77%). The remaining ~23%
are off by a small positive multiple of 4 (commonly +8, +12, +16, +20, +24, +36 bytes) — i.e.
`recordLength` slightly undercounts the true record span some of the time, in a way that's
always a clean multiple of 4, suggesting either an optional trailing sub-element some records
have and others don't (plausible for a navmesh polygon with a variable number of edges/portals),
or that `recordLength`'s own true meaning is closer to "the fixed-size portion of this record"
than "the whole record." Not resolved — this is exactly the kind of ambiguity that stalled
`.anim`'s own per-bone-record-width search for a while (see `research/animations.md`), so it's
flagged rather than papered over.

## What's inside one record's payload: real, spatially-clustered coordinates

Dumping one full record's payload (the 336-byte record at position 33706) as both `uint32` and
`float32` and inspecting by eye finds a clear internal shape:

- The first 11 `u32`-sized slots (payload offsets 0–40, right after `bucket`/`index`) are small
  integers — see "The 11-field prefix" below for what's now confirmed about them.
- Three floats immediately after that (payload offsets 44, 48, 52: `1.8`, `0.5`, `0.1`) are a
  **confirmed record-wide constant** — identical in every `VAND` record checked (15+ records
  across multiple buckets, all reading exactly `1.8, 0.5, 0.1`). The `0.5` in this triple is the
  same constant value found in the unrelated 212-byte "edge" records described above — a real
  cross-structure link, not yet explained.
- Starting at payload offset 56, a run of `float32` values that are unmistakably **real,
  physically clustered level-space coordinates**: `-84.11, -216.47, -311.79, -71.11, 352.84,
  -298.79, 10.0, -71.11, 26.03, -305.39, -71.11, 26.03, -311.79, -77.72, 26.03, -311.79, ...`.
  Several values repeat exactly 3-4 times within one record (`-311.79034423828125` four times,
  `-71.114990234375` three times, `26.0284423828125` three times) — strong evidence of a
  triangle-fan or shared-edge polygon representation, where adjacent triangles/edges legitimately
  reference the same vertex position multiple times when stored inline rather than by index.

### The 11-field prefix: real invariants, and a confirmed vertex-count formula

The 11 `u32` values between `index` and the `1.8/0.5/0.1` constant triple (payload offsets
0–40) aren't random — comparing them across all 325 records in the sample turns up exact,
load-bearing invariants. **Two separate fields matter here — an early mistake in this research
conflated them, worth calling out rather than silently fixing**: `prefix[2]` (with its
`==prefix[5]==prefix[10]` echo) is one real, confirmed relationship; `prefix[3]` is a
*different* field that turned out to be the one driving vertex/coordinate count. They were
briefly mixed up mid-investigation before being caught by a script that (correctly) used
`prefix[3]` disagreeing with hand-analysis that had used `prefix[2]` — re-verified carefully
below, this section reflects the corrected, re-checked version.

- **`prefix[0] == prefix[1] == prefix[9] == 0`, always**, in all 325 records.
- **`prefix[2] == prefix[5] == prefix[10]`, always** — the same value appears in three separate
  slots of the prefix (311 of 325 records; the other 14 are a distinct sub-shape, see below).
  Its own meaning is still unconfirmed (see Open Questions) — it is *not* the vertex-count field.
- **`prefix[8] == 2 × prefix[2]`, exactly**, in the same 311 records.

Calling **`prefix[3]`** value **N** (a genuinely different field from the `prefix[2]/[5]/[10]`
trio above), the total count of physically-plausible coordinate-range floats found in the
record's trailing geometry data (values with `0.5 < |v| < 2000`, a heuristic filter, but a
consistent and deterministic one) fits an exact linear formula — **refined twice this round,
each refinement strictly improving the match rate rather than replacing a wrong idea with
another guess**:

1. First pass: `3×N + 7`, matching every record with `prefix[2] == 1` exactly (verified across
   N = 3, 4, 5, 6 — 100% of the 42 such records in the sample), but failing on roughly 58% of
   `prefix[2] > 1` records.
2. **Corrected formula: `coordinateFloatCount = 3×(N + prefix[6]) + 7`.** Checked
   mechanically against all 311 "clean" records (not just a hand-picked few): matches **267 of
   311 exactly (86%)**, a large jump from the first pass's ~42%. `prefix[6]` — previously noted
   only as "a rare nonzero flag" — turns out to be a real, small, per-record **extra-vertex
   count** that adds directly to N, confirmed by the exact `diff == 3×prefix[6]` relationship
   holding for the 267 corrected matches. The remaining 44 records that still don't match are
   very likely a heuristic-detection problem (the `0.5 < |v| < 2000` "plausible float" filter is
   inherently noisy — it can both over- and under-count on records with enough raw bytes or
   coordinates that happen to fall outside that arbitrary range), not evidence the formula itself
   is wrong: the residual mismatches are frequently *not* clean multiples of 3, unlike every
   confirmed real discrepancy found so far in this file, which is the signature of measurement
   noise rather than a missing structural term.

**Crucially, `prefix[2]` (the sub-fan-count-shaped field) does *not* appear anywhere in the
corrected coordinate-count formula** — a `prefix[2]=2` record with `prefix[6]=0` and `N=7`
still has exactly `3×7+7=28` coordinate floats, verified byte-for-byte on a real record
(`reclen=568`, prefix `(0,0,2,7,17,2,0,3,4,0,2)` — all 28 floats from payload offset 76–184 read
in the expected coordinate range, no more, no less). So whatever `prefix[2]` counts, it isn't
"more total vertices" — the coordinate run's total size depends only on N and `prefix[6]`.

The cleanest physical reading of `3×(N+prefix[6]) + 7`: if `N+prefix[6]` is a **triangle-fan/
strip count** (this cell's local polygon has that many triangles, with `prefix[6]` contributing
some extra), a strip needs exactly `(N+prefix[6]) + 2` unique vertices —
`3×((N+prefix[6])+2) = 3(N+prefix[6]) + 6` floats — plus **one further trailing float**
unaccounted for (`+1`, giving the observed `+7` overall). This is a strong numeric fit (matching
exactly, integer-for-integer, across 267 independently-checked records — far beyond a
coincidence) but is still a hypothesis for what the combined count *means* semantically, not a
fully derived byte layout: the trailing `+1` float's role, and exactly how the vertices map onto
the observed value repetition pattern (described above), aren't nailed down yet.

### `prefix[2]`: very likely a sub-fan count, but the sub-structure itself isn't cracked yet

Immediately after the coordinate run, a fixed-width 11-`u32` block was found with two exact,
cross-record-constant fields — its own field `[7]` always exactly `1`, and field `[8]`
apparently echoing N. **This held with zero exceptions across the first 7 records checked
(spanning N=3 through N=6) — but every one of those 7 records happened to also have
`prefix[2] == 1`.** Extending the check to all 311 clean records (using the *corrected*
`3×(N+prefix[6])+7` coordinate-count formula to locate this block precisely) shows the
`field[8] == N` echo **only holds for `prefix[2] == 1` records** (40 of 40 match); every
`prefix[2] > 1` record has some *other*, smaller value in that position instead — e.g. a real
`prefix[2]=2, N=7` record has `field[8] = 4`, not `7`.

**The natural, well-motivated hypothesis this points to**: `prefix[2]` is a count of separate
**sub-fans** packed into one `VAND` record (hence its doubling in `prefix[8] = 2×prefix[2]`,
and its three-way echo in `prefix[2]/[5]/[10]`), N is the **combined** triangle count across all
of them, and the single 11-field block found right after the coordinates is actually the first
of `prefix[2]` per-sub-fan sub-headers — each with its own local triangle count (in the observed
`prefix[2]=2` example: `4` and presumably `3`, summing to the outer N=7) and, presumably, its
own edge/neighbor list. **This has not been confirmed or fully mapped out** — the coordinate
run itself is *not* split per sub-fan (it's one combined run of `N+prefix[6]` triangles' worth
of vertices, not `prefix[2]` separate runs), so any per-sub-fan structure must be confined to
the post-coordinate tail region alone, and its exact shape (how each sub-header's own edge list
is delimited, how to find where sub-fan *k*'s section ends and sub-fan *k+1*'s begins) is
unsolved. This is the clearest next target for this format.

**A genuinely solid structural fact, corrected twice before landing on the right number — both
corrections kept here rather than silently smoothed over, since each one meaningfully narrowed
the truth**: a `[field=1, count]` marker pair recurs at a **fixed 40-byte period**, confirmed
directly (not inferred) across a `prefix[2]=3` record showing three such pairs at exactly
offsets 28, 68, and 108 past `tail_start` (28, 68, 108 — each 40 apart) — the count of pairs
found (3) matches `prefix[2]` exactly for that record, and 19 of 19 real `prefix[2]=2` records
confirmed a pair at the fixed second position (68). The initial framing of this ("an 11-`u32`,
44-byte sub-header, with a second one 68 bytes past the *first sub-header's own start*") was
subtly wrong: 68 is actually the gap between the two `[1,count]` *pairs themselves*, not between
two full sub-header blocks — the true repeating unit is 40 bytes (10 `u32`), with the marker
pair sitting 28 bytes into each unit, not a separate 44-byte block per sub-fan. This matters for
anyone continuing this research: don't re-derive "68" as a block width, it isn't one.

**What's NOT confirmed**: the guess that `count1 + count2 == N` (i.e. that N is simply the
sub-fans' triangle counts added together) only happened to hold in 10 of 19 `prefix[2]=2`
records checked; in the other 9, the value genuinely present at the correct, fixed offset is
some *other* number, unrelated to `N − count1` by any simple relationship spotted so far (e.g.
one real record has `count1=6, N=7`, so `N−count1=1`, but the real second-pair value there is
`3` — not a near-miss, a different number entirely, sitting at the exact right offset). So the
fixed-period marker location is solid; the *semantic* assumption that these counts are "N split
across sub-fans" is specifically what's ruled out. What `count1`/`count2`/`count3`/... actually
measure — if not a simple split of N — is the open question, not where to find them. The
32-byte gap between consecutive marker pairs (offsets +8 to +36 relative to each pair) itself
has visible internal structure not yet decoded (compared byte-for-byte across two real gaps in
one sample record, position 0 was `0` in both, and position 4 loosely tracked the *following*
pair's own count in one case but not clearly in the other) — a plausible next target once the
marker/count semantics themselves are better understood.

**The `prefix[4]` edge/neighbor-count hypothesis remains promising but unconfirmed generally,**
for the same underlying reason: two `prefix[2]==1` records were hand-inspected and matched
`prefix[4]` exactly (a mix of clean `[neighborID, 0, 0]` triples and `0xFF`-prefixed sentinel
entries, summing to `prefix[4]`), but a mechanical re-check across more records didn't reproduce
it — almost certainly because most real records have `prefix[2] > 1` and therefore a
multi-sub-fan tail structure the simple "one edge list right after one leadin" model doesn't
capture at all, not because the underlying hypothesis about individual edges is wrong.

**A 14-record sub-shape breaks the `prefix[2]==prefix[5]==prefix[10]` invariant.** These are the
largest records in the file (`recordLength` 800–8616 bytes, vs. ~300-450 for the "clean"
majority) — e.g. one has `prefix = (0,0,42,68,172,36,3,65,72,6,36)`, where `prefix[2]=42` but
`prefix[5]=36` and `prefix[10]=36` (matching each other, but not `prefix[2]`) — plausibly a
larger/more-complex polygon shape where a 4th distinct count comes into play, not yet
investigated further.

**`recordLength` vs. the true measured gap to the next record**: confirmed more precisely this
round — for records that keep the `prefix[2]==prefix[5]==prefix[10]` invariant, the true gap
(tag-to-tag) equals `recordLength + 4` exactly in 238 of 309 checked cases (77%), with the
remainder needing a further positive multiple of 4 (see the original finding below) — unchanged
from the initial investigation, still unresolved.

## Open questions

1. **What the per-pair `count` value (at each `[1, count]` marker) actually measures**, now
   that "they sum to N" is specifically ruled out (confirmed wrong in 9 of 19 real `prefix[2]=2`
   cases, with no near-miss pattern — a genuinely different number, not a rounding or off-by-one
   issue). The *location* of every marker pair is solved — a fixed 40-byte period, confirmed
   directly with 3 pairs found at the expected offsets (28, 68, 108) in a real `prefix[2]=3`
   record, matching `prefix[2]`'s own count exactly — so this question is purely about what the
   numbers mean, not where they are. Worth checking against a per-sub-fan edge/neighbor count
   instead of a triangle/vertex count (i.e. maybe these are more directly related to `prefix[4]`
   than to N). Also open: what the other 32 bytes of each 40-byte unit hold (some internal
   structure was glimpsed but not decoded — see above), and whether the 40-byte period holds for
   `prefix[2] > 3` too (only checked up to 3 so far).
2. **`prefix[4]`'s exact byte-level edge/neighbor-list layout**, now understood to need the
   sub-fan structure above solved first — the earlier 2-record hand-match (a mix of
   `[neighborID,0,0]` triples and `0xFF`-prefixed sentinels summing to `prefix[4]`) only used
   `prefix[2]==1` records, so it was never actually contradicted by anything; it just can't be
   checked broadly until each sub-fan's own edge-list boundary is found.
3. **Why `recordLength` sometimes undercounts the real gap to the next record** (~23% of
   records, always by a positive multiple of 4) — still unresolved. Now narrowed slightly: this
   is independent of the 14-record "large polygon" sub-shape (which mostly does *not* show this
   discrepancy), so it's a separate phenomenon from whatever distinguishes those 14 records.
4. **The 14-record sub-shape** where `prefix[2] != prefix[5]`/`prefix[10]` — likely a 4th
   distinct count relevant only to larger/more complex polygons. Not investigated beyond noticing
   the pattern breaks cleanly (no ambiguous middle ground: a record's prefix either fully
   satisfies the invariant or doesn't).
5. **What occupies the ~28KB of `ARNM` header between the 22-record "edge" block (ending at byte
   4852 in the sample) and the first `VAND` record (byte 33706).** Doesn't match the edge-record
   marker anywhere in that range; raw values look hash/ID-like rather than geometric, with some
   exact-repeat values recurring at small regular offsets (a real, if unexplained, structural
   signal worth investigating with the same repeat-position technique that worked for the
   `VAND`/edge-record markers themselves).
6. **What `prefix[5]`/`prefix[7]` of the `ARNM` 8-field header count** — both confirmed to
   always be exact multiples of 256 (like `field3`, whose ×256 formula IS understood), but their
   own targets aren't identified yet. Worth checking against the total edge-record count (22 in
   the sample), the total `VAND` record count (325), or a sum of some per-record field across the
   whole file.
7. **The trailing `+1` float in the `3×(N+prefix[6])+7` coordinate-count formula**, and the
   exact vertex-to-byte-offset mapping within the coordinate run.
8. **The ~14% of records (44 of 311) where the coordinate-count formula still doesn't match**
   exactly — very likely a limitation of the "plausible float" *detection heuristic* (residual
   errors aren't clean multiples of 3, unlike every confirmed real structural discrepancy in this
   file), not evidence against the formula itself, but not proven either way yet.

## Resolved this round

- ~~Whether the file has any content before the first `"VAND"` record~~ — yes: an 8-field
  `ARNM` header (5 of 8 fields are cross-sample format constants; field3 has an exact
  `256×(totalLen−37)` formula), then 22 fixed-size 212-byte "edge" records with real paired
  coordinates, then ~28KB of still-unidentified content (see Open Question 5).
- ~~Grid semantics: is `bucket` a literal spatial cell index~~ — yes, confirmed via cross-sample
  bucket-distribution matching between two navmesh variants of the same level (see above).
- ~~Cross-file confirmation~~ — done: two additional real samples (`m_hellbase_actor.navmesh`,
  `m_italiancity_actor.navmesh`) pulled and compared; the differential approach that worked for
  `.anim` worked here too, most decisively for confirming the `ARNM` header constants and the
  `bucket` = grid-cell hypothesis.
- ~~The coordinate/vertex-count formula~~ — **fully generalized this round**: from an initial
  `3×N+7` (matching only `prefix[2]==1` records) to the corrected `3×(N+prefix[6])+7`, matching
  86% of all 311 clean records mechanically (not hand-picked), independent of `prefix[2]`'s
  value — the strongest, most broadly-verified numeric crack in this file so far.
- ~~Locate the 2nd (and further) sub-fan marker for `prefix[2]>1` records~~ — done: a
  `[1, count]` marker pair recurs at a fixed 40-byte period starting 28 bytes into the
  post-coordinate tail, confirmed directly with 3 pairs in a real `prefix[2]=3` record matching
  `prefix[2]`'s own count exactly. What the `count` values mean remains open (see Open Question
  1) — this round cracked *where* they are, not *what* they say.

## Suggested next steps

- Now that every sub-fan marker's location is known, try correlating the `count` values against
  `prefix[4]`/`prefix[7]` (rather than N) across many `prefix[2]=2` and `=3` records — the
  natural next hypothesis given "sums to N" is now ruled out.
- Decode the other 32 bytes of each 40-byte unit (between one marker pair and the next) — some
  internal structure was glimpsed (a leading `0` in both gaps compared) but not confirmed.
- Re-attempt the `prefix[4]` edge/neighbor-list count per-sub-fan rather than per-record, now
  that sub-fan boundaries are locatable — the original 2-record hand-match was never actually
  disproven, just untestable at record-level for `prefix[2]>1` records.
- Apply the repeat-position/marker-search technique (used successfully for the 212-byte edge
  records) to the still-opaque ~28KB middle section of the header.
- Pull a `.navmesh` sample from a much larger/more open level than the three checked so far
  (all reasonably contained industrial/urban levels) to see whether the grid dimensions or
  header constants change in a revealing way at a very different scale.
- If the coordinate data and vertex-count formula are confirmed complete enough, the payoff
  would be genuinely new: a visualizable navmesh overlay (even just as a debug
  point-cloud/wireframe export similar to how `mesh.go`'s geometry is exported today) —
  something no chunk type this project currently understands provides.
