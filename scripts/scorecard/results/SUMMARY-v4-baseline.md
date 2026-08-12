# v4 dogfood baseline — the composition band separates B from C

**Date:** 2026-08-12 · **Questions:** v4 (26 = all 22 v3 verbatim + P01–P04 composition)
**Commit:** af0ea09 (`feat/scorecard-v4`; binary reports `-dirty` from an uncommitted
OpenSpec tasks.md tick only — no Go source differed from HEAD)
**Corpus:** `git archive HEAD` minus `scripts/scorecard/`, 6,240 entities (v3-era runs
recorded ~5,410; growth is repo growth since, same construction)
**Stack:** scorecard-v4 compose project, NATS 25222, semembed host 28081, arm64/Darwin (M3 Pro)
**Gates:** check-composition + check-discrimination CLEAN on the exact corpus; repeats=3, zero UNSTABLE.

## The verdict this run exists for

**Separation, not parity.** The saturated v3 instrument could not distinguish the
fusion layer from raw cosine (22/22 both since 2026-08-09). The composition band —
questions whose facts provably cannot co-occur in one passage (checker-gated) — splits
them decisively:

| arm | composition | context on band | how it fails |
| --- | --- | --- | --- |
| A (grep floor) | **0/4** | 70,547 B | single-symbol query greps to one file; no file carries both facts, by construction |
| B (MCP) | **4/4** | **7,798 B** | — (`code_impact` names each cross-file dependent) |
| C (cosine top-20) | **0/4** | 72,172 B | every question missing one required caller name — 20 bodies of similarity cannot substitute for one edge set |

The query/fusion layer's structural traversal now measurably earns its keep: recall
cosine cannot reach, at ~9× less context on the band. This was the C-parity question
gating the next tag, and the answer is separation.

## Full table (v4, this corpus, this machine)

| band | A recall/ctx | B recall/ctx | C recall/ctx |
| --- | --- | --- | --- |
| code | 6/6 · 186,995 B | 6/6 · 65,938 B | 6/6 · 101,339 B |
| composition | 0/4 · 70,547 B | **4/4 · 7,798 B** | 0/4 · 72,172 B |
| discrimination | 0/2 · 50,490 B | 2/2 · 37,789 B | 2/2 · 33,755 B |
| doc-early | 3/3 · 99,549 B | 3/3 · 47,825 B | 3/3 · 41,992 B |
| doc-late | 5/7 · 423,884 B | 7/7 · 162,514 B | 7/7 · 117,029 B |
| impact | 2/2 · 42,277 B | 2/2 · 4,566 B | 2/2 · 32,252 B |
| negative | 2/2 · 45,260 B | 2/2 · 21,000 B | 2/2 · 26,165 B |
| **TOTAL** | **18/26 · 919,002 B** | **26/26 · 347,430 B** | **22/26 · 424,704 B** |

Schema overhead (B only, session-fixed, never folded in): 7,087 B / 9 tools.
The v3 subset still grades 22/22 on B and 22/22 on C — saturation there is real and
unchanged; only the new band moves, which is exactly what a restored instrument should do.

## Latency — first measurement ever (median/p95 ms, first call per question)

| band | A | B | C |
| --- | --- | --- | --- |
| ALL | 653 / 1,133 | **132 / 220** | 2,008 / 2,080 |
| composition | 224 / 229 | 158 / 348 | 2,000 / 2,004 |
| code | 521 / 653 | 71 / 84 | 2,008 / 2,152 |

Read arm C's column honestly: ~2 s is the **harness** (an awk cosine over all 6,240
vectors plus an embed round-trip), not a product surface — its value here is bounding
what a naive vector-scan integration would pay. Arm B is the only column that is a real
product figure, and it is also the fastest thing on the board: ~5× faster than the grep
floor's procedure. Same-machine rule applies (arm64/Darwin M3 Pro throughout).

## Observations for the record

- **Dangling call edge** (pre-existing, unrelated to v4): graph-query logs
  `not found: ...function.processor-supersession-lifecycle-go-stat` — the Go call
  indexer emits a call edge to `stat`, a function *parameter* of
  `decideLifecycleActions`, which never gets an entity. Cosmetic in this run; worth an
  upstream-shaped look at whether parameter calls should be skipped at emit time.
- **Zero UNSTABLE across 78 graded calls** (26 × 3 repeats) — second consecutive run
  with none since beta.160.
- Composition answers from B are also the *cheapest* answers in the entire table
  (~1,950 B mean): naming edges is smaller than shipping passages.
