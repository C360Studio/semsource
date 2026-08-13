# RC re-run for v1.0.0-beta.6 — both corpora, three arms

**Commit:** main `92b0d9f` (Wave 2 complete). **Date:** 2026-08-13. **Machine:** M3 Pro
(arm64/Darwin), shared dev box (load ~3 at start, one unrelated NATS demo container up).
**Sets:** dogfood `questions.json` v4, OSH `questions-osh.json` v1 (pin `235c0eab`) —
comparable ONLY to the 2026-08-12 v4/v1 runs. All three checkers green on both corpora at
this commit before scoring. Repeats = 3; **zero UNSTABLE, zero FABRICATED, zero errors in
any arm on either corpus.**

## Dogfood (semsource @ 92b0d9f, 6,623-revision graph)

| band | A (grep) | B (MCP) | C (cosine) |
|---|---|---|---|
| code | 6/6 · 190 KB | 6/6 · 67 KB | 6/6 · 102 KB |
| composition | 0/4 · 71 KB | **4/4 · 7.8 KB** | 0/4 · 71 KB |
| discrimination | 0/2 (IMPRECISE) | 2/2 | 2/2 |
| doc-early | 3/3 | 3/3 | 3/3 |
| doc-late | 6/7 | 7/7 | 7/7 |
| impact | 2/2 · 42 KB | 2/2 · 4.6 KB | 2/2 · 32 KB |
| negative | 2/2 | 2/2 | 2/2 |
| **TOTAL** | **19/26 · 938 KB** | **26/26 · 347 KB** | **22/26 · 426 KB** |
| latency med/p95 | 573 / 987 ms | **87 / 159 ms** | 1,848 / 1,884 ms |

Reproduces the 2026-08-12 separation exactly: B beats C only on the composition band
(4/4 vs 0/4) and beats A on doc-late + discrimination + composition. Arm A moved 18→19
vs the baseline run — arm A is a pure function of the corpus and the corpus changed
(Wave 2 merges); not a retrieval change on our side.

## OSH (osh-core @ 235c0eab, 32,157 entities, seed ~19.7 min)

| band | A (grep) | B (MCP) | C (cosine) |
|---|---|---|---|
| code | 3/3 · 210 KB | 3/3 · 109 KB | 3/3 · 103 KB |
| composition | 0/2 · 8.8 KB | **2/2 · 3.7 KB** | 0/2 · 12 KB |
| doc | 2/2 · 177 KB | 2/2 · 35 KB | 2/2 · 22 KB |
| impact | 1/1 · 21 KB | 1/1 · 2.1 KB | 1/1 · 7.5 KB |
| negative | 2/2 · 5.6 KB | 2/2 · 21 KB | 2/2 · 18 KB |
| **TOTAL** | **8/10 · 422 KB** | **10/10 · 171 KB** | **8/10 · 162 KB** |
| latency med/p95 | 450 / 2,627 ms | **148 / 255 ms** | 8,998 / 9,210 ms |

## What we can and cannot claim — read before quoting

**Holds, measured:**

- **Recall:** B is the only arm that answers composition questions, on both corpora.
  Everything else saturates across arms — a one-good-chunk question is answerable by
  grep, cosine, or the graph alike. The defensible recall claim is composition-band
  only.
- **Latency at scale:** B median went 87 ms → 148 ms from 6.6k-revision dogfood to
  32k-entity OSH (~1.7× at ~5× corpus). C went 1.8 s → 9.0 s (linear-ish in corpus
  vectors). A's p95 went 987 ms → 2,627 ms. B is the only arm whose latency is
  roughly flat in corpus size.
- **Zero fabrication, zero instability**, 3 repeats per question, both corpora.

**Does NOT hold — do not claim it:**

- **"B always uses fewer bytes" is FALSE on OSH.** Arm C's total context (162 KB) is
  *lower* than B's (171 KB) there; B's byte win is corpus- and band-dependent (on
  dogfood B is cheapest overall; on OSH it wins bytes only where structure matters —
  composition 3.7 KB vs 12 KB, impact 2.1 KB vs 7.5 KB). The honest cost claim: B
  buys *complete* recall at byte parity with cosine, ~2.5× cheaper than the grep
  floor, plus a 7,087 B session-fixed schema cost that the others don't pay.
- **B's negative band costs more bytes than grep's on OSH** (21 KB vs 5.6 KB): a
  correct "not found" from the MCP surface still ships near-miss evidence; grep's
  empty result is nearly free.
- Arm A is a *floor*, not an agent simulation — real agentic grep iterates and costs
  more. Comparisons against "grep" mean this fixed procedure.
- Latency figures are same-machine (M3 Pro) only, and this box was not idle.

**Instrument health:** the OSH set has no config band (authored against unlabeled
gradle deps). #157 (merged in Wave 2) added `DcTitle` to gradle deps, so a config band
is now *authorable* — but dependency property values (versions) remain unreachable
over MCP (#142's other half; graph_search renders id/type/label only). Adding the band
means a `questions-osh.json` version bump and a new comparability domain; do it
deliberately, post-tag.

**Slow-consumer data point (semstreams#950):** this seed produced **1,672**
`nats: slow consumer, messages dropped` ERROR lines (all inside seed minutes 6–18,
zero during query arms) vs ~124 on the 2026-08-12 baseline — a 13.5× run-to-run swing
on identical corpus/entity-count with identical scores and a converged graph. No
scored outcome moved, entity count matched exactly, readiness gates all passed. The
swing is unattributable by construction until the handler logs the subscription
(filed as semstreams#950); posted there as a data point.
