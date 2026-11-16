# SemEngine alpha.5 tier 0, graded against the 8b99efe9 floor (#233 part 2)

This is the graded 04B run. Main at `e5d90d3` (SemEngine `v0.1.0-alpha.5`, tier 0 search on
`graph-lexical`) was scored on the same corpora, question sets, source configuration, grader
and machine as the pre-cutover baseline in [`SUMMARY-pin-8b99efe9.md`](SUMMARY-pin-8b99efe9.md).
**Every condition of the milestone 3/4 floor holds.**

**Date:** 2026-11-16. **Machine:** M3 Pro (arm64/Darwin), the same one as the baseline.
**Sets:** dogfood `questions.json` v7, OSH `questions-osh.json` v3. Repeats = 3. No `error`,
`UNSTABLE`, `FABRICATED` or `MISLEADING` verdict appears in any run.

## What ran

| | |
|---|---|
| SemSource image | `ghcr.io/c360studio/semsource:e5d90d3` (CI build of main; arm64 manifest `sha256:1123f2c468ce…`, local image `sha256:e65c9d5c59d2…`) |
| SemEngine | `github.com/c360studio/semengine v0.1.0-alpha.5` |
| NATS | `nats:2.14-alpine` (`sha256:311da47e2f31…`), the same image as the baseline |
| Config | [`alpha5-tier0/tier0.json`](alpha5-tier0/tier0.json) = `configs/mvp.json` at main. Same sources as the baseline's tier 0 (ast + docs + config); the only difference is that the retired `embedder_type: bm25` is gone, and graph-lexical is the default |
| Corpora | the baseline's exact trees: dogfood `b4c2cfd9026b20b5` (8,192 entities, as at the pin) and OSH `9b1e77bc0bf0291f` (29,422 entities, as at the pin) |
| Stack | main's `docker-compose.yml` plus [`alpha5-tier0/compose.sem233.yml`](alpha5-tier0/compose.sem233.yml); fresh NATS storage per corpus |

Tier 1 and arm C are not run. SemEngine has no dense embedder until 04C (#234), and
`arm-c-cosine.sh` refuses to run without one. The baseline's tier 1 columns become the floor
for #234.

## Against the floor

| floor condition | baseline (8b99efe9) | alpha.5 | |
|---|---|---|---|
| dogfood tier 0, no band regresses | 23/36 | **32/36** | holds; 4 bands up, none down |
| OSH tier 0, no band regresses | 10/13 | **12/13** | holds; config 1/3 → 3/3, none down |
| lexical band flips at tier 0 | 1/3 | **3/3** | holds |
| R1 at tier 0 is RANK-STABLE | 10/36 identical | **36/36 identical** | holds |
| setup03a BM25 lane | 61/63 | **65/65 checks pass** | holds (67 observations) |

### Dogfood, tier 0, arm B

| band | baseline | alpha.5 | bytes, baseline → alpha.5 |
|---|---|---|---|
| code | 3/6 | **6/6** | 38 → 81 KB |
| composition | 3/4 | 3/4 | 8.7 → 8.0 KB |
| discrimination | 1/2 | 1/2 | 21 → 46 KB |
| doc-early | 2/3 | **3/3** | 26 → 68 KB |
| doc-late | 7/7 | 7/7 | 124 → 170 KB |
| impact | 2/2 | 2/2 | 4.6 → 4.2 KB |
| lexical | 1/3 | **3/3** | 34 → 27 KB |
| loose | 2/7 | **5/7** | 73 → 137 KB |
| negative | 2/2 | 2/2 | 19 → 18 KB |
| **TOTAL** | **23/36 · 349 KB** | **32/36 · 559 KB** | |
| latency median / p95 | 80 / 200 ms | 58 / 177 ms | |

alpha.5 tier 0 matches the baseline's *tier 1* total (32/36). It does so without an embedding
provider.

The bytes rose by 60%. Answers at tier 0 now return more passages with bodies: the word
index finds candidates that the SemStreams feature-hashed vectors never surfaced. This is
recorded here, not judged; it is the cost side of the code and loose gains.

The four remaining misses:

- **X02:** the top node lacks `-p 8083:8083`. This was also a miss at the pin.
- **P03:** `code_impact` still omits the caller `initializeWatchers`. The product gap recorded
  at the pin persists on SemEngine.
- **L02 and L07:** loose phrasing on `code_search`.

### OSH, tier 0, arm B

| band | baseline | alpha.5 |
|---|---|---|
| code | 2/3 | 2/3 |
| composition | 2/2 | 2/2 |
| config | 1/3 | **3/3** |
| doc | 2/2 | 2/2 |
| impact | 1/1 | 1/1 |
| negative | 2/2 | 2/2 |
| **TOTAL** | **10/13 · 190 KB** | **12/13 · 175 KB** |
| latency median / p95 | 73 / 180 ms | 58 / 117 ms |

G02 and G03 go green. `graph_search` on alpha.5 returns match properties that carry dependency
versions (#292), which closes the shape gap the config band was waiting on (semstreams#958 at
the pin). C03 (`code_search`) is the only miss, as at the pin.

### Lexical band (semengine#10's 04B questions)

| | baseline tier 0 | alpha.5 tier 0 |
|---|---|---|
| LX01 `"docker compose down -v"` | correct | correct |
| LX02 `"entity.lifecycle.stale"` | miss | **correct**: the top node is `docs/testing/source-removal215/README.md` |
| LX03 `retry backoff -test` | FABRICATED: 10 of 20 nodes are `_test` files | **correct**: 0 of 20 nodes are `_test` files |

LX03 shows real exclusion, not demotion: the pin returned ten test files, and alpha.5 returns
none.

### R1 rank stability

The procedure is the baseline's: score a fresh seed, restart `semsource` alone on the same
NATS store with `sources` reversed, score again, then `rank-diff.sh`. The result is **36/36
rank-identical, RANK-STABLE**, against 10/36 at the pin. The second-leg score is 32/36, the
same as the first.

### setup03a BM25 lane

`SETUP03A_PROFILE=bm25 SETUP03A_IDENTITY=governed`, `TestKnownAnswerCorpus`, with the binary
built from `e5d90d3`:

- Binary SHA-256 `6de2470e5f3348f7…`; corpus SHA-256 `9724050f1ebdc062…`; slices `[0, 1]`.
- **65 PASS, 0 FAIL**. This includes `cold_`/`warm_duplicate_name_anchors` and
  `cold_`/`warm_code_scope_before_limit` / `doc_scope_before_limit`, which together cover the
  epic's duplicate-name anchor and scope-before-limit items on mixed code and docs.
- The full report is in [`alpha5-tier0/setup03a-bm25-report.json`](alpha5-tier0/setup03a-bm25-report.json).
- The first attempt failed in the harness's broker setup (`docker port` before Docker had
  assigned the port, so no check ran). The rerun passed and is the one recorded here.

## Not covered

- **Tier 1** waits on 04C (#234); the baseline's tier 1 numbers are its floor.
- **R1 on OSH** was not run, as in the baseline.
- **semengine#620's `NameWeight` remeasure** was not attempted.
- **Arm A** is unchanged by construction: same corpus, same grader.
