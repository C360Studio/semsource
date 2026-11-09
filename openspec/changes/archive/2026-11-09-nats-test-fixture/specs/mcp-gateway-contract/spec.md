## ADDED Requirements

### Requirement: add_source is a namespaced NATS ingest request

The MCP `add_source` tool SHALL send the requested source as an add request on `graph.ingest.add.{namespace}`, using
the gateway's configured namespace. It SHALL return the reply, including the spawned component handles, to the caller
as a successful tool result.

#### Scenario: A git source is added

- **GIVEN** a gateway configured with namespace `acme` and a responder on `graph.ingest.add.acme`
- **WHEN** a client calls `add_source` with a git URL and branch
- **THEN** the responder receives an add request, and the tool result is not an error and names the spawned component
