## MODIFIED Requirements

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

## ADDED Requirements
