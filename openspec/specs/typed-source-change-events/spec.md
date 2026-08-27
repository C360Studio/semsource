# typed-source-change-events Specification

## Purpose
Typed source change events are the contract between a source handler's `Watch` channel and its
owning processor for deciding what may reach the graph on a file-system change. The doc and URL
handlers (`handler/doc`, `handler/url`) populate `handler.ChangeEvent.EntityStates` — fully-typed
entities carrying vocabulary-predicate triples — for every create/modify watch event, and their
processors' initial ingest pass (`doc-source`, `url-source`) calls the same `IngestEntityStates`
method rather than the package's older `RawEntity`-returning `Ingest`, which the ast, cfgfile,
image, video, and audio handlers still use for their own ingest. A delete event carries only the
changed path and `OperationDelete`, with `EntityStates` intentionally empty. A non-delete watch
event that reaches a doc-source or url-source processor without valid `EntityStates` is a contract
violation: the processor must refuse to publish and must surface the failure through its existing
error/health counters rather than accept the event or fall back to `RawEntity`. Watch and
periodic-reindex loops (ast-source's `handleWatchEvent`/`performFullIndex`, doc-source's watch
path) additionally treat a detected deletion or rename as a first-class event that triggers the
staleness-lifecycle pass (`graph.PublishLifecycleTrigger`) instead of one silently dropped.

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

### Requirement: Delete and rename events publish typed state changes

Watch and periodic-reindex paths SHALL treat file deletion and rename as first-class typed change
events that publish the staleness marker for affected entities — never silently discarded
(the audit found OpDelete events dropped on the floor).

#### Scenario: Delete event reaches the graph

- **WHEN** a watcher or reindex pass observes that a previously indexed file is gone
- **THEN** a typed change event publishes staleness markers for that file's entities within one
  index interval

### Requirement: Object-store changes publish typed entity state without a filesystem watcher

An object-store source SHALL populate canonical typed EntityStates for every object it ingests or
re-ingests, on the same contract the doc and URL handlers follow: no RawEntity fallback, no
dual-population, and a non-delete event lacking valid EntityStates is a contract violation that
publishes nothing and is reported through existing error and health evidence.

Change events here originate from comparing enumeration passes rather than from a filesystem
watcher, so the trigger differs while the published contract does not.

#### Scenario: An object is added or replaced

- **WHEN** an enumeration pass observes a new object, or an existing object with changed metadata
- **THEN** the source emits canonical typed EntityStates for it
- **AND** the event carries no RawEntity values

#### Scenario: A non-delete event without typed state

- **WHEN** an object-store change event for a new or replaced object carries no valid EntityStates
- **THEN** nothing is published for that object
- **AND** the contract failure is observable through existing health or metrics evidence

### Requirement: Object absence retracts only against a complete enumeration

An object-store source SHALL treat an object's absence as a deletion ONLY when observed in an
enumeration pass that completed successfully and covered the full configured prefix. A failed,
partial, or unauthorized listing MUST NOT be interpreted as absence, and MUST NOT publish staleness
or retraction for any entity.

Without this, one transient listing error retracts an entire corpus — the failure mode is total
rather than proportional, and it looks exactly like a legitimate empty bucket.

#### Scenario: An object is genuinely removed

- **WHEN** a completed enumeration pass covering the full prefix no longer contains a previously
  ingested object
- **THEN** a typed change event publishes staleness markers for that object's entities

#### Scenario: A listing fails partway through

- **WHEN** an enumeration pass fails after partial results
- **THEN** no staleness or retraction is published for any entity
- **AND** the failed pass is reported on the source's status surface

#### Scenario: Credentials stop working

- **WHEN** enumeration begins failing authentication against a previously readable bucket
- **THEN** no entity is retracted
- **AND** the source reports the failure rather than an empty corpus

#### Scenario: The prefix is legitimately emptied

- **WHEN** a completed pass over a reachable bucket returns no objects under the prefix, having
  previously returned many
- **THEN** staleness markers publish for the entities of every object that is gone
