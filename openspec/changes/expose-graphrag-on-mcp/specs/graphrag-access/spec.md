## Purpose

Make the substrate's graph-query surface — community-scoped search, corpus-wide thematic search,
and graph summary — reachable by an agent over MCP. Each tool gates on the capability it actually
depends on, and every answer discloses which retrieval path produced it, so neither a refusal nor a
non-community answer can be mistaken for a graph that has nothing to say.

## ADDED Requirements

### Requirement: The graph-query surface is reachable over MCP

SemSource SHALL expose the substrate's graph-query surface through the MCP gateway, routing to the
existing semstreams subjects `graph.query.localSearch`, `graph.query.globalSearch`,
`graph.query.summary`, and `graph.query.searchGraph`. Community-scoped search, corpus-wide thematic
search, and graph summary SHALL each be reachable through an advertised tool. Corpus-wide thematic
search MAY be routed through `graph.query.searchGraph`, which delegates to `graph.query.globalSearch`
and returns its response unchanged when non-empty; a separate tool per subject is not required. Tool
count, names, and argument grouping are fixed by design.

#### Scenario: The roster covers the surface

- **WHEN** an MCP client lists the gateway's tools
- **THEN** community-scoped search, corpus-wide thematic search, and graph summary are each
  reachable through an advertised tool

#### Scenario: A call reaches the substrate handler

- **GIVEN** a stack whose community index is populated
- **WHEN** an agent calls the community-scoped search tool with an entity ID and a query
- **THEN** the substrate's `graph.query.localSearch` handler serves the request and its response is
  what the tool reports

### Requirement: Each tool gates on the capability it actually requires

A tool SHALL be gated on the capability its own routed subject depends on, and SHALL NOT be gated on
a capability it does not use. Community-scoped search depends on the community index and SHALL be
capability-gated. Graph summary does not involve clustering and SHALL remain available at every
tier. Corpus-wide thematic search answers at every tier through non-community retrieval paths and
SHALL remain available, subject to the disclosure requirement below.

#### Scenario: Graph summary is not gated on clustering

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls the graph summary tool
- **THEN** the call succeeds and returns the substrate's summary, because that subject does not
  depend on clustering

#### Scenario: Thematic search still answers without clustering

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls the corpus-wide thematic search tool
- **THEN** the call succeeds through a non-community retrieval path rather than being refused

### Requirement: An unavailable capability is stated, never simulated

Where a tool's routed subject requires a capability the running stack does not provide, that call
SHALL fail with `isError: true` and a message naming the missing capability and the configuration
that enables it. It SHALL NOT return an empty success, a zero-count result, or a bare transport
error — an agent cannot distinguish any of those from "the graph has nothing to say". The condition
SHALL be determined before the request is issued, so the agent receives a stated capability
condition rather than a timeout.

#### Scenario: Community-scoped search without clustering is refused truthfully

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls the community-scoped search tool
- **THEN** the result is `isError: true` and states that community search is not enabled in this
  configuration and what enables it

#### Scenario: A missing responder is never an empty result

- **GIVEN** the routed subject has no subscribed handler, because the substrate registers
  community-scoped handlers only once the community index is available
- **WHEN** an agent calls the affected tool
- **THEN** the result names the unavailable capability, never a successful result with zero entities
  and never an unexplained timeout

### Requirement: Every answer discloses its retrieval path

A tool that can answer through more than one retrieval path SHALL disclose, in its result, whether
the answer was community-backed. A non-community answer SHALL NOT be presented in a way that
implies community reasoning. The disclosure SHALL be derived from fields the substrate returns and
SHALL sit alongside the substrate's response rather than replacing or rewriting any part of it, so a
substrate-reported path value always takes precedence over a SemSource-derived one.

#### Scenario: A non-community answer says so

- **GIVEN** a stack whose community index is empty
- **WHEN** an agent calls the corpus-wide thematic search tool and receives results
- **THEN** the result states that the answer is not community-backed, so the agent does not read
  semantic similarity hits as thematic community reasoning

#### Scenario: The substrate payload is not rewritten

- **WHEN** a tool adds its disclosure to a result
- **THEN** the substrate's own response is present unmodified beside it

### Requirement: Readiness distinguishes not-enabled from not-yet-built

Where a gated tool's capability is enabled but its backing index has not yet been populated, the
call SHALL report a retryable not-ready condition, distinct from the non-retryable "not enabled in
this configuration". The signal SHALL follow the existing readiness envelope's shape and spirit —
availability, readiness, state, and a reason carrying a retryable flag — so a caller can tell
retrying apart from reconfiguring.

#### Scenario: Enabled but still clustering

- **GIVEN** `enable_clustering` is true and the community index is empty
- **WHEN** an agent calls the community-scoped search tool
- **THEN** the reported condition is retryable and attributes the emptiness to clustering not having
  completed, not to configuration

#### Scenario: A caller can check before calling

- **WHEN** an agent reads the gateway's readiness surface
- **THEN** community-search availability appears as an explicit signal alongside the existing
  ingest-phase, structural-index, and semantic-index signals, so the agent can discover whether the
  capability is reachable without first calling a tool and interpreting the failure

### Requirement: Answer provenance is preserved

Where the substrate reports how an answer was produced — the synthesizing model, a degraded-
synthesis flag, the reason for degradation — the tool result SHALL preserve those fields. A
statistically derived community summary SHALL NOT be presented as an LLM-synthesized one.

#### Scenario: Degraded synthesis stays visible

- **GIVEN** a stack with clustering enabled but no LLM community-summary capability
- **WHEN** an agent calls a tool that returns a synthesized answer
- **THEN** the result carries the substrate's degraded flag and its reason, and attributes the answer
  to no model that did not produce it

### Requirement: Results are citable

A successful result SHALL carry the attribution the substrate provides: the entity identifier of
every returned entity, and — where the answer is community-backed — the community identifier
together with its level, since an identifier resolves only within its level. An agent SHALL be able
to follow any returned entity with a deterministic query.

#### Scenario: Community attribution survives the gateway

- **WHEN** a tool returns community-backed results
- **THEN** the result carries the community identifier and the level it belongs to

#### Scenario: Entity results are followable

- **WHEN** a tool returns entities
- **THEN** each carries its 6-part entity ID, so the agent can call a deterministic tool on that ID

### Requirement: The graph-query surface stays substrate-owned

SemSource SHALL NOT compute community membership, community summaries, or retrieval rankings
locally; its role is routing, gating, disclosure, and truthful reporting. Where the pinned semstreams
version does not offer a contract SemSource needs, the gap SHALL be recorded in
`docs/upstream/semstreams-asks.md` and raised as a semstreams issue, and the affected tool SHALL
state the limitation rather than substitute a locally computed value.

#### Scenario: A missing substrate signal is not reimplemented

- **WHEN** an availability, readiness, or retrieval-path signal a tool needs is absent from the
  pinned semstreams version
- **THEN** the gap is recorded as an upstream ask and the tool reports only what it can derive from
  the substrate's own response, with no locally computed substitute presented as a substrate signal
