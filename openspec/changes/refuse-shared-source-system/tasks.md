# Refuse sources that would share an entity-ID system — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-24, sha256:fdd5abe32728); line entered by claude at the owner's instruction. D4 OPEN-Q answered with option (b), recorded here so the approved digest stands. Ruling: #349 comment (refuse at config time, repro in the fix's test).

## 2. Repro (before code)

- [x] 2.1 Integration repro `internal/governance/shared_system_integration_test.go`, committed failing by design. Root a seeds, then root b seeds the same project, and the graph should hold both roots' files. It holds one: `main.go file entities [[…svc.golang.function.main-go-Beta]]` and `document titles [Beta]`, so root a's file and README are replaced, not merged (seeds skip reconcile, and per-predicate replace leaves the later root's set).

## 3. Collision check

- [ ] 3.1 `sourcespawn.CheckSystemCollisions(components)` with the D1 key and the D2 pin exception; unit table from design "Testing".
- [ ] 3.2 `Config.Validate` builds specs for the file's sources and calls the check (`semsource validate`, `semsource run`); repo sources needing a checkout are left to boot.
- [ ] 3.3 Boot calls the check over the planned set (file, desired manifest, discovered pins) before spawning; a collision stops boot naming the instances.
- [ ] 3.4 `AddWithChecker` calls the check over existing plus pending specs (same-instance re-adds excluded); refuses with `CodeValidationFailed`; nothing persisted.
- [ ] 3.5 ast-source and doc-source component `Validate` refuse roots sharing a system.
- [ ] 3.6 D4 recovery route, owner pick (b): one-shot `semsource remove --manifest <instance>` removes an entry from the desired manifest without a running service.

## 4. Remove the workaround

- [ ] 4.1 Delete `markSharedSystems`, `pathWatcher.sharedSystem` and the `component.go:938` shortcut; every root reconciles and publishes watch-event containers. #347 tests stay green.
- [ ] 4.2 The repro from 2.1 now asserts refusal at each entry point.

## 5. Close

- [ ] 5.1 `task check`, `go test -race -tags=integration ./...`, `openspec validate --all --strict`.
- [ ] 5.2 Archive the change before the PR is ready, and repoint the repro's `// spec:` citations from the change path to `openspec/specs/entity-identity-safety/spec.md`.
