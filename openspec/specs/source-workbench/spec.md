# source-workbench Specification

## Purpose
The SemSource workbench is the project-owned, opt-in operator UI for the ingestion service: a
SvelteKit app committed under `ui/`, built and released from this repository, and wired into the
Docker Compose stack only under the opt-in `ui` profile (`docker-compose.yml`), sitting behind
Caddy alongside the always-on `semsource` and `semembed` core services. It may port audited
behavior from sibling sem* UIs but owns it locally rather than depending on their source trees,
packages, or release process — `docker-compose.ui-dev.yml` and a pinned `SEMSOURCE_UI_IMAGE`
release digest are the only two ways to run it. `processor/source-manifest` serves the workbench's
versioned bootstrap contract at `GET /source-manifest/capabilities` (`workbench_capabilities.go`):
product/project identity, source and structural/semantic index readiness collected over NATS
request/reply, and a map of query and action capabilities each marked `ready`, `not_ready`, or
`unsupported` with a machine-readable reason, so the browser never has to discover a missing route
by trial and error.

The workbench itself (`ui/src`) leads with project identity, readiness, and source inventory
(`WorkbenchShell.svelte`) before offering the whole-graph drill-down as an investigation surface
rather than the primary explanation of a project. Every state-changing action it exposes calls the
same SemSource HTTP contract available to headless clients, and the graph view consumes the
existing `POST /code-context/context` contract with `want: ["graph"]` rather than a UI-only
projection endpoint, tolerating truncated or incoherent-revision responses by retaining previously
resolved nodes and facts instead of discarding them.
## Requirements
### Requirement: SemSource owns the reference workbench

The optional workbench SHALL be implemented, tested, packaged, and released from the SemSource
repository under `ui/`. SemStreams UI, SemSpec, SemDragon, and SemConnect MAY provide audited donor
behavior, but the workbench SHALL NOT depend on their source trees, packages, build outputs, images, or
release processes.

Ported behavior SHALL become locally owned and SHALL be protected by SemSource behavior tests. A
future shared package or upstream reference MAY be proposed only after this workbench proves a stable
contract; it is not part of this capability.

#### Scenario: Donor repositories are unavailable

- **GIVEN** no sibling sem* UI checkout exists
- **WHEN** the SemSource workbench is built and tested
- **THEN** its source, dependencies, fixtures, tests, and artifact remain complete
- **AND** no donor repository is resolved at build time or runtime

### Requirement: Project-first SemSource workbench

The optional SemSource workbench SHALL lead with project identity, source status, readiness, freshness,
search, and source inventory. Whole-graph visualization SHALL be an investigation drill-down rather
than the only or primary project explanation.

#### Scenario: Basic backend has no materialized views

- **GIVEN** SemSource advertises source status, search, and graph query but no materialized-view
  capability
- **WHEN** the workbench loads
- **THEN** it presents the supported project, readiness, source, search, and graph surfaces without
  showing failed or misleading materialized-view and export actions

### Requirement: SemSource workbench capability contract

SemSource SHALL expose a versioned workbench capability response through its headless HTTP surface.
The response SHALL identify the SemSource product and project, report authoritative source/graph
readiness, enumerate available query surfaces, distinguish supported and unavailable actions with a
machine-readable reason, and state whether materialized project views are available. Additive fields
SHALL preserve compatibility within the declared response version.

The version-1 document SHALL be served by `GET /source-manifest/capabilities`. It SHALL use maps keyed
by stable query and action capability IDs. Availability SHALL be `ready` when a verified route exists
and its named readiness dependencies are ready, `not_ready` when the route exists but a dependency is
building, degraded, or unavailable, and `unsupported` for a known contract that is not implemented.
Unknown fields and map keys SHALL be additive-compatible within version 1. The project key SHALL be
the configured deployment namespace with `identity_kind: deployment_namespace`; it SHALL NOT be
represented as or interpreted as a six-part graph entity ID.

#### Scenario: Headless client reads capabilities

- **WHEN** a headless client requests the workbench capability response without any UI profile running
- **THEN** it receives product/project identity, response version, readiness, query surfaces, optional
  action availability, and materialized-view availability

#### Scenario: Backend is partially ready

- **GIVEN** source discovery is complete but a required query index is not ready
- **WHEN** a client requests workbench capabilities
- **THEN** the response reports the authoritative partial-readiness state and does not advertise the
  affected query action as ready

#### Scenario: Optional readiness responder is unavailable

- **GIVEN** source status is available but an index readiness responder is missing, times out, or
  returns an undecodable payload
