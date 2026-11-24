# Refuse sources that would share an entity-ID system — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval (`proposal.md` and `design.md`, including the D4 recovery-route question). Ruling: #349 comment (refuse at config time, repro in the fix's test).

## 2. Repro (before code)

- [ ] 2.1 Integration repro: two ast roots under one project (`main.go` each) and two doc roots under one project (`README.md` each) through graph-ingest; assert one ID carries statements from both roots. Committed failing by design.

## 3. Collision check

- [ ] 3.1 `sourcespawn.CheckSystemCollisions(components)` with the D1 key and the D2 pin exception; unit table from design "Testing".
- [ ] 3.2 `Config.Validate` builds specs for the file's sources and calls the check (`semsource validate`, `semsource run`); repo sources needing a checkout are left to boot.
- [ ] 3.3 Boot calls the check over the planned set (file, desired manifest, discovered pins) before spawning; a collision stops boot naming the instances.
- [ ] 3.4 `AddWithChecker` calls the check over existing plus pending specs (same-instance re-adds excluded); refuses with `CodeValidationFailed`; nothing persisted.
- [ ] 3.5 ast-source and doc-source component `Validate` refuse roots sharing a system.
- [ ] 3.6 The D4 recovery route the owner picks.

## 4. Remove the workaround

- [ ] 4.1 Delete `markSharedSystems`, `pathWatcher.sharedSystem` and the `component.go:938` shortcut; every root reconciles and publishes watch-event containers. #347 tests stay green.
- [ ] 4.2 The repro from 2.1 now asserts refusal at each entry point.

## 5. Close

- [ ] 5.1 `task check`, `go test -race -tags=integration ./...`, `openspec validate --all --strict`.
- [ ] 5.2 Archive the change before the PR is ready.
