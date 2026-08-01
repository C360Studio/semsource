## Purpose

Make the substrate's corpus-wide thematic search reachable by an agent over MCP, at every tier.
What an answer contains escalates with what the stack provides (entity hits, then community
summaries, then an LLM-synthesized answer), and every result discloses which rung it reached, so a
thinner answer is never mistaken for a richer one.

## ADDED Requirements

### Requirement: Corpus-wide thematic search is reachable over MCP

SemSource SHALL expose the substrate's corpus-wide thematic search through the MCP gateway, routing
to the existing semstreams subject `graph.query.searchGraph`, which delegates to
`graph.query.globalSearch` and returns its response unchanged when non-empty. Tool names and
argument shapes are fixed by design.

A tool SHALL NOT be routed to a subject that more than one live handler answers. Where SemSource and
the substrate both subscribe to a subject, the reply is a race between two payload shapes, and a tool
built on it would return a nondeterministic result.

#### Scenario: The roster covers thematic search

- **WHEN** an MCP client lists the gateway's tools
- **THEN** corpus-wide thematic search is reachable through an advertised tool

#### Scenario: A contested subject is not routed to

- **GIVEN** a subject answered by both a SemSource component and a substrate component in the same
  deployment
- **WHEN** a graph-query tool is designed
- **THEN** that subject is not used, and the collision is recorded as a defect to resolve

#### Scenario: A call reaches the substrate handler

- **WHEN** an agent calls the thematic search tool with a query
- **THEN** the substrate's `graph.query.searchGraph` handler serves the request and its response is
  what the tool reports

### Requirement: The surface is available at every tier

No tool on this surface SHALL be gated on clustering. `graph.query.searchGraph` answers through
non-community retrieval paths when no community index exists. Capability differences between tiers
SHALL be disclosed in the result, never expressed by withholding the tool.

#### Scenario: Thematic search answers without clustering

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls the thematic search tool
- **THEN** the call succeeds through a non-community retrieval path rather than being refused

### Requirement: An answer discloses how far it escalated

A thematic search result SHALL disclose which rung of the capability ladder produced it: entity hits
only, community-backed summaries, or an LLM-synthesized answer. The disclosure SHALL be derived from
fields the substrate returns and SHALL sit alongside the substrate's response rather than replacing
or rewriting any part of it, so a substrate-reported value always takes precedence over a
SemSource-derived one.

#### Scenario: A non-community answer says so

- **GIVEN** a stack whose community index is empty
- **WHEN** an agent calls the thematic search tool and receives results
- **THEN** the result states that the answer is not community-backed, so the agent does not read
  semantic similarity hits as thematic community reasoning

#### Scenario: A community-backed answer says so

- **GIVEN** a stack with clustering enabled and a populated community index
- **WHEN** an agent calls the thematic search tool
- **THEN** the result states that the answer is community-backed and carries the community summaries
  the substrate returned

#### Scenario: The substrate payload is not rewritten

- **WHEN** a tool adds its disclosure to a result
- **THEN** the substrate's own response is present unmodified beside it

### Requirement: A template answer is never presented as an LLM answer

Where the substrate returns a synthesized answer, the result SHALL distinguish an LLM-synthesized
answer from the template floor. The presence of a model attribution is the discriminator; the
substrate's degraded flag SHALL NOT be treated as one, because a deployment with no LLM configured
returns a template answer with that flag false by design. The substrate's degraded flag and reason
SHALL still be preserved, since they carry the distinct meaning that an LLM-configured deployment
fell back.

#### Scenario: A template answer on a stack without an LLM

- **GIVEN** a stack with clustering enabled and no LLM community-summary capability
- **WHEN** an agent calls the thematic search tool and receives a synthesized answer
- **THEN** the result attributes the answer to no model, and does not present it as
  LLM-synthesized even though the substrate's degraded flag is false

#### Scenario: An LLM-configured deployment that fell back

- **GIVEN** a stack with an LLM configured whose synthesis failed or timed out
- **WHEN** an agent calls the thematic search tool
- **THEN** the result carries the substrate's degraded flag and its reason, so the caller can tell
  that per-query synthesis was expected and not delivered

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
locally; its role is routing, disclosure, and truthful reporting. Where the pinned semstreams version
does not offer a contract SemSource needs, the gap SHALL be recorded in
`docs/upstream/semstreams-asks.md` and raised as a semstreams issue, and the affected tool SHALL
state only what it can derive from the substrate's own response.

#### Scenario: A missing substrate signal is not reimplemented

- **WHEN** a retrieval-path signal a tool needs is absent from the pinned semstreams version
- **THEN** the gap is recorded as an upstream ask and the tool derives its disclosure from the
  substrate's returned fields, with no locally computed substitute presented as a substrate signal
