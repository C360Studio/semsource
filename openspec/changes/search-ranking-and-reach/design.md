# Design: Search Ranking and Reach

## Context

The graded interrogation put NL answer quality at 13/19; six misses share three
retrieval-layer causes: `_test.go` symbols drown production code (the #38
exported boost lifts `TestXxx` equally), archived OpenSpec planning docs outrank
canonical docs, and the config domain (go.mod deps incl. the semstreams pin) is
unreachable through every MCP tool (no `dc.terms.title` → invisible to
NAME_INDEX; NL scopes exclude the domain).

## Goals / Non-Goals

- Goals: production-over-test ranking, canonical-doc authority, config/git
  name-index reach, config answerable through doc_context. Re-run of the failed
  graded questions as acceptance.
- Non-goals: new ranking infrastructure (governed salience only — the
  don't-rebuild rule), Go call-graph recall (separate change), git domain in NL
  scopes (commits become byName-reachable via titles; NL inclusion deferred).

## Decisions

- **D1 — test demotion mirrors the exported boost**: presence-only marker
  `code.artifact.test` stamped on entities whose path is test code, registered
  `WithWeight(-2.0)` (the superseded_by precedent). Exported test helpers land
  near neutral (+2.0 − 2.0), production symbols outrank their tests, and tests
  stay fully indexed and structurally queryable. Alternative (exclude tests from
  the corpus) rejected: agents legitimately ask about tests by name.
- **D2 — path-based test detection, per-language**: Go `*_test.go`; TS/JS/Svelte
  `.test.`/`.spec.` filename infixes and `__tests__/` dirs; Python
  `test_*.py`/`*_test.py`; Java `src/test/` trees and `*Test.java`. One helper
  in `source/ast`, unit-pinned; conservative (miss = no demotion, never a wrong
  boost).
- **D3 — docs source default-excludes archived planning artifacts**:
  the walker skips `openspec/changes/archive` (relative to each root) and
  `node_modules`. Active proposals/specs and docs/adr stay indexed — the audit's
  polluting citations were archive entries. No new config surface until someone
  needs to opt back in.
- **D4 — reach via titles + docs-lens scope**: cfgfile entities gain
  `dc.terms.title` (dependency name, module path, image name, package name);
  git entities gain it too (commit subject, author name, branch name). The
  title predicate already carries the label alias, so NAME_INDEX visibility is
  automatic. The docs lens scope gains the `config` domain — dependency
  questions answer through doc_context. Code-lens scope unchanged (config
  entities in code_search would be noise).

## Risks / Trade-offs

- [Demotion weight tuning] −2.0 chosen to mirror existing precedent; the graded
  re-run is the empirical check → adjust only with evidence.
- [Title stamping grows the name index] config/git corpora are small relative to
  code; acceptable.
- [Existing graphs] new markers/titles appear on reindex; no ID changes.

## Migration Plan

Ship; periodic reindex (or restart) stamps new triples. Rollback = revert.

## Open Questions

- none blocking.
