# SemEngine alpha.6 tier 0, regraded against the alpha.5 run (#301)

The pin moves to `semengine v0.1.0-alpha.6` with embeddings off. This is the
[`SUMMARY-alpha5-tier0.md`](SUMMARY-alpha5-tier0.md) run repeated on the new pin: same corpora,
question sets, source configuration, grader and machine. **Every band and every miss is
identical to alpha.5.**

**Date:** 2026-11-17. **Machine:** M3 Pro (arm64/Darwin). **Sets:** dogfood `questions.json`
v7 as on main at `86626c0`, OSH `questions-osh.json` v3. Repeats = 3. Verdicts are `correct`
or `miss` only.

## What ran

| | |
|---|---|
| SemSource | branch commit `a35e4ac`, built locally as `semsource:301-a35e4ac` (image `sha256:31349880863a495cf2782509353b0e018681c5932f64d876df06ea2cf758c2a4`, arm64). Not a CI image, unlike the alpha.5 run |
| SemEngine | `github.com/c360studio/semengine v0.1.0-alpha.6`, `embedding_enabled` unset |
| Config | [`alpha6-tier0/tier0.json`](alpha6-tier0/tier0.json), byte-identical to the alpha.5 run's |
| Corpora | dogfood `b4c2cfd9026b20b5` (8,192 entities), OSH `9b1e77bc0bf0291f` (29,422 entities); hashes rechecked |
| Stack | `docker-compose.yml` plus [`alpha6-tier0/compose.sem301.yml`](alpha6-tier0/compose.sem301.yml); fresh NATS storage per corpus |

The dogfood corpus is the frozen `758a805` tree, whose `configs/tiers/README.md` names
alpha.5. D10 was therefore graded with main's `questions.json`; this PR moves D10's expected
answer to alpha.6 for corpora cut from later trees.

## Result

| | alpha.5 | alpha.6 |
|---|---|---|
| dogfood, arm B | 32/36 · 559 KB | **32/36 · 559 KB** |
| dogfood latency median / p95 | 59 / 161 ms | 58.5 / 169 ms |
| dogfood misses | X02, P03, L02, L07 | X02, P03, L02, L07 |
| OSH, arm B | 12/13 · 175 KB | **12/13 · 175 KB** |
| OSH latency median / p95 | 58 / 101 ms | 65 / 146 ms |
| OSH misses | C03 | C03 |
| R1 (restart, sources reversed) | 36/36 identical | **36/36 identical, RANK-STABLE** |
| setup03a BM25 lane | 67 observations pass | **67 observations pass** |

No band moves in either corpus. Latency is one run each on a shared laptop; the OSH p95
difference is recorded, not judged.

R1 evidence is in [`alpha6-tier0/r1-evidence/`](alpha6-tier0/r1-evidence/): the container was
recreated with `tier0-reversed.json`, and the new process logged all three initial ingests
(cfgfile 126, doc 1,225, code 6,989 entities, 0 parse failures) before leg 2 was scored.

setup03a: `SETUP03A_PROFILE=bm25 SETUP03A_IDENTITY=governed`, `TestKnownAnswerCorpus`, binary
SHA-256 `238f13e3641e9fd6b13b15ea764e568efcd5bbd5d588053ed1a494218440ab84`; report in
[`alpha6-tier0/setup03a-bm25-report.json`](alpha6-tier0/setup03a-bm25-report.json).

## `searchGraph` at tier 0 on alpha.6

A direct `graph.query.searchGraph` request against the OSH stack answered with `data` keys
`hints` and `text` only: no `merge`, no `semantic`, and matches carrying `id`, `label`,
`score` with BM25-scale scores (14.95, 14.55; 1,486 matched). The merged order and score
apply only with `embedding_enabled` true, as the release notes say, so `graph_search` needs no
change on this pin. The three OSH `graph_search` questions stay correct.

## Replay

As in `SUMMARY-alpha5-tier0.md` "Replay", with `sem301`/`SEM301_*` for `sem233`/`SEM233_*`,
`alpha6-tier0` for `alpha5-tier0`, the image built by `docker build -t semsource:301-a35e4ac .`
at `a35e4ac`, and `SCORECARD_QUESTIONS` set to main's `questions.json` for the dogfood legs.
