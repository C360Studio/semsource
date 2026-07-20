# Scorecard A/B — choosing the passage ceiling and floor (task 9.5)

> **Correction (2026-07-20).** The `discrimination 0/2` result below is **not a retrieval
> finding** and must not be quoted as one. X02 was graded by a matcher that could not evaluate
> its literals (a `grep` option-parsing bug) and failed on every system regardless of content;
> it retrieves correctly today. X01's failure is real but is a two-position ranking miss, not
> the total absence described below. See
> [`SUMMARY-instrument-diagnosis.md`](SUMMARY-instrument-diagnosis.md).
>
> **The bounds conclusion is unaffected and stands** — a 4x ceiling range changed no graded
> outcome, and every non-discrimination band scored identically across all three.

questions.json **version 2**. Corpus held fixed: `git archive` of `d554bcc`, 206
ingestable documents / 1.17 MB, `scripts/scorecard/` excluded (it quotes both
literals of every discrimination question, so ingesting it plants a
guaranteed-IMPRECISE passage — the apparatus corrupting the measurement). Only
the binary varied; `docker compose down -v` between every run.

| bounds (ceiling/floor) | score | entities | median top-node body | mean total body |
|---|---|---|---|---|
| 1000/200 (fine)        | 20/22 | 5698 | 571 B  | 11,209 B |
| **2000/400 (shipped)** | 20/22 | 5401 | 1,228 B | 15,978 B |
| 4000/800 (coarse)      | 20/22 | 5219 | 1,362 B | 25,657 B |

Per-band, all three runs were **identical**: code 6/6, doc-early 3/3, doc-late
7/7, impact 2/2, negative 2/2, discrimination 0/2. Zero fabrication throughout.

## The result

**A 4x range of ceilings changed no graded outcome.** The only thing that moved
was evidence size: the finest bounds carry 2.3x less body per answer than the
coarsest for exactly the same answers.

**This is a real answer, not a failed measurement — but it is a narrow one.** The
doc bands are *saturated*: 10/10 combined on every side. A saturated band cannot
show improvement, only regression, so this A/B establishes that none of these
bounds breaks anything and that finer bounds cost less evidence per answer. It
does not establish that finer bounds retrieve better, because nothing here could
have shown that.

It also cannot see the downside of going too fine. A passage small enough to lose
the context around a fact would show up as a doc-band miss, and the doc bands are
too easy to register it. 1000/200 scoring the same as 2000/400 is evidence of "no
harm at this granularity on this corpus", not evidence of "smaller is better".

## Recommendation

**Keep 2000/400.** Nothing measured justifies moving, and two weak signals favour
staying:

- The offline sweep (`TestBoundsSweep`) shows the share of under-floor passages is
  *minimised* at 2000/400 (7.0%); both 1000/200 (10.0%) and 4000/800 (12.5%) are
  worse. Under-floor passages are the ones the floor exists to merge away.
- 2000/400 sits mid-range on entity cost (5401 vs 5698 / 5219).

Changing a default on evidence that explicitly cannot distinguish the options
would be motion, not improvement.

## What would actually decide this

The graded set is the binding constraint, not the stack. To make the bounds
measurable, the question set needs items that a *saturated* doc band cannot
already answer:

1. **Working discrimination questions** (see below — the current two are blocked
   by a ranking defect, not by chunking).
2. **Multi-fact questions** where the answer needs two facts from one section, so
   an over-fine passage splits them and genuinely fails.
3. **Context-dependent questions** where a fact is only correct given the heading
   above it, penalising passages cut too small.

## Defect this measurement found: ranking prefers the override over the default

Both discrimination questions failed as `miss`, not `IMPRECISE` — the top node did
not contain the answer *at all*, rather than carrying it alongside its twin. That
is not the chunking failure the band was designed to catch. Chunking worked; the
passages were correctly bounded. Ranking chose wrong:

- **X01** — top node is `README.md § Quick Start`, a clean 711-byte passage that
  contains `NATS_MONITOR_HOST_PORT=28222`, the port-*conflict workaround*. The
  query asked for the **default**. The section holding the default
  (`§ Configuration`) never surfaced. Answering "what is the default X" with the
  section about **overriding X** is a plausible-looking wrong answer, which is the
  most expensive kind.
- **X02** — the Tier 2 (`seminstruct`, 8083) section never reaches the top 5;
  ranks 4 and 5 are both Tier 1 (`semembed`, 8081) content.

This reproduces identically at all three bounds, confirming it is independent of
passage size.

**A grading gap this exposed.** There are three states, and only two are named:

| top node holds | verdict today | should be |
|---|---|---|
| answer, no twin | `correct` | `correct` |
| answer **and** twin | `IMPRECISE` | `IMPRECISE` |
| twin, **no** answer | `miss` | its own verdict — this is *misleading*, worse than absent |

Scoring the third as a plain miss understates it: the evidence does not merely
fail to answer, it argues for the wrong answer.

## Method notes

- Retrieval was verified **deterministic** before any result was trusted — three
  identical calls returned the identical top handle and body length. An earlier
  debug query appeared to show a body-less top node (which would have meant the
  10.3 defect regressed); that was a mis-parse of the SSE stream picking up a
  non-result frame, not a regression.
- Readiness gated on the product's own three signals (`phase` + `index.ready` +
  `embedding.ready`), never a fixed sleep.
- `SCORECARD_EMBED_SETTLE` was documented as a knob but is not read by `run.sh`;
  removed from the README rather than left advertising a no-op.
