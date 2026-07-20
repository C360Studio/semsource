# Retrieval ranks the workaround above the default

> **Blocked on `repair-retrieval-scorecard`.** The instrument this change must be proven
> with cannot currently render a correct verdict (a matcher bug fails one discrimination
> question on every system, and results vary with question order). This change stays open
> and unimplemented until that lands. See Non-goals.

## Problem

A query for the **default** value of a setting ranks the section describing how to
**override** it above the section that defines it.

Measured on a live stack, 2026-07-20, on the corpus fixed by `git archive d554bcc`
(206 documents, 5410 entities, `scripts/scorecard/` excluded):

- Query: *"what is the default host port for the NATS monitor in docker compose"*
- **Top node:** `README.md § Quick Start` — a correctly-bounded 712-byte passage whose only
  mention of the setting is `NATS_MONITOR_HOST_PORT=28222`, the port-conflict **workaround**.
- `README.md § Configuration`, which carries the actual default
  (`NATS_MONITOR_HOST_PORT=8222`), is recalled at semantic **rank 7** and returned by
  `doc_context` at **node rank 3**.

> **Correction.** An earlier reading of this defect — recorded in this proposal's first
> revision and in `results/SUMMARY-9.5-bounds.md` — stated that `§ Configuration` "never
> appears". That is wrong: it appears, at node rank 3. The defect is a ranking miss of two
> positions, not an absence. The scorecard grades the **top node** alone (deliberately, so
> the band measures whether the single best piece of evidence answers on its own), which is
> why the failure presented as a total absence. The narrower claim is still a real failure,
> and still the expensive kind: an agent taking the first citation is told the answer is
> 28222.

**A second question is no longer evidence for this defect.** *"What port does the seminstruct
inference container publish"* was recorded as failing the same way. It does not: it retrieves
the correct Tier 2 section at rank 1 and returns it as node 1. It grades `miss` because of a
grader bug, and separately loses its top node when asked late in a run. Both belong to
`repair-retrieval-scorecard`. This change rests on X01 alone.

## Root cause — measured, not inferred

**Embedding dilution.** Offline re-embedding against the same `semembed` model reproduced the
live cosines exactly (0.7323 vs the index's 0.7322; 0.6569 vs 0.6569), so the harness is
trustworthy. Against the same query:

| passage text | bytes | cosine |
| --- | ---: | ---: |
| `§ Quick Start` body — the distractor | 712 | **0.7323** |
| `§ Configuration` body — the answer, as emitted today | 1363 | 0.6569 |
| the answer's `NATS_MONITOR_HOST_PORT=8222` line alone | 70 | **0.8133** |
| that line under its `### Configuration` heading | 89 | **0.8127** |
| the heading plus all three `NATS_*` lines | 236 | **0.7783** |
| the full body with a `README.md > … > Configuration` path prefix | 1407 | 0.6926 |
| the full body reframed to say "default values" | 934 | 0.6902 |

The same fact, isolated, gains **+0.156 cosine** and beats the distractor comfortably. It is
not hard to retrieve; it is drowned by the ten unrelated environment variables that share its
vector. **Cosmetic emission changes do not flip it** — neither a section-path prefix nor
"default" framing closes the gap. Only reducing what the passage is *about* does.

**Why the bounds A/B saw nothing.** The block is 1363 bytes — under *every* ceiling tested
(1000, 2000, 4000). The ceiling was never the binding constraint for it. The earlier
conclusion "independent of passage size" was true of the *ceiling* and false of the passage's
*content span*, which is what actually dilutes the vector. This resolves the paradox that
made the defect look intractable.

## Why predicate salience cannot fix it

`rankEntities` (semstreams `pkg/fusion/engine_lens.go:336`) scores:

```
s = 4.0×(N−i) + lexicalScore(query, label) + 1.5×ClassSpecificity + 3.0×entitySalience
```

with `N = resolveLimit = 40`.

- Recall order spans **160 points** at 4.0 per position; the rank-1 ↔ rank-7 gap is **24
  points**.
- SemSource registers only *negative* weights on the doc side, so `entitySalience` ranges
  **[−6, 0]** — a swing of **1.5 positions**. Maximally demoting the distractor still leaves
  it ~18 points ahead.
- Closing 24 points would need a weight ≥ 8.0 against existing weights of 2.0–3.0, directly
  against the substrate's stated intent that these are "deliberately small … break ties /
  nudge within a tier **rather than dominating the graph's own ranking**". Salience is a
  tiebreaker *by contract*, not a ranking lever.
- `lexicalScore` matches the **whole query string** against the entity label, so it is
  always **0** for a natural-language doc query.

⇒ For `doc_context` natural-language queries, ranking is **essentially pure cosine order**.
The only lever SemSource owns is the text it emits into the embedded body — and per upstream
ask #21, for offloaded (`StorageRef`) entities **only body bytes are embedded**, titles
excluded. The body is the whole lever.

## What is in scope

**1. The ranking defect.** Make a query for a canonical value prefer the section that defines
it. The mechanism is now known, so the design's job is the *shape* of the emission change,
not the diagnosis: split homogeneous structured blocks (environment-variable blocks, config
key tables) into **topically-grouped** sub-passages rather than holding them as one passage.
Grouping the three `NATS_*` keys together measures 236 bytes at 0.7783 — already past the
distractor — while per-key atoms (70 bytes, 0.8133) score higher but are thin as evidence.
The design must settle where that line sits and must not regress the saturated doc bands.

**2. A spec correction.** `openspec/specs/runtime-configuration/spec.md:43-45` claims
config-load validation covers *"namespace/org, explicit source identity overrides"* and that
values *"are rejected, never silently rewritten"*. Verified against code: only `Namespace` is
validated (`config.ValidateNamespace`). There is **no `Project` validation at all**, and
`entityid.SystemSlug` maps out-of-allowlist runes to `-` and truncates past 80 characters
with a content hash — silently rewritten. Narrow the requirement to current truth: `org` is
rejected because it is the sovereignty boundary and is never normalized; `project` is
normalized by design.

## Non-goals

- **Repairing the scorecard.** Moved to `repair-retrieval-scorecard`, which this change is
  blocked on. That includes the third verdict (top node carries the twin but not the answer)
  originally proposed here.
- **Reimplementing or forking the fusion ranking engine.** Substrate (semstreams
  `pkg/fusion`). The scoring analysis above is diagnosis, not a plan to change it. If a fix
  requires engine behaviour SemSource cannot influence, the outcome is an entry in
  `docs/upstream/semstreams-asks.md` and a GitHub issue — never a PR to semstreams.
- **Raising predicate salience weights to force the ordering.** Ruled out above on the
  substrate's own stated contract, not merely on taste.
- **A bespoke retrieval index or re-ranker inside SemSource.** The fusion gateway is
  deterministic over `graph.query.*` by design (ADR-0004).
- **Prose classification / LLM-judged ranking.** Deciding "is this paragraph canonical or a
  workaround" by reading it remains the fragile approach this change avoids — and the
  measurement shows it is unnecessary, because the signal is structural (block homogeneity),
  not rhetorical.
- **Re-tuning the passage bounds.** Settled in `doc-passage-chunking`. This change adjusts
  *what constitutes a passage boundary* inside a structured block, not the size ceiling.

## Consumers

`code_context` / `doc_context` over MCP and HTTP, so the consumers are every agent-facing
caller: **SemSpec** (whose hard e2e prompt is the project's canary), **SemTeams** (curator
workflows over NATS), and any agent driving SemSource through the MCP gateway. Entity count
rises with any finer split, which affects time-to-ready — it must be measured, not assumed.
