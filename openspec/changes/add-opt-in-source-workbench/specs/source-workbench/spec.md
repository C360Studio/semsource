## ADDED Requirements

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

#### Scenario: Headless client reads capabilities

- **WHEN** a headless client requests the workbench capability response without any UI profile running
- **THEN** it receives product/project identity, response version, readiness, query surfaces, optional
  action availability, and materialized-view availability

#### Scenario: Backend is partially ready

- **GIVEN** source discovery is complete but a required query index is not ready
- **WHEN** a client requests workbench capabilities
- **THEN** the response reports the authoritative partial-readiness state and does not advertise the
  affected query action as ready

#### Scenario: Optional action is not implemented

- **GIVEN** OKF or materialized-view behavior is not present in this SemSource version
- **WHEN** a client requests workbench capabilities
- **THEN** the response marks the action unavailable or omits it according to the versioned contract
  without requiring a failing feature request

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

### Requirement: Explicit directed graph projection

The workbench graph drill-down SHALL consume explicit nodes, directed relationships, property facts,
evidence metadata, and a query or view revision from the governed backend query contract. It SHALL
preserve opposite-direction and parallel-predicate relationships and SHALL NOT infer relationship
semantics from the string shape of a property value. The revision SHALL trigger synchronization even
when entity and relationship identifiers are stable.

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

### Requirement: Backend parity for workbench actions

Every state-changing or artifact-producing workbench action SHALL invoke a SemSource-owned backend
contract also available to non-UI automation through HTTP, MCP, or CLI. The UI SHALL NOT define a
browser-only source, materialized-view, evidence, or OKF policy.

#### Scenario: Workbench executes an existing search action

- **GIVEN** SemSource advertises a supported source or graph search action
- **WHEN** a user invokes that action in the workbench
- **THEN** the browser calls the same SemSource search contract available to headless clients and
  displays its result and readiness metadata
