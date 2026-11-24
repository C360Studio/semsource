# Complete folder sets on the AST watch path — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-23, sha256:269d2a1bbb6a; line entered by claude at the owner's instruction). Ruling: #347 comment (option B, deletes in scope).

## 2. Repro (before code)

- [x] 2.1 Repro committed and failing by design: `TestIntegration_WatchEditKeepsFolderContains` reports folder `a` shrinking from `[a/b, a/c, a/one.go]` to `[a/b]` (PR #358, 1dd01da). Other handlers checked: only ast-source publishes a container piecemeal (#347 comment 7118249050).

## 3. Hierarchy from a file set

- [ ] 3.1 `BuildHierarchyFromFiles` in `source/ast/hierarchy.go`; `BuildHierarchy` delegates to it. Unit test: same output as before for a seed-shaped input.
- [ ] 3.2 Delete `BuildFolderChain` and its tests.

## 4. ast-source file set and watch publishes

- [ ] 4.1 `pathWatcher.files` with replace (returns added/removed), set and prefix-delete, behind a mutex. Unit tests, including directory delete.
- [ ] 4.2 Seed and reindex replace the set; reindex reconciles only the repo and the ancestors of the changed paths (D3).
- [ ] 4.3 Watch create/modify/delete update the set and, when it changed, publish the repo and the ancestors from a full rebuild, with emptied ancestors published without `contains` (D2). Reconcile on, except `sharedSystem`.
- [ ] 4.4 Remove the two `#347` shortcut tags; tag the D4 window.

## 5. Proof

- [ ] 5.1 The repro passes.
- [ ] 5.2 Integration: a watch create of `a/new.go` adds it to folder `a`; a watch delete of `a/c/y.go` removes `a/c` from `a` and clears `a/c`'s `contains`.
- [ ] 5.3 `task lint`, `go test -race ./...`, `go test -race -tags=integration ./processor/ast-source/ ./internal/governance/ ./source/ast/`.

## 6. Land

- [ ] 6.1 Review (`ateam review request`), plus `graph-event-reviewer` on the watch path.
- [ ] 6.2 Archive this change before the PR is marked ready.
