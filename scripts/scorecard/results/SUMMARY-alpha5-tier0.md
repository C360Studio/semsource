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
| SemSource image | `ghcr.io/c360studio/semsource@sha256:e65c9d5c59d2853c13a52e936e2d47000fc3496d87dca695eb1f613cf51d19c0` (tag `e5d90d3`, CI build of main; arm64 manifest `sha256:1123f2c468ce0f4cd99391c7fcc40cec6b78f99af9d4c5d07d54da95a9e90d08`) |
| SemEngine | `github.com/c360studio/semengine v0.1.0-alpha.5` |
| NATS | `nats@sha256:311da47e2f31c26e99c623cd78d486a1e8ab039c49b32652770bb91a4f70512e` (`nats:2.14-alpine`), the same image as the baseline; setup03a uses `nats@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66` (`nats:2.14.4-alpine`) |
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
NATS store with `sources` reversed, score again, then `rank-diff.sh`. It is scripted as
[`alpha5-tier0/r1.sh`](alpha5-tier0/r1.sh). The result is **36/36 rank-identical,
RANK-STABLE**, against 10/36 at the pin. Both legs score 32/36.

The restart is proved by retained evidence, not inferred from equal ranks
([`alpha5-tier0/r1-evidence/`](alpha5-tier0/r1-evidence/)). The script stops if any check
fails:

- **The container was recreated.** `before.txt` and `after.txt` show different container IDs,
  a new start time, and `tier0-reversed.json` in the new process's arguments.
- **The new process reseeded every source**, and leg 2 was scored only after all three
  finished. `reseed-log.jsonl` holds that process's own log lines:
  - cfgfile, 126 entities
  - doc, 1,225 entities
  - code, 6,989 entities, 0 parse failures

  They start in the reversed order (cfgfile, then doc, then code). The wait is bounded at 15
  minutes and reads those log lines, not the status text.
- **`rank-diff.txt`** is the comparison output.

This R1 pair was re-measured with that evidence after review. An earlier pair, retained only as
result files, waited on a `grep seeding` that the status payload's readiness note satisfies
even when the service is ready, so it proved nothing about a reseed. The committed dogfood
result files come from the evidenced run.

Compared with the earlier pair, which ran on a different fresh store, both legs score the same
32/36. Each fresh store mints its own deployment authority (`c360.semsource-<id>`), so handles
differ by that segment. With it removed, 35 of 36 rankings are identical, and L03 swaps two
items at rank 18. R1 compares legs within one store, where authority is retained, so its handles
compare directly.

### setup03a BM25 lane

`SETUP03A_PROFILE=bm25 SETUP03A_IDENTITY=governed`, `TestKnownAnswerCorpus`, with the binary
built from `e5d90d3`:

- Binary SHA-256 `6de2470e5f3348f706380f86b3ba2f42e6805628d9030922c8b77b59870d412f`.
- Corpus SHA-256 `9724050f1ebdc0623460557bd4bb5c73c9b2591b4032737b79f9e7db821b0258`.
- Slices `[0, 1]`.
- **65 PASS, 0 FAIL**. This includes `cold_`/`warm_duplicate_name_anchors` and
  `cold_`/`warm_code_scope_before_limit` / `doc_scope_before_limit`, which together cover the
  epic's duplicate-name anchor and scope-before-limit items on mixed code and docs.
- The full report is in [`alpha5-tier0/setup03a-bm25-report.json`](alpha5-tier0/setup03a-bm25-report.json).
- The first attempt failed in the harness's broker setup (`docker port` before Docker had
  assigned the port, so no check ran). The rerun passed and is the one recorded here.

## Replay

Two checkouts are involved:

- `ART`: a checkout containing this PR, which holds the recipe artifacts.
- `SRC`: the measured commit `e5d90d3`, which builds the setup03a binary and runs its harness.

