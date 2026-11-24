# Refuse sources that would share an entity-ID system

Approved-by: Coby Leuschke (2026-11-24) sha256:31f3f142f323

Track: design (a new admission rule at three entry points, and a changed requirement in `entity-publish-integrity`).

## Why

Two ast-source roots that use the same `project` and `version`, or two doc-source roots under the same `project`, mint
their entity IDs under the same system. A file at the same relative path in each root then maps to one entity ID, and
each root publishes a different statement set for it (#349):

- **ast-source:** the system is `ScopedSystemSlug(project, version)` (`processor/ast-source/component.go:260`), and file
  and symbol instances come from the root-relative path.
- **doc-source:** with `project` set, `system()` returns `SystemSlug(project)` for every root
  (`handler/doc/handler.go:85-89`), and the doc and chunk instances come from the root-relative path
  (`handler/doc/entities.go:318`).

Today the two publishes merge into a mix of both roots. With the #336 reconcile on, they become last-writer-wins, and
each root deletes the other's statements. #336 and #347 worked around this: ast-source detects roots that share a system
(`markSharedSystems`, `component.go:1262`), skips reconcile for them and publishes no watch-event container
(`// shortcut:` at `component.go:938`, `fileset.go:155,194`). That detection only sees watch paths inside one
component, while `sourcespawn` gives each source entry its own component. So the common case, two source entries with
one project, is not detected at all.

Ruling: refuse at config time. Do not put the root into identity. The reproduction becomes part of the fix's test (owner,
2026-11-24, comment on #349).

## What Changes

- **A source admission rule.** A set of sources is refused when two roots of one kind (ast-source, doc-source) would
  mint IDs under one org and system. The error names both sources and asks for a distinct `project` (or `version`) for
  each. The one allowed exception is a collision that includes a submodule pin. Boot-time
  discovery expands the same submodule at the same gitlink SHA from two parents into byte-identical sources
  (`git-submodule-ingestion` "A shared submodule dedups across consumers"), and the quickstart and ADR-0012 register the
  same code standalone at the pin's project and version so it merges with the pin. A source that matches a pin's pinned
  identity therefore shares its IDs; any other collision is refused.
- **Checked at every entry point that admits sources:**
  - `semsource validate` and the `Config.Validate` call at the start of `semsource run`, over the file's `sources`;
  - boot, over the full set it is about to spawn: file sources, desired-manifest sources and discovered submodule pins;
  - a runtime `add_source` request, which is refused with `validation_failed` when it collides with a source already
    registered;
  - ast-source and doc-source component `Validate`, for component JSON that lists several roots directly.
- **The workaround is removed.** `markSharedSystems`, `pathWatcher.sharedSystem` and the `// shortcut:` at
  `component.go:938` go away. A root admitted under the new rule never shares its system, so every ast-source root
  reconciles and publishes watch-event containers under the existing rules.

Consumers see no read-surface change and no entity ID change. Operators whose config relied on a shared project get a
refusal that names the colliding sources, instead of a silently mixed graph.

## Non-goals

- Putting the root into identity, a storage break or a reseed. That stays open for later if someone needs the
  shared-project setup.
- git-source system collisions (#354), cfgfile manifest-scoped IDs (#348) and the git poll slug drift (#350).
- Two live folders whose paths sanitize to one ID (`a-b` and `a/b`). That is an identity limit inside one root and
  stays as the spec documents it.
- Healing a graph that already holds mixed entities. After an operator gives each root a distinct project, the new
  systems seed fresh IDs and the lifecycle pass marks the old ones stale. No IDs are rewritten.
