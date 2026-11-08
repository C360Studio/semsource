## ADDED Requirements

### Requirement: No replacement private recovery authority

The runtime SHALL contain no source lifecycle coordinator, journal, receipt store, seed/epoch seal,
effect fence, periodic replay, command stream or substitute recovery worker. Source publication SHALL
retain existing PubAck, loss accounting and owned Stop/drain guarantees without private receipt writes.
Configuration SHALL remain the source desired-state authority; ordinary domain staleness remains separate.

#### Scenario: Ordinary source publication

- **WHEN** source entities are published and the application stops
- **THEN** accepted publication work follows existing transport settlement and checked ownership contracts
- **AND** no private lifecycle receipt, manifest or recovery record is written

### Requirement: Legacy state is refused without interpretation

After the previous writer has stopped, startup SHALL perform a bounded existing-only lookup of
`SEMSOURCE_SOURCE_LIFECYCLE` before stream/config provisioning, producer construction or serving clients.
Only a classified missing bucket or an authoritative zero retained-message count SHALL admit startup.
Any other observation SHALL fail startup explicitly, without creating or modifying any storage.
The rule SHALL apply to the whole product bucket, regardless of claimed authority, phase or outcome.

#### Scenario: Missing or empty legacy bucket

- **WHEN** the existing-only lookup proves bucket absence or a fresh status reports zero retained messages
- **THEN** startup may proceed and the guard does not create the legacy bucket

#### Scenario: Retained legacy messages

- **WHEN** any message remains, including foreign, terminal-looking, tombstone or malformed data
- **THEN** startup refuses as incompatible and all retained bytes remain untouched
- **AND** no parser interprets the data as safe or resolved

#### Scenario: Unreadable or timed-out legacy storage

- **WHEN** lookup/status fails or its context expires
- **THEN** startup refuses and does not treat the error as absence

#### Scenario: Fresh deployment boundary

- **WHEN** an operator provisions separate fresh deployment storage with old writers stopped
- **THEN** the old data remains preserved and no automatic migration or resolution is claimed
- **AND** changing only a platform stem in the same nonempty account does not bypass refusal

### Requirement: Evidence remains attributable

Historical versioned corpus inputs and recorded outcomes SHALL remain unchanged. New changed-contract
checks SHALL be separately identified, with exact source/binary/config pins and checked process cleanup.

#### Scenario: Previously required positive source-removal proof

- **WHEN** historical evidence expected source-removal marking or positive reactivation
- **THEN** it remains historical evidence and is not rewritten as a passing unavailable-capability test
- **AND** the corrective contract's explicit deferral is recorded separately
