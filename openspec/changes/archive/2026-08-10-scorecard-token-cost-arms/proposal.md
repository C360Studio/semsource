# Proposal: scorecard-token-cost-arms

## Why

The retrieval scorecard measures whether SemSource answers correctly, but not what an answer
costs to consume — while the product's central claim to agent consumers is a cost claim:
querying the governed graph is supposed to be cheaper than grep-and-read over source. Issue
[#130](https://github.com/C360Studio/semsource/issues/130) records the methodology (adapted
from a published 4-arm benchmark) and the arms-mapping decision: this scorecard owns the
offline A/B/C comparison; semdev stays the two-arm live harness. The offline-first pattern is
proven — the session-only cosine harness predicted the live A/B outcome for
default-vs-override ranking — but that harness died with its session; this change also makes
it durable in-repo.

## What Changes

- **Context-cost accounting (arm B, the existing MCP path):** `run.sh` records, per question,
  the full tool-result bytes ingested — the entire decoded result an agent would receive, not
  only node bodies — including error results and failed attempts, plus a deterministic token
  estimate (bytes/4, no tokenizer dependency). Reported per band and in aggregate alongside
  the existing verdicts; existing fields and grading are unchanged.
- **Arm A — deterministic grep-and-read baseline:** a new script answers each question from
  the corpus with a fixed, deterministic procedure (grep over the question's query terms,
  charge the bytes of what a reader must then open). The procedure must never consult a
  question's expected answers — fairness rules are settled in `design.md`. Graded by the same
  grader as arm B.
- **Arm C — semembed-only ranking:** a new script ranks corpus chunks by cosine similarity
  via direct semembed `/v1/embeddings` calls (`query_prefix` on the query side only), grades
  top-K chunk bodies with the same grader, and charges their bytes. This isolates what the
  graph/fusion layer adds over embeddings alone.
- **MCP schema overhead:** the harness captures the `tools/list` response size at session
  init and records it in the results JSON as a separate, session-level figure — never folded
  into per-question cost. Feeds the measurement side of
  [#126](https://github.com/C360Studio/semsource/issues/126).
- **Per-band × per-arm reporting:** the summary compares fact recall and context cost across
  arms for the same `questions.json` version.

The question set itself does not change — no version bump; all arms consume the existing
`args.query` text, so cross-arm comparisons stay inside one comparability domain.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. This change is instrument-only (`skip_specs: true`): it extends `scripts/scorecard/`
and touches no product behavior — the MCP surface, fusion, ranking, and ingestion are all
unchanged. The harness's own contracts (comparability rules, verdict semantics, arm
procedures) live in `scripts/scorecard/README.md`, where the instrument's documentation
already lives.

## Non-goals

- **Staleness probe** (change corpus, re-ask, measure lag): requires provisioning control the
  harness deliberately does not have — follow-up issue, separate change.
- **Error-amplification probe** (seed a doc error, count retrieval surfacings): follow-up.
- **Question-set changes**: no new questions, no version bump in this change.
- **LLM-judged grading or live-agent arms**: the harness's determinism principle stands — a
  drifting judge cannot support an A/B.
- **MCP tool-surface changes**: [#126](https://github.com/C360Studio/semsource/issues/126)
  decides those; this change only measures.
- **A third semdev condition**: semdev stays two-arm per the arms-mapping decision on #130.

## Impact

- `scripts/scorecard/run.sh` — extended (cost fields, schema-overhead capture); results JSON
  gains fields, existing fields unchanged.
- `scripts/scorecard/` — two new arm scripts (A: grep baseline, C: semembed-only) plus README
  updates covering arm procedures and cost-comparability rules.
- No Go code, no product configs, no CI-path changes beyond optionally running the harness.
- Consumers: engineering decision-making only — evidence for #126/#130 and future
  competitive claims. No sem* product consumes the scorecard at runtime; semdev's live
  harness is unaffected.
