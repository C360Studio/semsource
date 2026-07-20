# Retrieval ranks the workaround above the default

## Problem

A query for the **default** value of a setting returns the section describing how to
**override** it, and the section holding the default never surfaces.

Measured on a live stack during the `doc-passage-chunking` change's bounds A/B
(task 9.5, `scripts/scorecard/results/SUMMARY-9.5-bounds.md`):

- Query: *"what is the default host port for the NATS monitor in docker compose"*
- Top node: `README.md § Quick Start` — a correctly-bounded 711-byte passage whose
  only mention of that setting is `NATS_MONITOR_HOST_PORT=28222`, the
  **port-conflict workaround**.
- `README.md § Configuration`, which carries the actual default
  (`NATS_MONITOR_HOST_PORT=8222`), does not appear.

A second question fails the same way: *"what port does the seminstruct inference
container publish"* never surfaces the Tier 2 section; ranks 4 and 5 are both Tier 1
content for a different service.

This is not a chunking defect. The passages are cleanly bounded and the right passage
exists in the graph. It reproduced **identically at 1000/200, 2000/400 and 4000/800**,
so it is independent of passage size. Retrieval is deterministic — verified by
repeated identical calls — so it is not flakiness either.

Why it matters more than a miss: answering *"what is the default X"* with *"how to
override X"* is a **plausible-looking wrong answer**. The consumer contract this
product sells is that evidence can be trusted without re-deriving it, and a confident
wrong section is the most expensive way to break that. The audit's headline result
was zero fabrication across 19 questions; this is the adjacent failure that
fabrication checks do not catch, because nothing was invented — the wrong true thing
was returned.

## What is in scope

**1. The ranking defect (primary).** Make a query for a canonical value prefer the
section that defines it over a section that overrides, troubleshoots, or works around
it — or establish that SemSource cannot influence this and record it as a framework
ask instead.

The mechanism is **deliberately unresolved in this proposal**. What is already known,
and what `design.md` must settle:

- Ranking is the substrate's: `processor/code-context` builds
  `fusion.NewEngine(...).WithSignals(fusionvocab.New())`. Per the Product Boundary,
  SemSource does not reimplement the engine.
- The levers SemSource *does* own are what it emits: predicate salience
  (`vocabulary.WithWeight`), the stamped ontology class, passage titles/sections, and
  passage bodies.
- Doc-side salience today is **demotion-only** — `source/vocabulary/lifecycle.go`
  (−3.0) and `navigational.go` (−2.0). The latter is what fixed the body-less-parent
  defect in the previous change, so this lever is known to work.
- No predicate distinguishes canonical guidance from troubleshooting prose, and
  inferring that from text is the risky part. Design must decide whether a
  structural signal exists that is not prose-sniffing, and must say plainly if the
  honest answer is an upstream ask.

**2. A third scorecard verdict.** The discrimination band grades three states but
names only two. A top node carrying the confusable twin **but not the answer** is
scored a plain `miss`, though it is worse than absent — it argues for the wrong
answer. It needs its own verdict alongside `correct` and `IMPRECISE`, so a run can
distinguish "found nothing" from "found the wrong thing". This is the instrument that
would show item 1 improving.

**3. A spec correction.** `openspec/specs/runtime-configuration/spec.md`'s
config-validation requirement claims coverage of *"namespace/org, explicit source
identity overrides"* and that values *"are rejected, never silently rewritten"*. Only
`Namespace` is validated (`config.ValidateNamespace`). `SourceEntry.Project` /
`WatchPathConfig.Project` get a non-emptiness check, then flow into
`entityid.SystemSlug`, which normalizes the charset and truncates past 80 characters
with a content hash — silently rewritten. The requirement is narrowed to current
truth: `org` is rejected because it is the sovereignty boundary and is never
normalized; `project` is normalized by design.

## Non-goals

- **Reimplementing or forking the fusion ranking engine.** It is substrate
  (semstreams `pkg/fusion`). If the fix requires engine behavior SemSource cannot
  influence, the outcome is an entry in `docs/upstream/semstreams-asks.md` and a
  GitHub issue — never a PR to semstreams.
- **A bespoke retrieval index or re-ranker inside SemSource.** The fusion gateway is
  deterministic over `graph.query.*` by design (ADR-0004).
- **Prose classification / LLM-judged ranking.** Deciding "is this paragraph
  canonical or a workaround" by reading it is exactly the fragile approach this
  change should justify before adopting, not assume.
- **Re-tuning the passage bounds.** Settled in the previous change: a 4x ceiling
  range changed no graded outcome, and this defect reproduced across all of it.
- **Widening the discrimination band.** An automated sweep found only one usable
  confusable pair in this corpus beyond the two shipped. Growing the band needs a
  larger or different corpus and is separate work.
- **Enforcing `project` validation at config load** (item 3). Normalizing it looks
  intentional; rejecting a value `SystemSlug` would cleanly slugify costs usability
  for no safety gain. Item 3 corrects the spec, not the code.

## Consumers

`code_context` / `doc_context` over MCP and HTTP are the surfaces affected, so the
consumers are every agent-facing caller: **SemSpec** (whose hard e2e prompt is the
project's canary), **SemTeams** (curator workflows over NATS), and any agent driving
SemSource through the MCP gateway. The scorecard (`scripts/scorecard/`) is the
in-repo instrument and is versioned — item 2 bumps `questions.json` to version 3, so
version 2 results are not comparable across it.