The PR changes only results and the ledger. The scorecard harness, `docker-compose.yml`,
`cmd/` and `test/` are byte-identical between the two: `git diff --quiet e5d90d3 HEAD --
docker-compose.yml scripts/scorecard/run.sh scripts/scorecard/rank-diff.sh scripts/scorecard/grade.sh
scripts/scorecard/questions.json scripts/scorecard/questions-osh.json test/ cmd/` exits 0. The
measured SemSource is the image pinned by digest in `compose.sem233.yml`.

```sh
ART=$(git rev-parse --show-toplevel)          # this PR's checkout
W=$(mktemp -d)
git -C "$ART" worktree add --detach "$W/src" e5d90d3
SRC="$W/src"
O="$ART/scripts/scorecard/results/alpha5-tier0"

# 1. Corpora, as in SUMMARY-pin-8b99efe9.md "Reproduce". Check each against its tree hash.
mkdir -p "$W/dogfood" "$W/osh"
git -C "$ART" archive 758a805 | tar -x -C "$W/dogfood" && rm -rf "$W/dogfood/scripts/scorecard"
curl -L https://codeload.github.com/opensensorhub/osh-core/tar.gz/235c0eabf24b6d6137b499b4402943d2794b70e6 \
  | tar -xz -C "$W/osh" --strip-components 1
for c in dogfood osh; do (cd "$W/$c" && find . -type f -print0 | LC_ALL=C sort -z \
  | xargs -0 shasum -a 256 | shasum -a 256 | cut -c1-16); done   # b4c2cfd9026b20b5, 9b1e77bc0bf0291f

# 2. Configs: tier0.json as committed, plus the reversed-source copy for R1.
mkdir -p "$W/configs" && cp "$O/tier0.json" "$W/configs/"
jq '.sources |= reverse' "$O/tier0.json" > "$W/configs/tier0-reversed.json"
export SEM233_CONFIGS="$W/configs"
cd "$ART"
dc() { docker compose -p sem233 -f docker-compose.yml -f "$O/compose.sem233.yml" "$@"; }

# 3. Dogfood: fresh storage, leg 1, the evidenced restart, leg 2, then rank-diff.
SEM233_CORPUS="$W/dogfood" "$O/r1.sh"

# 4. OSH on fresh storage.
export SEM233_CORPUS="$W/osh"
dc down -v
SEMSOURCE_CONFIG=tier0.json dc up -d nats semsource
SEMSOURCE_HTTP_PORT=28080 SCORECARD_READY_TIMEOUT=5400 \
  SCORECARD_QUESTIONS=scripts/scorecard/questions-osh.json scripts/scorecard/run.sh alpha5-tier0-osh-B
dc down -v

# 5. setup03a BM25 lane: binary and harness both from the measured commit.
cd "$SRC" && go build -o "$W/semsource" ./cmd/semsource
SETUP03A_BINARY="$W/semsource" SETUP03A_OUT="$W/setup03a-bm25" SETUP03A_PROFILE=bm25 \
  SETUP03A_IDENTITY=governed go test -tags=qualification -count=1 -v -run TestKnownAnswerCorpus ./test/setup03a
```

The measured setup03a binary was built from a clean `e5d90d3` worktree on darwin/arm64 with Go 1.26,
SHA-256 `6de2470e5f3348f706380f86b3ba2f42e6805628d9030922c8b77b59870d412f`. A replay build is not
byte-identical: the same build from another worktree path hashed `099b8ae991dbd0fa…`, because Go
embeds build metadata. The report records the binary that actually ran.

`r1.sh` runs from `ART`'s root and writes its evidence and result files there, so a replay
overwrites the committed ones; compare them with `git diff`. `compose.sem233.yml` pins the
image by its index digest, so the tag can't drift.

## Not covered

- **Tier 1** waits on 04C (#234); the baseline's tier 1 numbers are its floor.
- **R1 on OSH** was not run, as in the baseline.
- **semengine#620's `NameWeight` remeasure** was not attempted.
- **Arm A** is unchanged by construction: same corpus, same grader.
