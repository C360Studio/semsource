# Proposal: scorecard-v4

## Why

The scorecard is saturated: both product arms have scored 22/22 since 2026-08-09, and the beta.160
migration proof could only demonstrate non-regression. A saturated instrument cannot answer the
question now gating the next beta tag — is the fusion layer's recall actually better than raw
cosine (the C-parity finding), and what is our detailed search/query performance at scale? v4
restores the instrument's ability to discriminate, adds the missing latency dimension, and adds a
second corpus at ~3x scale, so the tag ships with a current performance story instead of a ceiling.

## What Changes

- **Composition band (questions v3 → v4, a documented comparability break):** new questions whose
  required facts provably do NOT co-occur in any single corpus passage — answerable only through
  graph traversal (impact closures, relationship joins, version diffs), not by retrieving one good
  chunk. A new automated checker (`check-composition.py`, sibling of `check-discrimination.py`)
  gates that property against the corpus, so the band cannot silently rot into
  single-passage-answerable questions. This is the band where arm B must beat arm C on **recall**;
  the harness stays one-tool-call-per-question and deterministic — composition happens server-side
  in the structural tools, not via scripted multi-call orchestration.
- **Latency dimension:** per-question wall-clock, recorded per repeat (three samples), reported as
  median/p95 per band per arm. Bytes remain the cost figure; latency is the performance figure the
  harness has never measured.
- **OSH second corpus:** a pinned-SHA Open Sensor Hub corpus (Java/Maven — the early adopter's
  shape, never measured) with its own question set (`questions-osh.json` v1, own comparability
  domain), corpus build recipe, and a scale report (entities, seed wall-clock, per-band latency)
  alongside the dogfood corpus.
- **Arm-D-ready cost model:** results JSON gains optional server-side LLM-cost fields (null for
  arms A/B/C) so a future research-graph deep-search arm plugs in without a schema break.
- Baseline runs of all arms on both corpora, committed with a SUMMARY.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None — instrument-only (`skip_specs: true`). Product behavior, the MCP surface, and ranking are
untouched; this change only measures them. The research-graph adoption that may follow is a
separate change and will carry its own spec deltas.

## Non-goals

- Any retrieval tuning (#137's reranker lands after v4 can measure it).
- Research-graph adoption or any arm-D implementation (separate, investigation-first change).
- Staleness/error-amplification probes (#132/#133 remain separate).
- Scripted multi-tool-call orchestration in the harness (breaks the one-call determinism model;
  free-form composition is exactly what arm D will be for).

## Impact

- `scripts/scorecard/`: `questions.json` v4, new `check-composition.py`, latency fields in all
  three arm scripts + `compare.sh`, `questions-osh.json` + OSH corpus recipe, README updates.
- No Go code, no product configs. Consumers: engineering decision-making, the tag decision, and
  issues #137/#130.
