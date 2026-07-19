# Delta: fusion-gateway

## ADDED Requirements

### Requirement: Config and git domains are reachable through the query surface

The query surface SHALL be able to answer questions about ingested config-domain entities (e.g.
declared dependency versions) and git-domain entities: their domains participate in an NL lens
scope or a dedicated tool, and their entities carry name-index-visible titles, so no ingested
domain is unreachable through every MCP tool.

#### Scenario: Dependency version is answerable

- **WHEN** an agent asks the MCP surface for the version of a dependency declared in the indexed
  repo's go.mod
- **THEN** a tool returns the declared version from the config-domain entity (not a miss, not a
  code-lens-only result set)

#### Scenario: No silently unreachable domains

- **WHEN** a source type publishes entities into the graph in a default deployment
- **THEN** at least one MCP tool can retrieve those entities, or the source's documentation states
  the gap explicitly
