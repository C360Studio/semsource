# Lifecycle producer scope tasks

## 1. Approval

- [ ] 1.1 Obtain owner approval of `proposal.md` and `design.md`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Contract

- [ ] 2.1 Register `source.lifecycle.owner` in `source/vocabulary/lifecycle.go`; confirm it is in `graph.OwnedPredicates()`.
- [ ] 2.2 Add `graph.LifecycleOwner(instance, root)` with unit tests (deterministic, separates instances and roots, no path in output).
- [ ] 2.3 Replace `LifecycleRunRequest.PathPredicate` with `Owner`; `livenessOracle` refuses an oracle request without one.
- [ ] 2.4 `lifecycleScope` filters on system and owner fact; unit tests for both #286 cases and for unowned entities.

## 3. Producers

- [ ] 3.1 ast-source stamps the owner fact per watch path and sends it in every lifecycle request.
- [ ] 3.2 doc-source stamps per resolved root and sends it.
- [ ] 3.3 objectstore-source stamps per bucket and sends it.

## 4. Verification

- [ ] 4.1 Keep `TestIntegration_ASTSweepDoesNotMarkLiveDocsOfSharedProject`; add a two-docs-roots integration test with disjoint filenames; both fail without the filter.
- [ ] 4.2 Update governance and `test/setup03a` lifecycle requests to send `owner`; spec citations above every new test.
- [ ] 4.3 `task check`, `go test -race -tags=integration` on the touched packages, setup03a `TestKnownAnswerCorpus` (bm25) and `TestDesiredSourceLifecycleWithoutRecovery`.
- [ ] 4.4 Archive this change before the PR is marked ready.
