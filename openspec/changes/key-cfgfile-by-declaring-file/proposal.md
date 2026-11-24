# Key cfgfile entities by their declaring file

Approved-by: Coby Leuschke (2026-11-24) sha256:b1d1fdb976aa

Track: design (identity-breaking: changes every cfgfile entity ID; clean storage break plus reseed, rule 1).

## Why

cfgfile mints dependency, image and project IDs from the declared coordinate alone, under one system per source
(`handler/cfgfile/entities.go`). Two manifests that declare one coordinate write one entity with different facts:
Maven and Gradle share `config.dependency` (`:368`, `:517`) with flipping `dependency-kind` and per-file
`dependency-scope`/`-configuration`; `config.image` is `slugify(image)` (`:244`) carrying one Dockerfile's ports and
path; Gradle `project` is the directory base name (`:663`), so `a/core` and `b/core` collide. Go `indirect` and npm
`dev` have the same shape. The last writer wins on every walk and watch event (#348), and under the #336 reconcile each
publish would delete the other file's facts. Separately, the `contentHashShort(filePath)` fallbacks (`:148`, `:302`,
`:466`) hash the walk path, which is absolute when the root is, so those IDs depend on the checkout location.

## What Changes

- **Every cfgfile entity's instance gains its declaring file.** `instance = <today's instance>-<hash8(rel)>`, where
  `rel` is the manifest's path relative to the source root, slash-separated. The fallbacks hash `rel`, not the walk
  path. The `config.file-path` fact stays as it is.
- **Gradle `project`** keys by `rel` (covered by the rule above), so same-named directories no longer collide.
- **Edges are unchanged in shape:** each manifest's `requires`/`contains` edges point at its own entities.
- **Tests first:** a repro ingests two manifests declaring one coordinate with different scope/configuration and shows
  one ID with flipping facts; after the change, two IDs with stable facts. Same for two Dockerfiles and `a/core` vs
  `b/core`. A test pins IDs as independent of the checkout's absolute location.

## Decisions you are making by approving

- **Identity keyed by declaring file** (option A in `design.md`), not a shared artifact node plus a per-declaration
  entity (option B). "Which manifests use log4j 2.17" becomes a name/version query over many nodes, not the in-edges
  of one node. Nothing in SemSource or the scorecard reads that join today.
- **The rule covers every cfgfile entity type**, including Go/npm dependencies, npm packages, Maven projects, POM
  modules and Go modules, not only the three types #348 names. They have the same defect.
- **A clean storage break that rides beta.11**, which already needs a reseed (IDs changed in #329/#338 since beta.10).
  No aliasing of old IDs.
- **cfgfile stays out of the #336 reconcile and the #349 `idSpace` check in this change.** Opting in is a follow-up
  issue, now unblocked.

## Out of scope

- The #336 reconcile opt-in and the `idSpace` row for cfgfile (follow-up).
- Option B's shared artifact graph (revisit only with a consumer asking for the cross-manifest join).
- The legacy `RawEntity` path in `handler/cfgfile/handler.go` (#244).
