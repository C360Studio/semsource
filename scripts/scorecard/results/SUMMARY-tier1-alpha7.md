# Tier 1 scorecard on alpha.7 with semembed 0.2.1 (#234, task 5.3)

Arm B (MCP, fusion tools included) on the restored Tier 1 profile. This is the acceptance run for
`qualify-semengine-tier1` task 5.3. It is compared with #230 Tier 1 (`SUMMARY-pin-8b99efe9.md`),
alpha.6 Tier 0 (`SUMMARY-alpha6-tier0.md`) and #337 Tier 0 (`SUMMARY-fix329-tier0.md`).

## Setup

- **SemSource:** built locally from `60a1565` by the base compose `build`.
- **Config:** `tier1-alpha7/tier1.json` is `fix329-tier0/tier0.json` plus `graph.semantic_search: true` and a
  `model_registry` that binds `embedding` to semembed `Snowflake/snowflake-arctic-embed-s` at
  `http://semembed:8081/v1`. No `query_prefix` is set.
- **Provider:** `ghcr.io/c360studio/semembed:0.2.1@sha256:22ea57c6…292e`. Its identity is recorded in
  `docs/testing/setup-03a/tier1-provider-identity.json`. One container (2 CPUs) served both corpora and was never
  recreated.
- **Corpora:** dogfood `b4c2cfd9026b20b5` and OSH `9b1e77bc0bf0291f`, hashes rechecked.
- **Questions:** dogfood `questions.json` v7; OSH `questions-osh.json` v3.
- **Stack:** `tier1-alpha7/compose.tier1.yml` over `compose.semembed-host.yml`, project `semsource234`, fresh NATS
  storage per corpus, driven by `tier1-alpha7/run-tier1.sh`.
- **Host:** M3 Pro, arm64, shared with another harness, so the latency figures are indicative only.

## Result

| | #230 T1 | alpha.6 T0 | fix329 T0 | **alpha.7 T1** |
| --- | --- | --- | --- | --- |
| dogfood | 32/36 · 563 KB | 32/36 · 559 KB | 32/36 · 569 KB | **33/36 · 590 KB** |
| dogfood misses | P03, L02, L05, LX03 (FABRICATED) | X02, P03, L02, L07 | as alpha.6 | **X02, P03, L02** |
| loose band | 5/7 | 5/7 | 5/7 | **6/7** |
| OSH | 11/13 · 187 KB | 12/13 · 175 KB | 13/13 · 195 KB | **13/13 · 214 KB** |

- No band regresses against any baseline. L07 is newly correct, the lexical band is 3/3, and there are no UNSTABLE
  or FABRICATED verdicts.
- Every `code_search` and `doc_context` answer carries `provenance: embedding`, and every OSH `graph_search` answer
  has `merge.honoured: true`.
- Bytes are up 3.7% on dogfood (mostly the lexical band: 70 KB vs 27 KB) and 9.4% on OSH, against fix329.
- Median latency: dogfood 77 ms (fix329 62 ms), OSH 81 ms (fix329 76 ms).
- The tools/list schema is 8,175 B, 17 B under the 8,192 B gate.

The full per-band tables are in `tier1-alpha7/compare-{dogfood,osh}.txt`.

## Seed, provider work and restart (semengine#722)

- **Dogfood:** the seed took 571 s and made 7,271 provider requests (1,426,105 tokens); the GRAPH stream ended at
  7,851 msgs and 49.4 MB. A restart with the same config settled in 121 s and repeated 10 requests (144 tokens). So
  the #722 re-embed is real, but small at this scale.
- **OSH:** the seed took 1,112 s and made 25,180 requests (2,724,705 tokens). The restart did not settle (see the
  next section).
- After the restart, both corpora briefly reported `embedding.state: degraded`, then returned to `ready` on their own.

## #178 OSH tail

- **Fresh seed:** clean at 28,505 msgs and 221,779,926 B, which is 82.6% of the 268,435,456 B limit (memory storage,
  `max_age` 1h, `discard:new`). No 10077 rejects.
- **Warm restart within `max_age`:** breaches the limit. SemSource re-publishes the corpus while the seed's messages
  are still retained, so GRAPH reached 34,612 msgs and 268,434,490 B. From 21:38:14Z the publisher logged 10077
  (`maximum bytes exceeded`) after exhausting its delivery budget, and dropped 114 entities after bounded backpressure.
  Status went to `phase: degraded` with 11,147 of 28,461 entities re-published. The run was aborted as wedged at
  21:46Z; nothing would have aged out before about 22:17Z.
- This is not specific to Tier 1: embeddings do not travel on GRAPH, and the restart re-publish is the same at
  Tier 0. It is the #160 warm-restart wedge. No Tier 0 run has restarted on OSH, so there is no paired figure.

## Not covered

Arms A and C, the reversed-sources (r1b) leg, and the OSH restart re-embed count.
