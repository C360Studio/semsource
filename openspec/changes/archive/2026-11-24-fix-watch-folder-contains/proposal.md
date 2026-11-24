# Watch edits publish each ancestor folder's complete contains set

Approved-by: Coby Leuschke (2026-11-24) sha256:033d15cb4a03

Track: design (new in-memory state in ast-source, a changed requirement in `entity-publish-integrity`).

## Why

A file create or modify on the AST watch path republishes every ancestor folder with a partial `code.structure.contains`
set: each folder gets at most the next folder down the chain and no files (`source/ast/hierarchy.go` `BuildFolderChain`,
called from `processor/ast-source/component.go` `publishFolderChain`). The stream lane replaces the stored set whole, so
the folder loses its other files and subfolders until the next full reindex, or for good when `index_interval` is off.

The repro `TestIntegration_WatchEditKeepsFolderContains` (`internal/governance/folder_contains_integration_test.go`)
runs the real ast-source → graph-ingest → `ENTITY_STATES` path on main `8c34a4a` (semengine `v0.1.0-alpha.7`). After an
edit to `a/b/x.go`, folder `a` goes from `[a/b, a/c, a/one.go]` to `[a/b]`.

It also keeps folder and repo entities out of the #336 reconcile, so a folder whose file was deleted keeps the
dangling `contains` edge.

Ruling: owner chose option B with deletes in scope (this session, 2026-11-23; recorded on #347).

## What Changes

- **ast-source tracks its indexed files.** Each watch path keeps the set of root-relative paths it indexed, with each
  file's entity ID. Every full pass (seed, periodic reindex) replaces the set. A watch create or modify adds the path. A
  watch delete removes the path and every path under it (a deleted directory arrives as one event).
- **Folders are built from that set.** The watch path stops using `BuildFolderChain`. When a watch event changes the
  set, ast-source rebuilds the hierarchy from the whole set and publishes the repo entity and every ancestor folder of
  the path, each with its complete `contains` set. A modify of a path already in the set changes nothing and publishes
  no folder. An ancestor left with no indexed files is published once with no `contains`, so its stale edges clear.
- **Folders and the repo join the #336 reconcile**, under the existing rule: a publish whose content is known to have
  changed reconciles, and every other publish (the seed) is `SkipReconcile`. On reindex, the folders whose set changed
  since the previous pass reconcile. Roots sharing one system (#349) stay `SkipReconcile`.

Consumers (SemTeams, SemDragon) see complete folder → file edges through graph query, MCP and GraphQL. No read surface
changes and no entity ID changes.

## Non-goals

- An identity change or reseed. Folder and repo IDs stay as they are.
- Deleting folder entities. A folder whose directory still exists but holds no indexed files keeps its entity and its
  `belongs-to` edge. Entity staleness stays with the lifecycle pass.
- Roots sharing one project (#349) and the git and cfgfile reconcile gaps (#348, #354).
- Making the first pass after a restart reconcile (the bounded form's known gap, semengine#730).
