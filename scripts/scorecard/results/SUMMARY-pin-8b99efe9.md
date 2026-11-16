# Pre-cutover baseline at SemStreams 8b99efe9 (#230)

The "before" number the SemEngine cutover is graded against: the last pre-cutover
SemSource (`bbfb3c6`, SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`), scored
with the #269 grader (content-only matching, failed calls grade `error`). #233 part 2 reruns
the same corpora, question sets and configs on SemEngine and compares band by band.

**Date:** 2026-11-16. **Machine:** M3 Pro (arm64/Darwin). **Sets:** dogfood `questions.json`
v7 (adds the lexical band), OSH `questions-osh.json` v2. Repeats = 3. Both corpora pass
`check-composition.py` and `check-discrimination.py`. Across every arm on both corpora there
are zero `error` and zero `UNSTABLE` verdicts; the one FABRICATED is LX03, which is
expected red (see below).

## What ran

| | |
|---|---|
| SemSource image | `ghcr.io/c360studio/semsource:bbfb3c6` (CI build of the commit; arm64 manifest `sha256:76426286f054…`, local image `sha256:045ad8baa8f5…`) |
| NATS | `nats:2.14-alpine` (`sha256:311da47e2f31…`), the bbfb3c6 compose pin |
| Embedder (tier 1) | semembed `ghcr.io/c360studio/semembed@sha256:7972174f8e34…` (bbfb3c6 compose pin), `Snowflake/snowflake-arctic-embed-s`, 384 dims, query prefix `Represent this sentence for searching relevant passages: ` |
| Tier 0 config | [`pin-8b99efe9/tier0.json`](pin-8b99efe9/tier0.json): bbfb3c6's `mvp.json` sources (ast + docs + config) with `embedder_type: bm25` |
| Tier 1 config | [`pin-8b99efe9/tier1.json`](pin-8b99efe9/tier1.json): bbfb3c6's `mvp.json` as shipped (`embedder_type: http` to semembed) |
| Dogfood corpus | `git archive 758a805` (main after #294) minus `scripts/scorecard/`; 1,239 files, tree hash `b4c2cfd9026b20b5`; 8,192 entities (ast 6,841 + docs 1,225 + config 126) |
| OSH corpus | osh-core `235c0eab` via `codeload.github.com/…/tar.gz/<sha>` (GitHub's `git archive`; the shallow fetch in `corpus-osh.sh` kept dropping on this network); 2,325 files, tree hash `9b1e77bc0bf0291f`; 29,422 entities |

The tree hash is `find . -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256`,
first 16 hex. The compose overrides are in [`pin-8b99efe9/`](pin-8b99efe9/): the bbfb3c6
`docker-compose.yml` plus `compose.base230.yml` (image, ports 28080/25222/29222, mounts) and
`compose.tier0.yml` (no semembed dependency) or `compose.tier1.yml` (semembed on host 28081 for
arm C). Each tier ran on fresh NATS storage.

Arm A's result files are not committed. Since #269 they keep every whole file each question
read (2 MB for dogfood, 4.4 MB for OSH), and arm A is a pure function of the corpus: rerun
`arm-a-grep.sh` on a corpus with the hash above to get them back byte for byte.

## Dogfood (v7, 36 questions)

| band | A (grep) | B tier 0 | B tier 1 | C tier 1 (cosine) |
|---|---|---|---|---|
| code | 6/6 | 3/6 | 6/6 | 6/6 |
| composition | 0/4 | 3/4 | 3/4 | 0/4 |
| discrimination | 1/2 | 1/2 | 2/2 | 2/2 |
| doc-early | 3/3 | 2/3 | 3/3 | 3/3 |
| doc-late | 3/7 | 7/7 | 7/7 | 7/7 |
| impact | 2/2 | 2/2 | 2/2 | 2/2 |
| lexical | 0/3 | 1/3 | 2/3 | 2/3 |
| loose | 5/7 | 2/7 | 5/7 | 7/7 |
| negative | 2/2 | 2/2 | 2/2 | 2/2 |
| **TOTAL** | **22/36 · 1,702 KB** | **23/36 · 349 KB** | **32/36 · 563 KB** | **31/36 · 632 KB** |
| latency median / p95 | 812 / 1,405 ms | 80 / 200 ms | 70 / 190 ms | 2,332 / 2,399 ms |

Misses, by run:

- **B tier 0** (13 misses):
  - D01 and L05, the `ast` doc fact.
  - C04–C06 and L01/L02/L04/L06/L07: `code_search` on the SemStreams feature-hashed BM25 ranks nothing relevant at rank 1.
  - X02: the top node doesn't carry the answer.
  - P03 (see below).
  - LX02 and LX03.
- **B tier 1** (4 misses): P03, L02, L05, and LX03.
- **C tier 1** (5 misses): P01–P04, because cosine over chunks cannot compose, plus LX03.

**P03 misses on every arm B run, and that is a measured product gap.** `code_impact
ScopedSystemSlug` lists 12 callers but omits `initializeWatchers`. The graph holds the edge:
`code_context initializeWatchers` lists `ScopedSystemSlug` as a callee. So the impact closure
at this pin truncates or filters callers. The misses come from retrieval, not from the question.

## OSH (v2, 13 questions)

| band | A (grep) | B tier 0 | B tier 1 | C tier 1 (cosine) |
|---|---|---|---|---|
| code | 2/3 | 2/3 | 3/3 | 3/3 |
| composition | 0/2 | 1/2 | 1/2 | 0/2 |
| config | 0/3 | 1/3 | 1/3 | 0/3 |
| doc | 1/2 | 2/2 | 2/2 | 2/2 |
| impact | 1/1 | 0/1 | 0/1 | 0/1 |
| negative | 2/2 | 2/2 | 2/2 | 2/2 |
| **TOTAL** | **6/13 · 4,095 KB** | **8/13 · 190 KB** | **9/13 · 187 KB** | **7/13 · 197 KB** |
| latency median / p95 | 1,306 / 2,789 ms | 73 / 180 ms | 89 / 258 ms | 8,537 / 12,014 ms |

**I01 and P02 miss on every arm B run because of the #269 grader, not retrieval.** Both
expect class names (`InMemoryDataStreamStore`, `SystemDataProvider`). The correct answer
carries them only as the caller's file:

- `checkKey` at `.../InMemoryDataStreamStore.java`
- `getProcedureDescriptions` at `.../SystemDataProvider.java`

#269 stopped grading paths. In Java the class is the file, so these two questions are
unanswerable under content-only grading as written. They need either a question repair or a
grader ruling on relation paths before #233 grades them. They count as misses here, and #233
must grade them the same way.

G02 and G03 remain red, as the config band always has been since v2: `graph_search`'s
compact shape carries no dependency versions (semstreams#958).

## R1 rank stability (dogfood)

Procedure (README, "The lexical band and R1 rank stability"):

1. Score a fresh seed.
2. Restart `semsource` alone on the same NATS store, with `sources` reversed.
3. Score again.
4. Run `rank-diff.sh`.

| tier | rank-identical | different | verdict |
|---|---|---|---|
| tier 0 (BM25) | **10/36** | 26 (all embedding-ranked questions; 14 diverge at rank 1) | RANK-UNSTABLE |
| tier 1 (semembed) | **36/36** | 0 | RANK-STABLE |

The 10 stable tier 0 questions are the structural ones (`code_context`, `code_impact`), which
don't depend on the embedder. The restart reset the tier 0 corpus statistics, which is the
instability semengine#10's R1 names. The second-leg score moved from 23/36 to 22/36 (loose
2/7 → 0/7, code 3/6 → 4/6). Tier 1 is deterministic across the same restart.

## Lexical band (LX01–LX03, expected red)

| | tier 0 | tier 1 | A |
|---|---|---|---|
| LX01 `"docker compose down -v"` | correct | correct | miss |
| LX02 `"entity.lifecycle.stale"` | miss | correct | miss |
| LX03 `retry backoff -test` | FABRICATED (a `func Test` body returned) | FABRICATED | FABRICATED |

At tier 0, two of the three are red as predicted. LX01 passes because "reseed" and "down -v"
share the top passages anyway. At tier 1, LX01 and LX02 pass, so the phrase questions
measure the tier 0 tokenizer, not dense retrieval. LX03 (exclusion) is red everywhere: no
surface excludes a term today. 04B is a tier 0 slice, so the tier 0 column is the one it must
move.

## The acceptance floor for milestones 3 and 4

#233 part 2 reruns these exact corpora, question sets and configs on SemEngine at its
admitted SHA, with each tier's equivalent composition:

- **No band regresses**, judged per tier and corpus against the B columns above.
  - Dogfood tier 0 is 23/36 and tier 1 is 32/36.
  - OSH tier 0 is 8/13 and tier 1 is 9/13.
- **The lexical band flips at tier 0**: at least LX02 and LX03 go from red to green.
- **R1 at tier 0 becomes RANK-STABLE**, against 10/36 here.
- Comparisons hold only on the same machine class for latency. Recall and bytes compare
  across machines.

## Reproduce

```sh
# corpora
git archive 758a805 | tar -x -C /tmp/base230/corpus-dogfood && rm -rf /tmp/base230/corpus-dogfood/scripts/scorecard
curl -L -o osh.tgz https://codeload.github.com/opensensorhub/osh-core/tar.gz/235c0eabf24b6d6137b499b4402943d2794b70e6
tar -xzf osh.tgz -C /tmp/base230/corpus-osh --strip-components 1
# stack (from a git archive of bbfb3c6; tier 1 swaps compose.tier0.yml for compose.tier1.yml)
BASE230_CORPUS=/tmp/base230/corpus-dogfood SEMSOURCE_CONFIG=tier0.json \
  docker compose -p base230 -f docker-compose.yml -f compose.base230.yml -f compose.tier0.yml up -d nats semsource
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh pin-8b99efe9-tier0-dogfood-B
SCORECARD_DENSE_EMBEDDER=Snowflake/snowflake-arctic-embed-s scripts/scorecard/arm-c-cosine.sh pin-8b99efe9-tier1-dogfood-C
scripts/scorecard/arm-a-grep.sh /tmp/base230/corpus-dogfood pin-8b99efe9-dogfood-A
```

The config mount is `/tmp/base230/configs`; copy `pin-8b99efe9/tier*.json` there, or edit
`compose.base230.yml`.
