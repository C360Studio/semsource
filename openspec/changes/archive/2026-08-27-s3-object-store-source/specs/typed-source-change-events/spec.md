# typed-source-change-events Specification

## ADDED Requirements

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
