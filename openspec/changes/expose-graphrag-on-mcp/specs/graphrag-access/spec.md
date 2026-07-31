## Purpose

Make the substrate's GraphRAG surface — community-scoped search, corpus-wide thematic search,
graph summary, and fallback graph search — reachable by an agent over MCP. Availability,
readiness, and answer provenance are stated truthfully at every tier, so a refused or degraded
GraphRAG answer is never indistinguishable from a graph that has nothing to say.

## ADDED Requirements

### Requirement: GraphRAG is reachable over MCP

SemSource SHALL expose the substrate's GraphRAG query surface through the MCP gateway, routing to
the existing semstreams subjects `graph.query.localSearch`, `graph.query.globalSearch`,
`graph.query.summary`, and `graph.query.searchGraph`. Each of those capabilities SHALL be
reachable through a tool the MCP roster advertises. Tool count, names, and argument grouping (one
tool with a mode argument versus separate tools) are fixed by design, not by this spec.

#### Scenario: The roster covers the GraphRAG surface

- **WHEN** an MCP client lists the gateway's tools
- **THEN** community-scoped search, corpus-wide thematic search, graph summary, and fallback graph
  search are each reachable through an advertised tool

#### Scenario: A call reaches the substrate handler

- **GIVEN** a stack whose community index is populated
- **WHEN** an agent calls the community-scoped search tool with an entity ID and a query
- **THEN** the substrate's `graph.query.localSearch` handler serves the request and its response is
  what the tool reports

### Requirement: An unavailable capability is stated, never simulated

GraphRAG requires clustering to be enabled (`enable_clustering`, plus `clustering_llm` for
LLM-synthesized community summaries). When the running stack does not provide the capability a
tool needs, that tool call SHALL fail with `isError: true` and a message naming the missing
capability and the configuration that enables it. It SHALL NOT return an empty success, a
zero-count result, or a bare transport error — an agent cannot distinguish any of those from
"the graph has nothing to say".

#### Scenario: A call on a stack without clustering is refused truthfully

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls a GraphRAG tool
- **THEN** the result is `isError: true` and states that GraphRAG is not enabled in this
  configuration

#### Scenario: A missing responder is not reported as an empty result

- **GIVEN** the routed subject has no subscribed handler, because the substrate registers
  community-scoped handlers only once the community index is available
- **WHEN** an agent calls the affected tool
- **THEN** the result names the unavailable capability, never a successful result with zero
  entities and never an unexplained timeout

### Requirement: Readiness distinguishes not-enabled from not-yet-built

Where GraphRAG is enabled but the community index has not yet been populated, a tool call SHALL
report a retryable not-ready condition, distinct from the non-retryable "not enabled in this
configuration". The signal SHALL follow the existing readiness envelope's shape and spirit —
availability, readiness, state, and a reason carrying a retryable flag — so a caller can tell
retrying apart from reconfiguring.

#### Scenario: Enabled but still clustering

- **GIVEN** `enable_clustering` is true and the community index is empty
- **WHEN** an agent calls a GraphRAG tool
- **THEN** the reported condition is retryable and attributes the emptiness to clustering not
  having completed, not to tier configuration

#### Scenario: A caller can check before calling

- **WHEN** an agent reads the gateway's readiness surface
- **THEN** GraphRAG availability appears as an explicit signal alongside the existing ingest-phase,
  structural-index, and semantic-index signals, so the agent can discover whether GraphRAG is
  reachable without first calling a tool and interpreting the failure

### Requirement: Answer provenance is preserved

Where the substrate reports how an answer was produced — the synthesizing model, a degraded-
synthesis flag, the reason for degradation — a GraphRAG tool result SHALL preserve those fields. A
statistically derived community summary SHALL NOT be presented as an LLM-synthesized one.

#### Scenario: Degraded synthesis stays visible

- **GIVEN** a stack with clustering enabled but no LLM community-summary capability
- **WHEN** an agent calls a GraphRAG tool that returns a synthesized answer
- **THEN** the result carries the substrate's degraded flag and its reason, and attributes the
  answer to no model that did not produce it

### Requirement: Results are citable

A successful GraphRAG tool result SHALL carry the attribution the substrate provides: the
community identifier together with its level — an identifier resolves only within its level — and
the entity identifier of every returned entity, so an agent can cite evidence and follow up with a
deterministic query.

#### Scenario: Community attribution survives the gateway

- **WHEN** a GraphRAG tool returns community-backed results
- **THEN** the result carries the community identifier and the level it belongs to

#### Scenario: Entity results are followable

- **WHEN** a GraphRAG tool returns entities
- **THEN** each carries its 6-part entity ID, so the agent can call a deterministic tool on that ID

### Requirement: GraphRAG stays substrate-owned

SemSource SHALL NOT compute community membership, community summaries, or GraphRAG rankings
locally; its role is routing, gating, and truthful reporting. Where the pinned semstreams version
does not offer a contract SemSource needs, the gap SHALL be recorded in
`docs/upstream/semstreams-asks.md` and raised as a semstreams issue, and the affected tool SHALL
state the limitation rather than substitute a locally computed value.

#### Scenario: A missing substrate signal is not reimplemented

- **WHEN** an availability or readiness signal a tool needs is absent from the pinned semstreams
  version
- **THEN** the gap is recorded as an upstream ask and the tool reports the limitation, with no
  locally computed substitute presented as a substrate signal
