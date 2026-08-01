## MODIFIED Requirements

### Requirement: Corpus-wide thematic search is reachable over MCP

SemSource SHALL expose the substrate's corpus-wide thematic search through the MCP gateway, routing
to the existing semstreams subject `graph.query.searchGraph`, which delegates to
`graph.query.globalSearch` and returns its response unchanged when non-empty. SemSource SHALL also
expose the substrate's graph overview, routing to `graph.query.summary`. Tool names and argument
shapes are fixed by design.

A tool SHALL NOT be routed to a subject that more than one live handler answers. Where SemSource and
the substrate both subscribe to a subject, the reply is a race between two payload shapes, and a tool
built on it would return a nondeterministic result. A subject SHALL be treated as eligible only once
SemSource has vacated any competing claim on it.

#### Scenario: The roster covers thematic search

- **WHEN** an MCP client lists the gateway's tools
- **THEN** corpus-wide thematic search is reachable through an advertised tool

#### Scenario: The roster covers the graph overview

- **WHEN** an MCP client lists the gateway's tools
- **THEN** the substrate's graph overview — entity-type counts with examples and the predicate
  schema — is reachable through an advertised tool

#### Scenario: A contested subject is not routed to

- **GIVEN** a subject answered by both a SemSource component and a substrate component in the same
  deployment
- **WHEN** a graph-query tool is designed
- **THEN** that subject is not used, and the collision is recorded as a defect to resolve

#### Scenario: A call reaches the substrate handler

- **WHEN** an agent calls the thematic search tool with a query
- **THEN** the substrate's `graph.query.searchGraph` handler serves the request and its response is
  what the tool reports

## ADDED Requirements

### Requirement: The graph overview is a passthrough

The graph-overview tool SHALL return the substrate's response unmodified. It has no retrieval rung
to disclose — the routed subject is a discovery resolver that does not involve clustering, retrieval
strategy, or answer synthesis — so it SHALL carry no retrieval disclosure, whose presence would imply
a choice of path that was never made.

#### Scenario: The overview result is the substrate response

- **WHEN** an agent calls the graph-overview tool
- **THEN** the result is exactly the substrate's payload, with no added disclosure block

#### Scenario: The overview answers at every tier

- **GIVEN** a stack running with `enable_clustering` false
- **WHEN** an agent calls the graph-overview tool
- **THEN** the call succeeds, because the routed subject does not depend on clustering
