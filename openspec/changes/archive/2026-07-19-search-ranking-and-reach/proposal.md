# Proposal: Search Ranking and Reach

**Priority: P1** — the largest single block of the 68%→90% graded-answer gap (audit 2026-07-19:
all 3 code_search misses, both doc_context misses, and the config-domain coverage gap)

## Why

The graded interrogation put answer quality at 13/19 against a ≥90% bar, and six of the misses
share three retrieval-layer root causes — none of which involve fabrication (zero hallucinations
across 38 graded calls; the engine is honest, just noisy and partially blind):

1. **Test noise drowns ranking**: `_test.go` files are indexed with no excludes and no demotion on
   the live component path; the exported-symbol boost (#38) boosts `TestXxx` equally. A paraphrase
   query for the ID sanitizer returned 12 test functions in the top 20 and never surfaced
   `SanitizeInstance` itself. semstreams#441 signed salience (available since beta.130) was adopted
   for supersession demotion but the test/generated demote-complement never shipped.
2. **Archived planning docs outrank canonical docs**: the docs source indexes
   `openspec/changes/archive/**` and proposals as peers of README/ROADMAP. "Which languages does
   the ast source support?" retrieved five archived-change docs and zero canonical answers
   (deterministically). The shipped corpus also self-contradicts (beta.144 vs beta.153 across
   ROADMAP/CLAUDE.md).
3. **The config domain is unreachable through every MCP tool**: 125 live entities (go.mod module +
   91 Go deps incl. the semstreams pin, npm deps, docker base images) carry no `dc.terms.title`
   (invisible to NAME_INDEX/byName) and sit outside every NL lens scope. Same latent gap: git
   commits, media. Verified live — all probes failed honestly.

## What Changes

- Test/generated demotion: test-file symbols (and recognized generated code) carry a demotion
  marker predicate registered with negative signed salience, so production symbols outrank their
  tests for NL queries; optionally default-exclude tests from NL retrieval while keeping them
  structurally queryable.
- Doc authority: the docs source default-excludes archived planning artifacts
  (`openspec/changes/archive/**`, or all of `openspec/**`) and/or canonical docs (README, docs/**)
  carry an authority boost; doc_context answers cite canonical docs for product questions.
- Reach: config-domain (and git-domain) entities become reachable — stamp `dc.terms.title` on
  config/git entities and include their domains in an appropriate lens scope (or a dedicated
  dependency-lookup surface, decided in design).
- Acceptance: the audit's graded questions Q9–Q13 (code_search), Q16–Q17 (doc_context), and Q19
  (config reach) re-run against a live stack and grade correct.

## Capabilities

### New Capabilities
- `retrieval-ranking`: NL retrieval ranks production code above test/generated code and canonical
  docs above archived planning docs, using governed salience — not result-set filtering hacks.

### Modified Capabilities
- `fusion-gateway`: "Domain-scoped NL retrieval per lens" — scope set gains the reach decision
  (config/git domains in a lens, or an explicit new surface).
- `source-vocabulary-contract`: config/git entity vocabularies gain the title predicate (and any
  demotion marker) as registered, canonical vocabulary — no ad-hoc predicates.

## Impact

- `source/ast` (test/generated detection + marker), `source/vocabulary` + `handler/cfgfile` +
  `handler/git` (title stamping), `handler/doc` / `processor/doc-source` (default excludes),
  `processor/code-context` (scope), salience registrations.
- Consumers: every MCP/GraphQL consumer's search quality; semdev A/B harness results.
- Boundary check: signed salience and NAME_INDEX are semstreams substrate (already shipped);
  SemSource only registers weights/predicates and sets scopes. If per-lens authority weighting
  needs framework support, file the ask — do not build a bespoke ranker (see
  feedback_understand_framework_before_rolling_own).
