# v1.0.0-beta.10 released image, tier 0, regraded against the alpha.6 run (#237)

This is the [`SUMMARY-alpha6-tier0.md`](SUMMARY-alpha6-tier0.md) run repeated on the **released**
`v1.0.0-beta.10` image instead of a local build: same corpora, question sets, source configuration,
grader and machine. **Every verdict, every miss, and the byte totals match alpha.6. The R1 gate
does not hold: one question reorders two adjacent results at the tail of its list (see "R1").**

**Date:** 2026-11-21. **Machine:** M3 Pro (arm64/Darwin). **Sets:** dogfood `questions.json` v7
as on main at `86626c0` (the version the alpha.6 run graded; see "D10"), OSH
`questions-osh.json` v3. Repeats = 3. Verdicts are `correct` or `miss` only.

## What ran

| | |
|---|---|
| SemSource | the released core image `ghcr.io/c360studio/semsource:1.0.0-beta.10@sha256:98584d13e4024b0792802e7bd4550b784e2900f508309056e4fc9041150850f3`, published by tag run [44797269782](https://github.com/C360Studio/semsource/actions/runs/44797269782) from `9537f59` |
| SemEngine | `github.com/c360studio/semengine v0.1.0-alpha.6`, `embedding_enabled` unset |
| Config | [`beta10-tier0/tier0.json`](beta10-tier0/tier0.json), byte-identical to the alpha.5 and alpha.6 runs' |
| Corpora | dogfood `b4c2cfd9026b20b5` (8,192 entities), OSH `9b1e77bc0bf0291f` (29,422 entities); hashes rechecked |
| Stack | `docker-compose.yml` (NATS `2.14.10` pinned by digest) plus [`beta10-tier0/compose.semb10.yml`](beta10-tier0/compose.semb10.yml); fresh NATS storage per corpus leg |

## Result

| | alpha.6 (local build) | beta.10 (released image) |
|---|---|---|
| dogfood, arm B | 32/36 · 559 KB | **32/36 · 559 KB** |
| dogfood latency median / p95 | 58.5 / 169 ms | 59 / 178 ms |
| dogfood misses | X02, P03, L02, L07 | X02, P03, L02, L07 |
| OSH, arm B | 12/13 · 175 KB | **12/13 · 175 KB** |
| OSH latency median / p95 | 65 / 146 ms | 71 / 152 ms |
| OSH misses | C03 | C03 |
| R1 (restart, sources reversed) | 36/36 identical, RANK-STABLE | **35/36 identical, 1 reordered: RANK-UNSTABLE** |
| setup03a BM25 lane | 67 observations pass | **67 observations pass** |

Latency is one run each on a shared laptop and is recorded, not judged. Both columns use the
same convention: each question's `latency_ms` over all questions, median, and p95 as the sorted
value at index `round(0.95 × (n − 1))`. Recomputed that way from the committed alpha.6 JSON, this
convention gives the alpha.6 summary's own 169 and 146 ms. It is not `run.sh`'s per-band p95
(`sorted[ceil(0.95n) − 1]`), which would read 191 / 177 ms here.

## R1

[`beta10-tier0/r1-evidence/`](beta10-tier0/r1-evidence/) holds the restart evidence: the container
was recreated with `tier0-reversed.json`, and the new process logged all three initial ingests
before leg 2 was scored. `rank-diff.sh` reports one question out of order:

- **L03** (`code_search`, loose band, verdict `correct` in every leg): ranks 18 and 19 of 20 swap.
  The two entities are `cmd/semsource/config_drift.go` `retainedOnlyHint` and
  `handler/doc/entities.go` `Entity`. No other question differs at any rank.

Handles carry the platform authority minted per fresh store, so the comparisons below strip the
first two ID segments.

| leg | source order | L03 rank 18 | the other 35 questions |
|---|---|---|---|
| alpha.6 leg 1 | `tier0.json` | `Entity` | — |
| alpha.6 leg 2 | reversed | `Entity` | identical to alpha.6 leg 1 |
| beta.10 leg 1 | `tier0.json` | `retainedOnlyHint` | identical to alpha.6 |
| beta.10 leg 2 | reversed | `Entity` | identical to alpha.6 |
| beta.10 repeat, fresh storage ([`beta10-tier0-dogfood-rep.json`](beta10-tier0-dogfood-rep.json)) | `tier0.json` | `retainedOnlyHint` | identical to beta.10 leg 1 |

So on the released image the L03 tail order follows the source order and repeats with it. Every
other ranking in all five legs is identical. The fusion lens sorts by score and breaks ties by
the position the upstream search returned
(`semengine@v0.1.0-alpha.6/pkg/fusion/engine_lens.go:578`). An insertion-order tie in the word
index would produce exactly this, but this run has no scores for the two entities
(`graph_search` returns its top 25 only, with no limit argument), so a tie is the likely
explanation, not a proven one. The follow-up is #324.

## D10

The dogfood corpus is the frozen `758a805` tree, whose `configs/tiers/README.md` names alpha.5.
Main's `questions.json` now expects alpha.6 for D10, as corpora cut from later trees do. These
legs graded with `questions.json` from `86626c0`, the version the alpha.6 run used, so D10 is
comparable with the alpha.6 row.

## setup03a

`SETUP03A_PROFILE=bm25 SETUP03A_IDENTITY=governed`, `TestKnownAnswerCorpus`, 67/67 observations
pass. The binary was built from `9537f59` (the tagged commit) on darwin/arm64, SHA-256
`85d640854c5d5e17f8427268a22322f4f6261dc9dbc720637c683fe04d8b9691`. The harness exercises
the binary, not the image. Report:
[`beta10-tier0/setup03a-bm25-report.json`](beta10-tier0/setup03a-bm25-report.json).

## Replay

As in `SUMMARY-alpha5-tier0.md` "Replay", with these substitutions:

- `SRC` is `9537f59b5d30149e7b9a1c456d6f7599cf618c28` (the tagged commit), not `e5d90d3`:
  `git -C "$ART" worktree add --detach "$W/src" 9537f59b5d30149e7b9a1c456d6f7599cf618c28`, and check
  `git -C "$SRC" rev-parse HEAD` before building the setup03a binary from it.
- `semb10` / `SEMB10_*` for `sem233` / `SEM233_*`, and `beta10-tier0` for `alpha5-tier0`.
- `SCORECARD_QUESTIONS` is `questions.json` from `86626c0` for the dogfood legs
  (`git -C "$ART" show 86626c0:scripts/scorecard/questions.json > "$W/questions-86626c0.json"`).
- No image build: `compose.semb10.yml` pins the released image by its index digest, so the tag
  can't drift.
- The repeat leg is one more fresh-storage dogfood run with `tier0.json`, labelled
  `beta10-tier0-dogfood-rep`.

## Not covered

- **Tier 1** waits on 04C (#234).
- **R1 on OSH** was not run, as in the baseline.
- **Arm A and arm C** were not rerun; they don't depend on the SemSource image.
- **The UI image** (`semsource-ui`) is verified by `ui-release-smoke` in the tag run, not by
  this scorecard.
