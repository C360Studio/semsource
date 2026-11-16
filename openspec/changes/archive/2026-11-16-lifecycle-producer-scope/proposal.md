# Scope a lifecycle sweep to the root that produced the entity

Approved-by: Coby Leuschke (2026-11-16) sha256:2c15ba2d8aae

Track: design (adds a spec Requirement, changes the exported `graph.LifecycleRunRequest` wire
contract, and adds a graph fact every sweeping source writes).

## Why

#286: a staleness sweep selects the entities it judges by entity-ID system alone. Every source
configured with the same `project` shares that system slug, so one root's sweep stats another
root's artifacts under the wrong directory, finds them "missing", and marks live entities stale
(`path_missing`). A stale marker carries negative salience, so live documents are demoted in
fusion ranking and nothing is logged as wrong.

PR #293 at `d1b50cf` filtered by path predicate (`code.artifact.path` vs `source.doc.file-path`).
That separates AST roots from docs roots only. Review on #293 reproduced the remaining case: two
docs roots under one project with disjoint filenames have distinct IDs and still cross-mark. The
same holds for one doc-source or ast-source instance watching several roots: each root's sweep
judges the others. Owner ruling on #286 (2026-11-16): scope by producer ownership.

## What Changes

- Each sweeping source (ast-source, doc-source, objectstore-source) writes an owner fact,
  `source.lifecycle.owner`, on every entity it publishes: a deterministic key for the root
  (or bucket) that produced it. It joins the reconcile-mode `source` predicate group, so every
  publish restamps it.
- `graph.LifecycleRunRequest` gains a required `owner` whenever it carries a liveness oracle
  (`root_path` or `absent`). The pass judges only in-scope entities whose owner fact equals it,
  and refuses an oracle request without one.
- The `path_predicate` field from #293 is removed; owner scoping subsumes it (an AST root and a
  docs root never share an owner key).
- Entities without an owner fact are never judged by an oracle sweep. They gain one on the next
  publish; the SemEngine cutover already requires fresh storage plus re-ingest (ADR-102/104).

## Non-goals

- Entity identity is unchanged. Two roots that produce the same entity ID still collide; that is
  an identity limit, recorded as a known limit, not solved here.
- No change to `source_removed` (still refused) or to sources that do not sweep (git, url,
  cfgfile, media).

## Impact

- Specs: `entity-staleness` gains one Requirement.
- Code: `graph/lifecycle.go`, `processor/supersession/lifecycle.go`, the three sweeping sources,
  `source/vocabulary` (new predicate), tests in `internal/governance` and `test/setup03a`.
- Wire: a caller that sends `root_path`/`absent` without `owner` is refused. All callers are in
  this repository.
- Lands on PR #293 (Closes #286).
