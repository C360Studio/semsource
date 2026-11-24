# Refuse git sources that would share an entity-ID system — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` (2026-11-24, sha256:e653387ba8e0); line entered by claude at the owner's instruction. Ruling: #354 comment (refuse at config time).

## 2. Git systems

- [x] 2.1 Export `handler/git.SystemSlug(repoURL, repoPath, branchSlug)`; the handler's `systemSlug` calls it.
- [x] 2.2 `idSpace` row for git-source; `systemRoots` reads `repo_path`/`repo_url`/`branch_slug` and roots on the path, else the URL.
- [x] 2.3 `CheckSourceCollisions` plans `git` entries with remote branch resolution skipped.
- [x] 2.4 Git refusals carry the distinct-URL-or-name remedy; other kinds unchanged.

## 3. Tests

- [x] 3.1 `TestCheckSourceCollisions`: `/a/core` and `/b/core` refused naming both and system `core`; one path listed twice admitted; same basename on different branch slugs admitted; a repo whose git leaf collides with a git source refused. The "git sources are outside the rule" row is replaced.
- [x] 3.2 `TestCheckSystemCollisions` and the runtime-add table: a git row each; `TestSystemSlug_MatchesMintedSystem` (handler/git, integration) pins `SystemSlug` to the system ingest mints for path, URL and branch-scoped inputs. Mutation: dropping the git `idSpace` row fails the four refusal rows.

## 4. Gates

- [x] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./internal/sourcespawn/... ./handler/git/...`, `task spec:check`.
- [ ] 4.2 Review via `ateam review request`; archive before ready.
