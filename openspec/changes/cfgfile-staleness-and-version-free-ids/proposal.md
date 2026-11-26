# Config-file staleness, and dependency and image IDs without versions

Approved-by: Coby Leuschke (2026-11-26) sha256:eed09562c886

Track: design (identity break riding the beta.12 reseed; extends the staleness pass to config files).

## Why

#393, ruled 2026-11-26. Two gaps leave config-file entities live after they stop being true.

1. **Each version bump makes a new entity.** Go, npm, Maven and Gradle dependency IDs hash the version
   (`handler/cfgfile/entities.go:102`, `:207`, `:393`, `:547`), and image IDs hash the full reference including the
   tag (`:262`). A bump mints a new entity and leaves the old one live with no inbound edge. After a few
   `go get -u` rounds, `graph_search` on a dependency name returns every past version, unmarked.
2. **A deleted manifest is never marked stale.** cfgfile-source never publishes a lifecycle trigger (doc-source does,
   `processor/doc-source/component.go:586-608`). Its entities carry no `entity.lifecycle.owner` fact, and the stat
   oracle's `pathOf` reads only `code.path` and `source.doc.file-path` (`processor/supersession/lifecycle.go:363`),
   not `source.config.file-path`.

## What Changes

- **Version-free IDs.** A dependency's instance hashes its coordinate only: Go module path, npm name, Maven
  `group:artifact` (plus type/classifier as today), Gradle `group:name`. An image's instance is
  `slug(name)-hash(name)`, where `name` is the reference without its tag or digest. The #348 declaring-file suffix
  stays. Versions and the full image reference stay as facts. One manifest declaring a coordinate at two versions
  (or an image at two tags) folds into one entity with multi-valued version or reference facts, as #375 did for
  scopes. A bump replaces the fact through the existing reconcile (#384).
- **Staleness for config files.**
  - Every cfgfile entity gets an `entity.lifecycle.owner` fact.
  - Dependency and image entities also carry their declaring manifest's `source.config.file-path`, so a deleted
    manifest takes them with it.
  - cfgfile-source triggers the lifecycle pass on seed and on a watch delete, as doc-source does.
  - `pathOf` reads `source.config.file-path`, and the stat oracle stats an absolute path as given.
  - Re-adding the manifest clears the mark, through the pass's existing clear path.
- **Documented, not fixed:** a dependency removed from a manifest that still exists stays live as retained history
  (option B is #421).
- **Tests first**, each failing on main:
  - a version bump leaves one dependency entity, with the new version;
  - deleting a watched manifest marks its entities stale, dependencies included;
  - re-creating the manifest clears the mark.

## Decisions you are making by approving

- **Every dependency and image ID changes once.** This rides the beta.12 reseed, which #408's notes must name. There
  is no aliasing.
- **Two versions of one coordinate in one manifest are one entity** with both versions as facts. Recommended over
  keeping the version in the ID just for that case.
- **Dependencies carry their declaring manifest's path,** so the existing stat oracle judges them. This adds one fact
  per dependency. There is no new liveness mechanism.
- **The stat oracle accepts absolute paths.** `source.config.file-path` is absolute today
  (`source/vocabulary/config.go:12`). Rewriting it to a root-relative path would be a second fact change.

## Out of scope

- Option B, marking stale what a manifest stops declaring: #421.
- Any other source's identity.
