# Tasks: SemSource-owned NATS test fixture

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md`
  Hold: OPEN-Q proposal awaits owner approval

## 2. Fixture and migration (PR #252, done ahead of approval)

- [x] 2.1 `internal/natstest`: `New`, `WithJetStream`, `WithStreams`, bounded setup, guarded replacement, ordered
  cleanup, unit tests through the `broker` seam, integration tests against a real broker
- [x] 2.2 Migrate 48 `natsclient.NewTestClient` sites in 38 files; add `./internal/natstest/` to CI's
  integration list

## 3. Attribution

- [x] 3.1 Cite the 12 `internal/natstest` tests to the new requirement
- [x] 3.2 Cite the 46 migrated integration tests to the requirements they prove; flag any with no fitting
  requirement on #228 (39 cited; 7 flagged)
- [ ] 3.2a Resolve the 7 flagged tests (no fitting requirement exists). The owner decides: new requirements in
  this change, or a separate change.
  Hold: OPEN-Q 7 migrated tests have no fitting requirement; owner to choose where their requirements live
- [ ] 3.3 `ateam test-spec --base cb74efd` exits 0

## 4. Close

- [ ] 4.1 Archive this change (seed the requirement into `openspec/specs/integration-fixture-reproducibility/`)
  and repoint any citation to the archived path before the PR is marked ready
