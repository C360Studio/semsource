# graphrag-access Specification

## Purpose
Make the substrate's corpus-wide thematic search reachable by an agent over MCP, at every tier.
What an answer contains escalates with what the stack provides (entity hits, then community
summaries, then an LLM-synthesized answer), and every result discloses which rung it reached, so a
thinner answer is never mistaken for a richer one.
## Requirements
### Requirement: Corpus-wide thematic search is reachable over MCP

SemSource SHALL expose the substrate's corpus-wide thematic search through the MCP gateway, routing
to the existing semstreams subject `graph.query.searchGraph`, which delegates to
`graph.query.globalSearch` and returns its response unchanged when non-empty. Tool names and argument
shapes are fixed by design.

A tool SHALL NOT be routed to a subject that more than one live handler answers. Where SemSource and
the substrate both subscribe to a subject, the reply is a race between two payload shapes, and a tool
built on it would return a nondeterministic result. A subject SHALL be treated as eligible only once
SemSource has vacated any competing claim on it.

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
fields the substrate returns, and SHALL NOT contradict a value the substrate reported.

The result SHALL be a bounded ranked list, not the substrate's payload in full. A search verb ranks
so the caller can follow up; transferring every matched entity's triples costs an order of magnitude
more than the deterministic tools and carries no readable content, since bodies are held by
reference. SemSource SHALL preserve the substrate's own values — entity IDs, community attribution,
answer text, model attribution, degradation — and SHALL invent none. Where the list is capped, the
result SHALL report the true total and that it was truncated.

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

#### Scenario: The result is bounded and honest about it

- **WHEN** a thematic search matches more entities than the result cap
- **THEN** the result carries the capped list, the true total, and a truncation flag

#### Scenario: Substrate values are preserved, not restated

- **WHEN** the substrate reports an answer, a model attribution, or community summaries
- **THEN** those values appear in the result as the substrate gave them

#### Scenario: A ranked list survives a substrate response carrying no digests

- **GIVEN** the substrate returns matched entities without its own digest list
- **WHEN** an agent calls the thematic search tool
- **THEN** the result still carries a ranked match list derived from those entities, never an empty
  one

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
does not offer a contract SemSource needs, the gap SHALL be raised as a semstreams issue,
and the affected tool SHALL
state only what it can derive from the substrate's own response.

#### Scenario: A missing substrate signal is not reimplemented

- **WHEN** a retrieval-path signal a tool needs is absent from the pinned semstreams version
- **THEN** the gap is recorded as an upstream ask and the tool derives its disclosure from the
  substrate's returned fields, with no locally computed substitute presented as a substrate signal

### Requirement: Matches carry bounded value properties when the substrate supplies triples

A `graph_search` match SHALL include a bounded `properties` map of the entity's own value-bearing facts — populated exclusively from an explicit allowlist of property predicates — whenever the substrate response already carries that entity's triples. Property values SHALL be the substrate's own triple objects, never derived or invented. The map SHALL be capped in entry count per match and in bytes per value, and a response path that carries no triples (digests, bare entity IDs) SHALL render matches without properties rather than fetching them — no additional substrate round-trips are permitted on behalf of match rendering.

#### Scenario: Config dependency values are answerable in one call

- **WHEN** `graph_search` matches a config dependency entity on a response path that carries the entity's triples
- **THEN** the match's `properties` include the allowlisted config values present on the entity (such as dependency version and kind), so a value question is answerable without a follow-up call

#### Scenario: Absence stays absence on triple-less paths

- **WHEN** the substrate response carries digests or bare entity IDs instead of entity triples
- **THEN** matches render without a `properties` map, and no substrate query is issued to fill it

#### Scenario: Rendering stays bounded

- **WHEN** a matched entity carries more allowlisted properties than the per-match cap, or a property value longer than the per-value byte cap
- **THEN** the rendered map is truncated to the caps deterministically, and the match otherwise renders normally

#### Scenario: Non-allowlisted predicates never render

- **WHEN** a matched entity carries triples whose predicates are not on the allowlist (relationship edges, bodies, timestamps)
- **THEN** none of them appear in `properties`, regardless of remaining cap headroom
