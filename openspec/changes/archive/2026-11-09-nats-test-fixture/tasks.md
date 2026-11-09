# Tasks: SemSource-owned NATS test fixture

## 1. Approval

- [x] 1.1 Owner approves `proposal.md` and `design.md`

## 2. Fixture and migration (PR #252, done ahead of approval)

- [x] 2.1 `internal/natstest`: `New`, `WithJetStream`, `WithStreams`, bounded setup, guarded replacement, ordered
  cleanup, unit tests through the `broker` seam, integration tests against a real broker
- [x] 2.2 Migrate 48 `natsclient.NewTestClient` sites in 38 files; add `./internal/natstest/` to CI's
  integration list

## 3. Attribution

- [x] 3.1 Cite the 12 `internal/natstest` tests to the new requirement
- [x] 3.2 Cite the 46 migrated integration tests to the requirements they prove; flag any with no fitting
  requirement on #228 (39 cited; 7 flagged)
- [x] 3.2a Add requirements for the 7 tests with no fitting requirement (fusion-gateway ×2, mcp-gateway-contract,
  semstreams-governance) and cite them
- [x] 3.3 `ateam test-spec --base cb74efd` exits 0

## 4. Close

- [x] 4.1 Archive this change (seed the 5 requirements into their `openspec/specs/` capabilities) and repoint the
  19 delta-path citations to `openspec/specs/` before the PR is marked ready
