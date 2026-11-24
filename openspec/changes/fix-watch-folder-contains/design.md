# Design: complete folder sets on the AST watch path

## Context

Pin: semengine `v0.1.0-alpha.7`. Main `8c34a4a`.

- `BuildHierarchy(results, authority, project)` (`source/ast/hierarchy.go:22`) builds the repo and every folder from
  a full pass's parse results, with complete `Contains` sets (child folders, then files). It also sets each file's
  `ContainedBy`. Seed (`component.go:484`) and reindex (`:707`) call it through `publishHierarchy`.
- `BuildFolderChain` (`hierarchy.go:87`) builds only the ancestors of one file, with partial `Contains`. The watch
  path calls it on every create and modify (`component.go:584`).
- A whole-file ID fold (`idguard.go` `dropDuplicateIDs`) sets the folded result's `FileEntity` to nil, so the
  hierarchy never lists it. `refuseFoldedFile` keeps a folded file's watch event from publishing.
- The watcher sends one `OpDelete` with the root-relative path for a removed or renamed file or directory
  (`source/ast/watcher.go:434-450`). The handler only triggers the lifecycle run (`component.go:597-601`).
- Every folder and repo publish is `SkipReconcile` today (`component.go:937-939, 963-965`, tagged `shortcut` for this
  fix).

## Decisions

### D1. One file set per watch path, owned by ast-source

`pathWatcher` gains `files map[string]string` (root-relative path → file entity ID) behind its own mutex. It is written
in three places:

- **Full pass (seed, reindex):** replaced with every post-fold result that has a `FileEntity`, before the hierarchy is
  published. The replace returns the paths added and removed since the previous set.
- **Watch create or modify:** after `refuseFoldedFile` and before the file publish, the path is set. The event changes
  the set only when the path was absent.
- **Watch delete:** the path and every key with the prefix `path + "/"` are removed.

Memory is one path and one ID per indexed file: about 3 MB at the 28k-file OSH corpus.

### D2. Folders come from the set, never from one file

`BuildHierarchy` is split. A new `BuildHierarchyFromFiles(files map[string]string, authority, project)` builds the repo
and folders from the set. `BuildHierarchy` collects the set from its results, calls it, and keeps setting each file's
`ContainedBy` as it does now. Seed and reindex output does not change. `BuildFolderChain` and `publishFolderChain` are
deleted.

When a watch event changes the set, ast-source rebuilds from the whole set and publishes the repo and each ancestor
folder of the event's path. An ancestor missing from the rebuild (no indexed file left under it) is published once with
its name, path and `belongs-to` edge and no `contains`. The rebuild is O(files) per changing event. Modifies of known
paths, the common case, cost nothing.

### D3. Reconcile follows the existing known-changed rule

| Publish | Reconcile? |
| --- | --- |
| Seed hierarchy | No (`SkipReconcile`): nothing is known to have changed in this process (the restart gap, semengine#730) |
| Reindex hierarchy | Only the repo and the folders that are ancestors of a path added or removed since the previous pass |
| Watch event that changed the set | Yes, for each published entity |
| Any publish on a root with `sharedSystem` (#349) | No |

The two `shortcut` tags for #347 go away. The `entity-publish-integrity` requirement that excludes folders and repos is
changed to match.

### D4. Watch and reindex race on the set

The watch goroutine and the reindex goroutine both write the set. The mutex keeps the map consistent, and each
writer's publish happens under it, so publishes for the set never interleave. A file created after the reindex walk has
passed its directory, but before the reindex replaces the set, is dropped from the set until the next pass or its next
event. The file itself is still published by its watch event. The same window exists today for file entities. It is
tagged `// shortcut:` with that limit.

## Risks

- **A directory rename** arrives as a delete of the old path and creates of each new file. Those creates add the new
  folders one event at a time, and each event republishes the ancestors. That is correct but costs one hierarchy
  rebuild per moved file.
- **The emptied folder stays as an entity** (non-goal). Its parent no longer lists it, but it keeps its `belongs-to`
  edge, so an inverse walk still finds it. Today's reindex behaves the same.
