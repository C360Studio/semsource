# questions.json version 3 — baseline on the repaired instrument

**Date:** 2026-07-20. **questions.json version 3.** Corpus held fixed: `git archive d554bcc`,
206 ingestable documents, `scripts/scorecard/` excluded. Stack: `COMPOSE_PROJECT_NAME=ranktest`,
`configs/mvp.json`, 5410 entities, gated on `phase` + `index.ready` + `embedding.ready`.
`SCORECARD_REPEATS=3`. Result file: `v3-baseline.json`.

This is the first score taken on the repaired grader, and the reference point future runs compare
against. **It does not compare to any version-2 number** — see the comparability note in
[`../README.md`](../README.md).

## Result

**20/22.** code 6/6 · doc-early 3/3 · doc-late 7/7 · impact 2/2 · negative 2/2 ·
discrimination 0/2. Zero fabrication.

| id | verdict | note |
|---|---|---|
| X01 | `MISLEADING` | top node carries `NATS_MONITOR_HOST_PORT=28222`, not the default `=8222` |
| X02 | `UNSTABLE` | verdict varied across 3 calls: `correct` / `miss` |

## The prediction was wrong, and that is the finding

The change's tasks predicted **X02 `correct`, X01 `MISLEADING`, discrimination 1/2**, and recorded
that prediction as something to check rather than to confirm. Half of it held.

X01 is `MISLEADING`, as expected — the verdict that did not exist before v3, now correctly
describing the defect `fix-default-vs-override-ranking` exists to fix.

**X02 is `UNSTABLE`, not `correct`.** The prediction assumed the matcher fix alone would settle it,
because X02 retrieves correctly. It does retrieve correctly — on two of three calls. Asked last in
the run, the first call still loses the passage to the upstream fusion defect (ask #24), and the
repeats disagree.

So the honest answer is that **X02 has no defensible verdict on this platform**, and the instrument
now says so instead of picking one. Under the old grader this question reported a flat `miss` and
looked like a ranking failure; under a warm-up or retry-until-pass it would report `correct` and
look fixed. Neither would have been true. This is design decision D3 doing exactly the job it was
chosen for, on the first run where it mattered.

**Consequence for reading the score:** discrimination 0/2 here is *not* comparable to the 0/2 that
version 2 reported. That one was a grep bug plus an unmeasured flake. This one is one real product
defect (X01) and one unmeasurable question (X02) — and X02 becomes scoreable the moment ask #24 is
resolved upstream, with no change to SemSource.

## The body-less-node fix worked, and did not over-filter

`dropNavigationalNodes` was widened to cover body-less nodes with **no declared kind**, which is
what Go module dependency entities are.

- Dependency-entity nodes in doc answers: **1 → 0** (comparing the retained answer prefixes of
  `diagnostic-baseline` and `v3-baseline`).
- **Doc bands unchanged at 10/10.** They are saturated, so they cannot show improvement — they are
  here as the regression detector, and they detected nothing. That is the result being claimed: the
  filter removed non-evidence without removing evidence.
- Node counts fell slightly where those entities had been competing (N02 16 → 15, X01 20 → 19,
  X02 20 → 18), which is the filter's visible footprint.

## Known and deliberately not addressed here

**The code lens has an analogous body-less-lead case.** C04 (`code_search`) returns a top node
`entityid.go` with `kind: "file"` and no body — `top=0`. `dropNavigationalNodes` is applied to the
docs answer only (`processor/code-context/component.go:291`), so the code lens is untouched by this
change.

It is left alone deliberately, not overlooked:

- The kind is **declared** (`file`), so it is not the same defect. The rule this change extended
  turns on *declared-but-empty versus never-declared*, and a `file` node claiming to be a file is
  the former — the case the original author explicitly wanted to keep visible.
- C04 grades `correct`: its matchers run against the whole answer, so the fact is present.
- Whether a body-less file node is legitimate navigational context on the code lens is a product
  question about `code_search` semantics, not an instrument repair.

Worth revisiting on its own evidence. Recorded here so it is not rediscovered as a surprise.

## What this baseline is good for

- **The reference point for `fix-default-vs-override-ranking`.** If that change works, X01 moves
  `MISLEADING` → `correct` and nothing else moves. That is a single, unambiguous signal.
- **A standing reproduction of ask #24.** X02 reports `UNSTABLE` on every run until the upstream
  defect is fixed, so regression or resolution is visible without a hand-built probe.
- **Not** a measure of chunking bounds, which `SUMMARY-9.5-bounds.md` settled and this does not
  revisit.
