# Forward a repo source's index_interval to its ast child — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` (2026-11-26, sha256:df98622bde17); line entered by claude at the owner's instruction. Note: the fix and test were written before this proposal (#409, c4aca97), when no spec covered `index_interval`; the owner chose this change so the test has a requirement to cite.

## 2. Fix

- [x] 2.1 Repro: `TestRepoExpansionForwardsIndexInterval` fails on main for both expansion paths (`ast child index_interval = "", repo set "30m"`).
- [x] 2.2 `config/expand.go` copies `IndexInterval` onto the ast child in `ExpandRepoSources` and `expandBranchSources`.

## 3. Gates

- [x] 3.1 `task lint`, `go test ./config/ ./internal/sourcespawn/`, `task spec:check`.
- [ ] 3.2 Review via `ateam review request`; archive before ready.