- **WHEN** a client requests workbench capabilities
- **THEN** the response remains HTTP 200 with the affected query `not_ready`
- **AND** the affected signal has state `unknown` and reason `status_unavailable`
- **AND** raw transport or payload detail is not exposed
- **AND** repeated discovery requests do not count expected responder absence against the shared NATS
  circuit breaker

#### Scenario: An all-reported source is errored

- **GIVEN** aggregate source phase is `ready` but a reported source is errored, has a nonzero error
  count, or carries a last error
- **WHEN** a client requests workbench capabilities
- **THEN** source and overall readiness are not ready
- **AND** the response carries a sanitized retryable reason without the source error detail

#### Scenario: Optional action is not implemented

- **GIVEN** OKF or materialized-view behavior is not present in this SemSource version
- **WHEN** a client requests workbench capabilities
- **THEN** the response marks the known action `unsupported` with a machine-readable reason
- **AND** the client does not need to call a nonexistent feature route

#### Scenario: Version-1 server adds a capability

- **GIVEN** a version-1 client ignores unknown fields and capability keys
- **WHEN** a newer version-1 server adds a query, action, contract, or optional metadata field
- **THEN** the client continues to use known fields without a contract-version change

### Requirement: Capability-driven product composition

The workbench SHALL derive available routes and actions from a SemSource-owned capability contract. It
SHALL NOT assume SemTeams, SemOps, flow-builder, trajectory, or other product-specific routes exist.
Unsupported capabilities SHALL be omitted or displayed as unavailable with a concrete reason rather
than discovered through repeated failing requests.

#### Scenario: Embedded product routes are absent

- **GIVEN** a standalone SemSource backend does not advertise SemTeams or flow-builder capabilities
- **WHEN** the workbench initializes
- **THEN** it does not request those routes or classify their absence as degraded SemSource health

### Requirement: Truthful evidence presentation

The workbench SHALL render confidence, provenance, source, authority, freshness, and timestamps exactly
as supplied by SemSource. Missing values SHALL remain unknown or omitted and SHALL NOT be synthesized.
Updates to those attributes SHALL refresh the graph and detail surfaces even when entity and
relationship identities are unchanged.

#### Scenario: Evidence metadata is absent

- **GIVEN** a graph response omits confidence, provenance, source, authority, or timestamp values
- **WHEN** the workbench renders the entity or relationship
- **THEN** it shows the values as unknown or omits them instead of inventing defaults

#### Scenario: Stable identity receives new evidence

- **GIVEN** an entity retains the same deterministic ID while its evidence, confidence, freshness, or
  authority attributes change
- **WHEN** the workbench receives the updated state
- **THEN** both its graph projection and detail view display the new attributes

### Requirement: Accessible graph investigation

The graph drill-down SHALL provide a synchronized keyboard- and screen-reader-accessible entity or
result navigator in addition to pointer interaction with the WebGL canvas. Selection in either surface
SHALL identify the same entity and expose its details.

#### Scenario: User navigates without a pointing device

- **WHEN** a keyboard user searches results and selects an entity through the accessible navigator
- **THEN** the graph selection and entity detail surface update without requiring a canvas click

### Requirement: Capability-gated explicit directed graph projection

When SemSource advertises `graph_projection` as ready, the graph drill-down SHALL consume explicit
nodes, directed relationships, property facts, evidence metadata, and a query or view revision from
the governed backend query contract. It SHALL preserve opposite-direction and parallel-predicate
relationships and SHALL NOT infer relationship semantics from the string shape of a property value.
The revision SHALL trigger synchronization even when entity and relationship identifiers are stable.

SemSource SHALL expose this projection through the existing `POST /code-context/context` contract
with `want: ["graph"]`. It SHALL NOT create a parallel projection endpoint, adapt display-oriented
fusion role maps into edges, or require GraphQL for this capability. Node and edge handles SHALL be
treated as opaque. A handle absent from returned node details MAY be represented only when the backend
supplies it as an explicit edge endpoint, and SHALL remain visibly unresolved.

Graph completeness SHALL be evaluated from the graph facet independently of top-level fusion
truncation. The workbench MAY delete previously displayed nodes or edges absent from a replacement
only when `graph.truncated` is false and `view_revision` is coherent, has equal nonzero start and end
values, and therefore identifies a meaningful complete view. A truncated, incoherent, or zero-revision
projection SHALL retain prior nodes and edges while merging supplied updates and SHALL be presented as
partial rather than authoritative deletion.

When `graph_projection` is unsupported or not ready, the workbench SHALL show the supplied concrete
reason, SHALL remain useful through supported non-graph capabilities, and SHALL NOT request, infer, or
synthesize a replacement graph payload.

