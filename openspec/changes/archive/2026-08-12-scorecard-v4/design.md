# Design: scorecard-v4

## Context

See `proposal.md` — Why. Standing harness principles carry over unchanged: deterministic grading
(no LLM judge), one tool call per question, repeats for instability detection, first-answer
retention, corpus excludes `scripts/scorecard/`, comparability bound to a `questions.json`
version. The C-parity finding defines success: v4 is adequate only if some band separates arm B
from arm C on recall, or proves — against questions verified compositional — that they do not
separate, which is itself the decision-grade answer.

## Goals / Non-Goals

**Goals:** restore discrimination; measure time; measure scale; stay deterministic end-to-end.

**Non-goals (design-level):** simulating agent orchestration; cross-corpus score comparison (the
OSH set is its own comparability domain — its numbers never merge with the dogfood set's).

## Decisions

### D1 — The compositional property is verified against the corpus, never assumed

A composition question's defining property: **its `expect_all` facts must not co-occur within any
single ingested passage or code body.** If one chunk carries the whole answer, single-shot
retrieval (arm C, or doc_context) can pass and the question measures nothing new — the same rot
class `check-discrimination.py` guards against, so it gets the same treatment:
`check-composition.py` scans the corpus (chunk-sized windows over docs, body-sized over code) and
FAILS any composition question whose full fact set co-occurs in one window. Run before every
scored run, like the discrimination checker. The property is about the DATA; it keeps the harness
one-call: arm B answers through a structural tool whose server-side semantics are compositional
(impact closure, relation join, version diff), while arm C must try to luck into multi-fact
recall across its top-20 — and the top-node discrimination-style variant (`expect_top_all`)
stays unavailable to it by construction.

### D2 — Composition question shapes, from what the corpus actually supports

Authored against the dogfood corpus (with `why` fields naming the traversal), from these shapes:

1. **Impact closure**: "which components are affected if entity X changes" — `code_impact`;
   facts = specific dependent names that live in different files than X.
2. **Relation join**: "what calls X and belongs to Y" — `code_context` relations facet; facts
   span the caller's file and the membership declaration.
3. **Version composition**: "what changed between versions A and B affecting callers of C" —
   `code_changes` (`graph.query.versionDiff`); facts span the diff and the call graph.
4. **Cross-source join**: a fact from a config entity plus a fact from a code entity (e.g. a
   dependency declared in go.mod and the package that imports it) — no single passage carries
   both sides.

Band sizing is corpus-limited, like discrimination: we ship what the checker admits (target 4–6),
and the README states the band is small because the corpus supports a small band. The
question-set version bumps to 4; every existing question is retained verbatim so v4 runs still
grade the v3 bands identically (but scores are still never quoted across versions).

### D3 — Latency is sampled from the repeats we already make

Each repeat already issues a real first-class call; v4 timestamps every call and records
per-question `latency_ms` (first call, consistent with retention) plus `latency_samples` (all
repeats). Reporting: median and p95 per band per arm (arm A included — grep has latency too).
No new calls, no warm-up special-casing: the first call's latency is the one a real caller pays,
and cold-start effects are visible rather than hidden, consistent with the UNSTABLE philosophy.
Wall-clock is nondeterministic by nature; the README's comparability rules extend: latency
figures compare only across runs on the same machine class, and the results header records the
host arch (M3 Pro vs the Intel dev mini — the two-machines rule).

### D4 — OSH is a separate comparability domain with a pinned recipe

`corpus-osh.sh` builds the corpus: shallow clone of Open Sensor Hub core at a PINNED commit SHA
(recorded in the script; `master` branch per the adopter), `git archive` extraction, scorecard
exclusions applied. `questions-osh.json` (version 1) is authored against that snapshot —
Java/Maven shapes: POM dependency facts (cfgfile handler), Java symbol retrieval, impact on a
Java interface, plus at least one composition question if the checker admits it. The scale
report records total entities, seed wall-clock to full readiness, and per-band latency — the
first Java/Gradle-family numbers this project has ever had. OSH scores never merge with dogfood
scores; `compare.sh` already refuses cross-version joins, and the README states the
cross-corpus rule explicitly.

### D5 — Arm-D-ready cost fields, dormant until an arm D exists

Per-question results gain `llm_calls` (int) and `llm_cost_note` (string), null/absent for arms
A/B/C. The header records `arm_uses_llm: false`. When a research-graph arm D lands, it fills
these without a schema break, and `compare.sh` renders the column only when non-null. Grading for
a future arm D stays the deterministic matcher over whatever evidence the pipeline returns; run
variance lands in the existing UNSTABLE machinery. Nothing else about arm D is designed here.

## Risks / Trade-offs

- [Composition questions may be authorable but not checker-admissible on the small dogfood
  corpus] → the band ships at whatever size survives the checker; if that is zero, THAT is the
  finding (the corpus cannot discriminate composition and the OSH corpus becomes the primary
  vehicle) — stated, never papered over.
- [Latency noise on a shared dev machine] → three samples per question via existing repeats,
  median/p95 not means, host recorded, comparability rule scoped to same-machine runs.
- [OSH upstream churn] → pinned SHA in the recipe; the question set names the pin it was
  authored against.
- [v4 bump invalidates score history] → deliberate and documented, exactly like v3's bump; the
  v3 results remain in `results/` under their version.

## Open Questions

- Exact OSH pin (chosen at corpus-build time from current master — recorded, not pre-decided).
- Whether `code_changes` questions are viable on a static corpus snapshot (needs two ingested
  versions; may require the corpus recipe to ingest two refs — resolved during authoring, falls
  back to shapes 1/2/4 if not).
