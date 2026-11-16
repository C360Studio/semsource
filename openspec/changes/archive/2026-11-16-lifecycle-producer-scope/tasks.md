# Lifecycle producer scope tasks

## 1. Approval

- [x] 1.1 Obtain owner approval of `proposal.md` and `design.md`.

## 2. Contract

- [x] 2.1 Register `source.lifecycle.owner` in `source/vocabulary/lifecycle.go`; confirm it is in `graph.OwnedPredicates()`.
- [x] 2.2 Add `graph.LifecycleOwner(instance, root)` with unit tests (deterministic, separates instances and roots, no path in output).
- [x] 2.3 Replace `LifecycleRunRequest.PathPredicate` with `Owner`; `livenessOracle` refuses an oracle request without one.
- [x] 2.4 `lifecycleScope` filters on system and owner fact; unit tests for both #286 cases and for unowned entities.

## 3. Producers

- [x] 3.1 ast-source stamps the owner fact per watch path and sends it in every lifecycle request.
- [x] 3.2 doc-source stamps per resolved root and sends it.
- [x] 3.3 objectstore-source stamps per bucket and sends it.

## 4. Verification

- [x] 4.1 Keep `TestIntegration_ASTSweepDoesNotMarkLiveDocsOfSharedProject`; add a two-docs-roots integration test with disjoint filenames; both fail without the filter.
- [x] 4.2 Update governance and `test/setup03a` lifecycle requests to send `owner`; spec citations above every new test.
- [x] 4.3 `task check`, `go test -race -tags=integration` on the touched packages, setup03a `TestKnownAnswerCorpus` (bm25) and `TestDesiredSourceLifecycleWithoutRecovery`.
- [x] 4.4 Archive this change before the PR is marked ready.
