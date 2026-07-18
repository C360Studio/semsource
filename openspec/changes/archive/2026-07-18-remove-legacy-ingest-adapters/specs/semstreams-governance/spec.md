## MODIFIED Requirements

### Requirement: Standalone mode boots ownership before graph-ingest

The SemSource external service MUST create the ownership substrate and bind SemSource projection
contracts before graph-ingest starts. This behavior is intrinsic to the sole runtime and MUST NOT be
selected through a compatibility mode field or environment variable.

#### Scenario: Graph-ingest sees OWNER_CLAIMS on startup

**GIVEN** SemSource starts as an external service
**WHEN** graph-ingest starts
**THEN** OWNER_CLAIMS and OWNER_PRESENCE already exist
**AND** SemSource projection contracts have been registered

## REMOVED Requirements

### Requirement: Headless mode declares host-owned governance

**Reason**: Embedded headless operation has already been removed; retaining a headless governance
requirement conflicts with the one external-service runtime.

**Migration**: Remove `mode` from configuration and deployment inputs. The external SemSource service
continues to bootstrap its governed graph before graph-ingest starts.
