# typed-source-change-events Specification

## Purpose
TBD - created by archiving change remove-legacy-ingest-adapters. Update Purpose after archive.
## Requirements
### Requirement: Doc and URL watch changes publish typed entity state

Doc and URL handlers used by their source processors MUST emit canonical EntityStates for create and
modify watch events. Delete events MUST carry the changed path and delete operation and MAY omit
EntityState. These watch paths MUST NOT use RawEntity fallback events or dual-populate
`ChangeEvent.Entities`. RawEntity use by other active handlers and synchronous ingest is unaffected.

#### Scenario: A document or URL is created or modified

- **WHEN** a watched document or URL produces changed content
- **THEN** its handler emits canonical typed EntityStates
- **AND** the processor publishes them without inspecting RawEntity fallback data
- **AND** the event contains no RawEntity values

#### Scenario: A watched path is deleted

- **WHEN** a document delete is observed
- **THEN** the event carries the path and delete operation
- **AND** typed or raw entity content is not required

### Requirement: Missing typed state is an observable contract error

A non-delete doc or URL event without valid EntityState MUST publish nothing and MUST report the
contract failure through existing error/health evidence. The processor MUST NOT normalize RawEntity,
silently accept the event, or use a mixed-version fallback.

#### Scenario: Non-delete event has no typed state

- **WHEN** a doc or URL processor receives create or modify without EntityStates
- **THEN** it records a bounded contract error and publishes no entity
- **AND** the failure is observable through existing health or metrics evidence
