## Why

SemSource's headless graph and MCP service is useful when embedded in other sem* products, but its
standalone experience hides source readiness, project knowledge, evidence, and export behind APIs.
An opt-in workbench closes that usability gap without making a UI part of SemSource's default runtime.

## What Changes

- **BREAKING**: Repurpose the existing opt-in Compose `ui` profile from a sibling SemTeams checkout to
  the SemSource workbench. Users who relied on `docker compose --profile ui up` for SemTeams must move
  that packaging into SemTeams or connect SemTeams to headless SemSource APIs.
- Keep `docker compose up` and embedded/service deployments headless and behaviorally unchanged.
- Consume a pinned release of the reusable `semstreams-ui` application rather than requiring a sibling
  checkout or copying another graph implementation into SemSource.
- Make SemSource responsible for source status, materialized-project-view, provenance/evidence, and
  project-knowledge interoperability contracts; keep the reusable shell and graph rendering in
  `semstreams-ui`.
- Require a cross-product best-of audit and canonicalization of the shared graph visualization before
  the workbench depends on it.
- Explicitly supersede both UI ownership decisions in archived `add-ui-profile`: SemTeams no longer
  owns the app launched by SemSource's profile, and `../semteams/ui` is no longer its default context.

## Non-goals

- Making a UI mandatory for SemSource, its MCP surface, or downstream sem* product integration.
- Changing SemTeams code or defining how SemTeams packages its own UI in this change.
- Copying Svelte graph components into the SemSource repository.
- Implementing materialized project views, OKF import/export, or one-action installation in this
  change. Proposed follow-on IDs are `materialize-project-views`, `add-okf-interop-mvp`, and
  `add-one-action-local-start`; none is created or approved by this proposal.
- Turning the workbench into a generic SemStreams administration console or a whole-graph authority.
- Changing SemStreams graph storage, query, indexing, or lifecycle primitives.

## Consumers

- Standalone SemSource users gain the optional workbench through the existing `ui` flag.
- SemTeams must own any future SemTeams UI packaging and consume SemSource's headless contracts.
- SemSpec, SemDragon, SemOps, and other embedded consumers remain on the headless HTTP, MCP, NATS,
  GraphQL, and graph contracts unless they explicitly link to the workbench.
- `semstreams-ui` consumes SemSource browser-facing contracts through a SemSource product profile.

## Capabilities

### New Capabilities

- `source-workbench`: the SemSource-specific workbench experience, capability boundaries, degradation
  behavior, evidence presentation, and graph-drill-down role.

### Modified Capabilities

- `ui-profile`: replaces the optional SemTeams checkout with a pinned SemSource workbench while
  preserving the default headless core.

## Impact

- `docker compose --profile ui up` intentionally changes meaning and requires release notes and a
  SemTeams handoff.
- SemSource Compose/Caddy packaging gains a released-workbench path and drops its sibling SemTeams UI
  dependency.
- `semstreams-ui` needs a governed SemSource product profile and a canonical shared graph surface.
- Browser-facing source, readiness, evidence, and future materialized-view/OKF contracts require
  explicit compatibility tests.
- The public roadmap and OKF design note need to describe the workbench, headless-default promise, and
  sequenced dependency changes consistently.