#### Scenario: Governed graph projection is unavailable

- **GIVEN** SemSource advertises `graph_projection` as unsupported with reason
  `upstream_contract_pending`
- **WHEN** the workbench loads
- **THEN** it presents source, readiness, project, and search surfaces that are supported
- **AND** it shows graph drill-down as unavailable with the supplied reason
- **AND** it does not request or infer graph nodes or relationships

#### Scenario: Parallel directed relationships are rendered

- **GIVEN** two entities have multiple predicates in one direction and a different predicate in the
  opposite direction
- **WHEN** the workbench renders the backend projection
- **THEN** it preserves each directed relationship without collapsing it into an undirected edge

#### Scenario: Literal resembles an entity identifier

- **GIVEN** a property literal has a dotted string value that resembles a deterministic entity ID but
  the backend does not classify it as a relationship
- **WHEN** the workbench renders the property
- **THEN** it remains a literal fact and does not create a graph edge

#### Scenario: Edge endpoint details are not returned

- **GIVEN** the backend returns an explicit directed edge whose source or target handle has no node
  detail in the bounded projection
- **WHEN** the workbench renders the projection
- **THEN** it creates an unresolved display stub only for that explicit opaque handle
- **AND** it does not infer identity, label, type, facts, or evidence for the endpoint

#### Scenario: Partial projection omits previously displayed items

- **GIVEN** a replacement graph projection is truncated, has an incoherent revision, or has a zero
  revision bound
- **WHEN** nodes or edges from the previous view are absent from the replacement
- **THEN** the workbench retains the absent items and marks the projection partial
- **AND** supplied nodes, facts, edges, evidence, and revision values still refresh

#### Scenario: Complete projection omits previously displayed items

- **GIVEN** a replacement graph projection is untruncated and has an equal coherent nonzero start and
  end revision
- **WHEN** nodes or edges from the previous view are absent from the replacement
- **THEN** the workbench removes those absent items

### Requirement: Backend parity for workbench actions

Every state-changing or artifact-producing workbench action SHALL invoke a SemSource-owned backend
contract also available to non-UI automation through HTTP, MCP, or CLI. The UI SHALL NOT define a
browser-only source, materialized-view, evidence, or OKF policy.

#### Scenario: Workbench executes an existing search action

- **GIVEN** SemSource advertises a supported source or graph search action
- **WHEN** a user invokes that action in the workbench
- **THEN** the browser calls the same SemSource search contract available to headless clients and
  displays its result and readiness metadata

### Requirement: The workbench tolerates every legitimate backend state

The workbench SHALL render usefully for every state the backend contract can legitimately report —
including `reset_required` index states, fusion error statuses beyond 400/503/504, and source
inventories containing multiple instances of one repo (distinct branch/language) — degrading the
affected panel, never the whole page.

#### Scenario: reset_required degrades gracefully

- **WHEN** the capabilities contract reports index state `reset_required`
- **THEN** the workbench renders with the affected capability marked degraded/not-ready and the
  rest of the page functional

#### Scenario: Two branches of one repo

- **WHEN** the source inventory lists two sources differing only by branch
- **THEN** the sources list renders both entries (no duplicate-key failure)

### Requirement: Projection retention preserves facts

The workbench SHALL retain previously resolved graph items WITH their facts when a projection
response is truncated, incoherent, or zero-revision — a prior-resolved node SHALL never be downgraded to an
unresolved stub by a partial update.

#### Scenario: Partial update keeps resolved details

- **WHEN** a node resolved in an earlier projection is absent from a truncated later projection
- **THEN** the workbench continues to show the node with its previously known facts

### Requirement: Readiness display refreshes

Panels gated on readiness SHALL re-poll while not-ready and unlock without a manual page reload;
the overall-readiness banner SHALL reflect (or explicitly name) the readiness dependencies it
covers, never reporting blanket readiness while an advertised dependency is still building.

#### Scenario: Panel unlocks on readiness

- **WHEN** the semantic index becomes ready while the page is open
- **THEN** the code search panel becomes usable without a manual reload

### Requirement: Drill-down presents resolved knowledge first

The graph drill-down SHALL present resolved entities before unresolved endpoints, select the
queried symbol's entity by default when present, and group/de-emphasize unresolved endpoint
classes (`builtin:`, `external:`, unhydrated in-graph references) so investigation starts from
knowledge, not noise.

#### Scenario: Queried symbol selected

- **WHEN** a drill-down query resolves the queried symbol
- **THEN** that entity is selected by default and unresolved endpoints are grouped below the
  resolved entities
