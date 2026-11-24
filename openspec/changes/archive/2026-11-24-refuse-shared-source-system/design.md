# Design: refuse sources that would share an entity-ID system

## Context

A source entry becomes one component through `internal/sourcespawn` (`astComponentConfig`, `docComponentConfig`). The
component's root and the project it was given then determine the system segment of every ID it mints:

| Kind | System for one root | Where |
| --- | --- | --- |
| ast-source | `ScopedSystemSlug(project, version)`, where `project` is already branch-scoped by sourcespawn (`BranchScopedSlug`) and path-derived when the entry has none | `sourcespawn/components.go:74-110`; `processor/ast-source/component.go:260` |
| doc-source | `SystemSlug(project)` when `project` is set, else `SystemSlug(root)`; one component may list several roots | `handler/doc/handler.go:85-89` |

Sources reach a running deployment by three routes, and each must refuse a collision:

1. **The file.** `semsource validate` → `cli.Validate` → `config.LoadConfig` + `Config.Validate`. `semsource run` calls the
   same `Validate` before boot.
2. **Boot.** File sources, the desired-manifest sources that earlier `add_source` requests persisted, and the submodule
   pins `subwatch.Expand` discovers (`cmd/semsource/discovery.go:158`) are planned together and spawned.
3. **Runtime `add_source`.** `sourcespawn.AddWithChecker` builds the new entry's component specs and compares them with
   the existing component snapshot (`internal/sourcespawn/sourcespawn.go:152-178`). The running set never changes; the
   entry is persisted for the next boot (ADRs 094/100).

## Decisions

### D1 — The collision key is (kind, org, system), computed by the code that mints the system

Two roots collide when they have the same component kind (ast-source or doc-source), the same org, and the same system
as computed in the table above. Kind is part of the key because ast and doc IDs differ in their domain segment and
cannot meet. The key is computed from the component config sourcespawn builds, using the same helpers the components
use (`entityid.ScopedSystemSlug`, `entityid.SystemSlug`). It is not computed from a separate reading of `SourceEntry`,
because a second derivation would drift from the first. `sourceScopeSystems` (`cmd/semsource/source_scopes.go`)
already reads systems from component configs this way and is the model.

A re-add of the same source produces the same instance name. `AddWithChecker` already treats that as an update, not a
second writer, so specs whose instance name is already registered are left out of the comparison.

### D2 — A collision that includes a submodule pin is admitted

`subwatch.pinEntries` gives every parent that links one submodule at one SHA the same project and version on purpose,
and marks each entry with an `InstanceSuffix` so the instances stay distinct. The pins carry the same content, so their
statement sets agree. The multi-source quickstart (`docs/QUICKSTART.md`), the `onboarding-quickstart` scenario
"Shared code dedups across sources" and ADR-0012 also register the same code **standalone**, at the pin's project and
version, and document that it merges with the pin into one entity.

So a collision is admitted when at least one root in it is a submodule pin. For ast-source, an equal system means equal
project and version (`ScopedSystemSlug`), and a pin's version is its gitlink SHA. For doc-source, the pin's project
carries the SHA (`<slug>-<sha12>`). Either way, a standalone source that collides with a pin has declared the pin's
exact pinned identity, which is a claim of the same content. Every other collision is refused, including two
operator-declared sources at one project and version with no pin among them. (Owner ruling, 2026-11-24, after the
first approval.)

Boot recognises pins from the discovery record (kind `submodule`). A runtime `add_source` of a standalone source at a
pin's identity is judged the same way against the pins already registered. The file has no pins, so `semsource
validate` refuses only operator-declared collisions.

### D3 — One check function, called from four places

`sourcespawn.CheckSystemCollisions(components) error` takes the component configs and returns an error naming each pair
of colliding instances and the shared system. It is called:

- by `Config.Validate`, after it builds specs for the file's sources. Repo sources whose specs need a checkout that
  doesn't exist yet are checked at boot instead; validate does not clone;
- by boot, over the planned set, before any component is spawned. A collision stops boot with that error;
- by `AddWithChecker`, over the existing snapshot plus the pending specs. It returns `CodeValidationFailed`, so the
  manifest reply maps it to `validation_failed`, and nothing is persisted;
- by the ast-source and doc-source component `Validate` methods, over their own roots, for component JSON written by
  hand.

### D4 — Boot refuses rather than starting the non-colliding rest

Refusing at boot means a collision persisted before this change stops the service, and the error names the instances
to remove. Starting the rest and skipping the later entry would hide the problem behind a running service, which is the
silent failure the ruling replaces. Today a manifest entry can only be removed through the running service's
`remove_source`, so an operator needs a way out while boot refuses. Boot logs the colliding instance names, and the
open question below asks which route to give.

### D5 — The #336/#347 workaround goes away in the same change

Once admission guarantees one writer per system, `markSharedSystems` and `pathWatcher.sharedSystem` have nothing to
mark. Deleting them, and the `// shortcut:` they carry, is part of this change. The spec text that excludes shared roots
from reconcile and watch-event container publishes is removed in the same delta.

## Open questions

- **OPEN-Q (D4): how does an operator remove a persisted colliding manifest entry while boot refuses?** Options:
  (a) boot starts only source-manifest, so `remove_source` works, and refuses the source components; (b) a one-shot CLI
  `semsource remove --manifest <instance>` that edits the desired manifest offline; (c) document deleting the
  desired-config KV key by hand. My recommendation is (b). (a) adds a degraded boot mode, and (c) is error-prone.

## Testing

- **The reproduction first** (DoD on #349). With the guard bypassed, two ast roots under one project, each holding
  `main.go` with different content, and two doc roots each holding `README.md`, run through ast-source/doc-source →
  graph-ingest → `ENTITY_STATES`. The test asserts that one ID carries statements from both roots, and it is the failing
  baseline. With the guard in place, the same config is refused at each of the four entry points.
- Unit table for `CheckSystemCollisions`:
  - same project + same version collides;
  - same project + different version does not;
  - different branch scope does not;
  - a path-derived project that matches an explicit one collides;
  - doc multi-root under one project collides;
  - two pins at one SHA are allowed;
  - a pin plus a standalone source at the pin's project and version is allowed;
  - two operator sources at one project and version, with no pin, collide;
  - re-adding the same instance does not.
- The existing #347 folder and reconcile tests keep passing after `sharedSystem` is removed.
