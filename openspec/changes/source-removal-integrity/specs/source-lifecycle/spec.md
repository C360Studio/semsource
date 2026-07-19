# Delta: source-lifecycle

## ADDED Requirements

### Requirement: Removal is real and observable

`remove_source` (and the underlying NATS remove request) SHALL stop the named source component,
remove it from the source manifest, and deregister it from status aggregation; the removal SHALL
be observable via `source_status` within one documented aggregation interval. Ingestion from the
removed source SHALL cease.

#### Scenario: Removed source leaves status

- **WHEN** a registered source is removed by its handle
- **THEN** within the documented interval it no longer appears in `source_status` sources and no
  further entities from it are published

#### Scenario: Unknown handle fails loudly

- **WHEN** `remove_source` is called with a handle that matches no registered source
- **THEN** the reply is the reserved `NOT_FOUND` error, not `removed: true`

#### Scenario: Expanded-repo removal is instance-scoped

- **WHEN** one instance of an expanded repo source (e.g. its git instance) is removed
- **THEN** exactly that instance leaves the manifest and status while sibling instances remain
  listed and ingesting

### Requirement: The manifest mirrors the running set

At all times, the set of sources reported by the manifest and status surfaces SHALL equal the set
of actually-running source components — additions appear promptly (existing behavior) and removals
disappear promptly (this change); no phantom entries in either direction.

#### Scenario: Add/remove sequence converges

- **WHEN** any sequence of add_source and remove_source operations completes
- **THEN** the manifest and status source lists equal the running component set within one
  aggregation interval
