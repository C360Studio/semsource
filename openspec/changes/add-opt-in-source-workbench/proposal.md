## Why

SemSource's headless graph and MCP service is useful when embedded in other sem* products, but its
standalone experience hides source readiness, project knowledge, evidence, and export behind APIs.
An opt-in workbench closes that usability gap without making a UI part of SemSource's default runtime.

The sem* ecosystem already contains several descendants of a capable Svelte graph interface. Their
strongest behaviors are distributed across SemStreams UI, SemSpec, SemDragon, and SemConnect. The
SemSource workbench should deliberately port and test those best-of behaviors in one SemSource-owned
reference implementation rather than inherit another product application or add a cross-repo runtime
dependency.

## What Changes

- **BREAKING**: Repurpose the existing opt-in Compose `ui` profile from a sibling SemTeams checkout to
  the SemSource workbench. Users who relied on `docker compose --profile ui up` for SemTeams must move
  that packaging into SemTeams or connect SemTeams to headless SemSource APIs.
- Keep `docker compose up` and embedded/service deployments headless and behaviorally unchanged.
- Build the SemSource-owned Svelte 5 workbench under `ui/`, including its contracts, component tests,
  Playwright suite, production Dockerfile, and release image.
- Port selected behavior from audited sem* UI donors into the local implementation with explicit
  provenance, rejection rationale, and behavior-level regression tests.
- Make SemSource responsible for the workbench application, source/project composition,
  provenance/evidence presentation, accessibility, packaging, and release acceptance.
- Keep governed graph projection in SemStreams. Ship the non-graph source/readiness/search MVP while
  `graph_projection` is unsupported; enable graph drill-down only after semstreams#533 is adopted and
  live-tested.
- Explicitly supersede both UI ownership decisions in archived `add-ui-profile`: SemTeams no longer
  owns the app launched by SemSource's profile, and `../semteams/ui` is no longer its default context.

## Non-goals

- Making a UI mandatory for SemSource, its MCP surface, or downstream sem* product integration.
- Changing SemTeams code or defining how SemTeams packages its own UI in this change.
- Importing, building, or releasing `semstreams-ui` or another product UI as a SemSource dependency.
- Publishing a generic component package before the SemSource workbench proves a stable model.
- Implementing materialized project views, OKF import/export, or one-action installation in this
  change. Proposed follow-on IDs remain `materialize-project-views`, `add-okf-interop-mvp`, and
  `add-one-action-local-start`; none is created or approved by this proposal.
- Turning the workbench into a generic SemStreams administration console or a whole-graph authority.
- Changing SemStreams graph storage, query, indexing, or lifecycle primitives.

## Consumers

- Standalone SemSource users gain the optional workbench through the existing `ui` flag.
- SemTeams owns its application packaging and consumes SemSource's headless contracts.
- SemSpec, SemDragon, SemOps, and other embedded consumers remain on the headless HTTP, MCP, NATS,
  GraphQL, and governed graph contracts.
- Other sem* UI teams may later use the proven SemSource workbench as a reference or extraction
  candidate, but this change creates no dependency in either direction.

## Capabilities

### New Capabilities

- `source-workbench`: the SemSource-owned workbench experience, capability boundaries, degradation
  behavior, evidence presentation, search, and capability-gated graph-drill-down role.

### Modified Capabilities

- `ui-profile`: replaces the optional SemTeams checkout with a SemSource-owned workbench artifact
  while preserving the default headless core.

## Impact

- `docker compose --profile ui up` intentionally changes meaning and requires release notes and a
  SemTeams handoff.
- SemSource gains a `ui/` application, Node-based test/build gates, a production UI image, and
  SemSource-owned Playwright coverage.
- Compose, Caddy, Task, and smoke scripts drop sibling UI paths and SemTeams-owned dependencies.
- Browser-facing source, readiness, evidence, and future materialized-view/OKF contracts require
  explicit compatibility tests.
- SemStreams #533 gates only graph-enabled drill-down; it does not block the useful non-graph MVP.
