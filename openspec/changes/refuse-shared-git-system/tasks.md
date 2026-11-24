# Refuse git sources that would share an entity-ID system — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval (ruling: #354 comment, 2026-11-24, refuse at config time).

## 2. Git systems

- [ ] 2.1 Export `handler/git.SystemSlug(repoURL, repoPath, branchSlug)`; the handler's `systemSlug` calls it.
- [ ] 2.2 `idSpace` row for git-source; `systemRoots` reads `repo_path`/`repo_url`/`branch_slug` and roots on the path, else the URL.
- [ ] 2.3 `CheckSourceCollisions` plans `git` entries with remote branch resolution skipped.
- [ ] 2.4 Git refusals carry the distinct-URL-or-name remedy; other kinds unchanged.

## 3. Tests

- [ ] 3.1 `TestCheckSourceCollisions`: `/a/core` and `/b/core` refused naming both and system `core`; one path listed twice admitted; same basename on different branch slugs admitted; a repo whose git leaf collides with a git source refused. The "git sources are outside the rule" row is replaced.
- [ ] 3.2 `TestCheckSystemCollisions` and the runtime-add table: a git row each; a test pins the guard's slug to the handler's for URL, path and branch-scoped inputs.

## 4. Gates

- [ ] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./internal/sourcespawn/... ./handler/git/...`, `task spec:check`.
- [ ] 4.2 Review via `ateam review request`; archive before ready.
