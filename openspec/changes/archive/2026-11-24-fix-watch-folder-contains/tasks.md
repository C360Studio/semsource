# Complete folder sets on the AST watch path — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-23, sha256:269d2a1bbb6a). Re-approved after the graph-event review amended D2 and D4 (2026-11-24, sha256:033d15cb4a03). Both lines entered by claude at the owner's instruction. Ruling: #347 comment (option B, deletes in scope).

## 2. Repro (before code)

- [x] 2.1 Repro committed and failing by design: `TestIntegration_WatchEditKeepsFolderContains` reports folder `a` shrinking from `[a/b, a/c, a/one.go]` to `[a/b]` (PR #358, 1dd01da). Other handlers checked: only ast-source publishes a container piecemeal (#347 comment 7118249050).

## 3. Hierarchy from a file set

- [x] 3.1 `BuildHierarchyFromFiles` in `source/ast/hierarchy.go`; `BuildHierarchy` delegates to it. Unit test: same output as before for a seed-shaped input.
- [x] 3.2 Delete `BuildFolderChain` and its tests.

## 4. ast-source file set and watch publishes

- [x] 4.1 `pathWatcher.files` with replace (returns added/removed), set and prefix-delete, behind a mutex. Unit tests, including directory delete.
- [x] 4.2 Seed and reindex replace the set; reindex reconciles only the repo and the ancestors of the changed paths (D3).
- [x] 4.3 Watch create/modify/delete update the set and, when it changed, publish the repo and the ancestors from a full rebuild, with emptied ancestors published without `contains` (D2). Reconcile on, except `sharedSystem`.
- [x] 4.4 Remove the two `#347` shortcut tags; tag the D4 window.

## 5. Proof

- [x] 5.1 The repro passes.
- [x] 5.2 Integration: a watch create of `a/new.go` adds it to folder `a`; a watch delete of `a/c/y.go` removes `a/c` from `a` and clears `a/c`'s `contains`.
- [x] 5.3 `task lint`, `go test -race ./...`, `go test -race -tags=integration ./processor/ast-source/ ./internal/governance/ ./source/ast/`.

- [x] 5.4 graph-event-reviewer (approve with warnings, at 8e219d4). Fixed: W1 the emptied folder of a deleted directory is now cleared (no stat gate; containers carry no owner fact, so the lifecycle pass never judges them), with a directory-delete step in the delete test; W2 the D4 shortcut names both directions; W3 shared-system roots publish no container from watch events (spec carve-out); S1 a watch fold onto an indexed ID is not added; S2 an empty seed publishes no repo. S3 (filesMu held across a full container publish) is accepted per D4. D2/D4 amended, so re-approval is needed.
- [x] 5.5 Codex review round 1 (REQUEST CHANGES at 14beaa2). Fixed:
  - P1: container targets are keyed by entity ID, so a folder colliding with a live one (`a-b` vs `a/b`) is never emptied, and two live folders sharing an ID are `SkipReconcile` (the existing shared-writer rule). `TestIntegration_WatchDeleteKeepsCollidingFolder` fails on 14beaa2 and passes here, and `TestPlanContainers` covers the watch and reindex paths.
  - P2: the changed paths of a refused container publish are kept and retried by the next one, including an unchanged full pass (`TestPublishContainers_RetriesRefusedPublish`).
  - P3: the integration tests wait on the expected outcome, and the delete test proves the watcher is live with a probe before deleting once.

## 6. Land

- [ ] 6.1 Review (`ateam review request`), plus `graph-event-reviewer` on the watch path.
- [x] 6.2 Archive this change before the PR is marked ready.
