# Refuse git sources that would share an entity-ID system

Approved-by: Coby Leuschke (2026-11-24) sha256:e653387ba8e0

Track: standard (extends one Requirement in `entity-identity-safety` to git-source; no identity change).

## Why

git-source mints every commit, author and branch ID under one system: `workspace.URLToSlug(repo_url)` when a URL is
set, else the repo directory's basename (`repoSlug`), branch-scoped by `branch_slug` (`handler/git/handler.go`
`systemSlug`). Its component instance is keyed on the full URL or path (`internal/sourcespawn/components.go`
`gitComponentConfig`), so `a/core` and `b/core` spawn two instances that both write `…core.git.branch.main` and
`…core.git.author.<email>`. Nothing refuses them: the #349 check (`internal/sourcespawn/collisions.go` `idSpace`) leaves
git-source out, and `CheckSourceCollisions` skips git entries. Today the writes merge into one entity; with the #336
reconcile on, each HEAD change in one repo would delete the other's statements, so git-source cannot opt in (#354).

Ruling: refuse at config time; the error names the colliding sources and tells the operator to give each a distinct
URL or name. Entity identity does not change (owner, 2026-11-24, comment on #354). #350 (merged, #370) made the poll
path mint under the seed's slug, so the slug the guard computes is the one every ingest uses.

## What Changes

- **handler/git exports its system slug** (`SystemSlug(repoURL, repoPath, branchSlug)`), and the handler's own
  `systemSlug` calls it, so the guard and ingest cannot drift.
- **`idSpace` gains git-source** as its own space (`git` IDs share no domain with the others). Its one root is its
  `repo_path`, or its `repo_url` when no path is set; its system is the exported slug. The existing file, boot and
  runtime-add checks then refuse two git sources under one org and system, and the same-instance check covers git.
- **`CheckSourceCollisions` plans `git` entries** (and the git leaf of single-branch `repo` entries) with remote
  default-branch resolution skipped: the branch is not an identity input, and validate must not contact a remote.
- **The git refusal's remedy** says to give each git source a distinct repo URL, or a repo directory with a distinct
  name (or a distinct branch slug). Other kinds keep their remedy (#369 covers media wording).

## Out of scope

- Opting git-source into the #336 reconcile (follow-up once this lands, with its own completeness test).
- Putting the repo into identity (stays a later option per the ruling), #348, #369.
- URL-only clones never fetching on poll (separate gap, noted on #370).
