# Fold same-coordinate declarations within one cfgfile manifest

Approved-by: Coby Leuschke (2026-11-25) sha256:3de5c126b709

Track: design (modifies an entity-identity Requirement; changes some cfgfile IDs, which rides the beta.11 reseed).

## Why

#348 keyed every cfgfile entity by its declaring manifest, so two manifests no longer share an ID. One manifest can
still declare one coordinate twice, and each declaration mints the same ID with different facts
(`handler/cfgfile/entities.go`): a Gradle `group:name:version` under `implementation` and `testImplementation`
(`:520`), a Maven `g:a:v` under two scopes (`:371`), an npm `name@version` in both `dependencies` and
`devDependencies` (`:203`). Maven `<type>`/`<classifier>` are never parsed (`parsers.go:247`), so a `test-jar` and the
main jar of one coordinate are one ID too. Two `FROM` images that slugify alike (`foo/bar:1`, `foo-bar:1`) share an
image ID (`:247`). One file then writes two states under one ID in one pass, and its `requires` edges list that ID
twice. This blocks cfgfile opting into the #336 reconcile, where each declaration would delete the other's facts.

## What Changes

- **Usage attributes fold into one entity.** Per manifest, a dependency is keyed by what it *is* (coordinate and
  version); the ways it is *used* become facts on that one entity. Gradle configurations and Maven scopes become
  multi-valued facts. npm `dependency-kind` is `npm-prod` if any declaration is a prod dependency, else `npm-dev`. Go
  `indirect` is true only when every declaration is indirect. Each manifest's `requires` edges name each ID once.
- **Artifact attributes stay in identity.** The Maven parser reads `<type>` and `<classifier>`; a dependency that sets
  either keys on them too, so a `test-jar` is its own entity. A plain jar keeps today's ID.
- **Image instances hash the raw image reference** after the slug, so images that slugify alike stay distinct.
- **Tests first:** each shape above fails on main with one ID carrying conflicting facts or a duplicate edge.

## Decisions you are making by approving

- **When a manifest lists the same dependency twice, you get one entity per dependency.** It lists every
  scope/configuration it is used under, not one entity per declaration line. "Does this manifest use X" stays one
  answer. Recommended over minting a separate entity per scope.
- **A Maven test-jar or classified artifact becomes its own entity.** A plain jar dependency keeps the ID it has today.
- **Every Docker base-image ID changes once.** The ID gains a short hash of the exact image name. This rides the
  beta.11 reseed, which every cfgfile ID already needs since #348; no aliasing.
- **cfgfile still stays out of the #336 reconcile in this change.** Opting in becomes its own follow-up, now unblocked.

## Out of scope

- The #336 reconcile opt-in for cfgfile (follow-up issue on merge).
- Gradle map notation and version catalogs, which the regex parser does not read today.
- A cross-manifest shared artifact graph (option B of the #348 design; still no consumer).
