# OSH v1 baseline — first Java/Gradle corpus, first scale point

**Date:** 2026-08-12 · **Questions:** questions-osh.json v1 (10) · **Own comparability
domain — never merge with dogfood numbers.**
**Corpus:** osh-core @ `235c0eabf24b6d6137b499b4402943d2794b70e6` via `corpus-osh.sh`
(21 MB, 1,932 Java files) · **Commit under test:** af0ea09 · **Host:** arm64/Darwin (M3 Pro)
**Gates:** check-composition + check-discrimination CLEAN on the pin; repeats=3, zero UNSTABLE.

## Scale report — the numbers that have never existed

| measure | dogfood | OSH | ratio |
| --- | --- | --- | --- |
| corpus size | 6.8 MB | 21 MB | 3.1x |
| entities | 6,240 | **32,157** | 5.2x |
| vectors | 6,240 | 31,504 | 5.1x |
| seed → full readiness | ~6 min | **1,311 s (~22 min)** | ~3.6x |
| arm B latency, median (ALL) | 132 ms | **108 ms** | **~1x — flat** |

**The product surface holds first-call latency flat at 5x entities** — that is the
detailed-search-performance evidence the tag was waiting on. The offline cosine arm,
by contrast, scales linearly (~2 s → ~8.8 s per question over the full vector set),
which is the cost any naive vector-scan integration would pay.

## Three arms on the Java corpus

| band | A recall/ctx | B recall/ctx | C recall/ctx |
| --- | --- | --- | --- |
| code | 3/3 · 210,204 B | 3/3 · 109,179 B | 3/3 · 103,285 B |
| composition | 0/2 · 8,797 B | **1/2 · 3,242 B** | 0/2 · 11,797 B |
| doc | 2/2 · 176,828 B | 2/2 · 40,293 B | 2/2 · 22,290 B |
| impact | 1/1 · 20,826 B | 1/1 · 2,141 B | 1/1 · 8,485 B |
| negative | 2/2 · 5,581 B | 2/2 · 20,648 B | 2/2 · 17,604 B |
| **TOTAL** | **8/10 · 422,236 B** | **9/10 · 175,503 B** | **8/10 · 163,461 B** |

B separates from C here too, and the separation is exactly the composition question
the Java call graph can answer (P02, class-qualified static calls). Zero fabrication
on a corpus the system had never seen.

## Findings — what the second corpus surfaced on its first run

1. **Java instance-receiver call edges are missing** (filed **#141**). P01:
   `code_impact("cloneAsTemplatePermission")` returns `impact: {nodes: 0}` against
   three real call sites — all of the form `localVar.method(...)`. P02's
   class-qualified static calls resolve 2/2 on the same stack. The empty closure is
   the *misleading* shape of wrong: an agent reads 0 dependents as "safe to change".
   P01 stays in the set as a discriminating question — it is measuring a real gap,
   which is the instrument working.
2. **Config facts are unreachable over MCP on a Gradle corpus** (filed **#142**).
   `gradleDependencyEntity` emits no `dc.terms.title`, so graph_search shows bare
   hashed IDs; no config band could be authored (recorded in `config_band_absent`).
3. **`nats: slow consumer, messages dropped`** — ~124 ERROR lines during seed, the
   first corpus large enough to overflow a core-NATS subscription in the fan-out
   path. Entity data rides JetStream (PubAck'd, durable) and the scored results show
   no ingest loss (all non-composition bands pass; P02's cross-module edges intact).
   Needs attribution to a specific subscription before filing upstream — parked here
   with the numbers (32k entities, 2-CPU semembed, ~22 min seed).
4. Doc retrieval on a 2-file doc corpus is unremarkable by design; the OSH set's
   value is the code/impact/composition side and the scale figures.
