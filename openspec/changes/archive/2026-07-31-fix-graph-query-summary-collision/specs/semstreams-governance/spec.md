## ADDED Requirements

### Requirement: A request subject has exactly one handler

SemSource MUST NOT subscribe a component to a request/reply subject that the pinned SemStreams
target also serves. Where both subscribe, the requester receives whichever handler replies first and
discards the other; when the two return different payload shapes, the result is a nondeterministic
contract that no caller can rely on and no shape assertion can pin.

SemSource MAY continue to serve SemSource-specific concepts on subjects inside the
`graph.query.*` namespace where the substrate defines no such subject, but the burden is on
SemSource to keep them disjoint from the substrate's set at the pinned target.

#### Scenario: An overlapping subscription fails the guard

- **GIVEN** a SemSource component subscribes to a request subject the pinned SemStreams target also
  serves
- **WHEN** the subject-ownership guard runs
- **THEN** it fails, naming the contested subject and both claimants

#### Scenario: A SemSource-specific subject is permitted

- **GIVEN** a subject inside `graph.query.*` that names a SemSource concept the substrate does not
  define
- **WHEN** the guard runs
- **THEN** it passes, because no substrate handler answers that subject

#### Scenario: The source summary is served on a SemSource subject

- **GIVEN** SemSource has ingested source entities
- **WHEN** a consumer requests `graph.query.sourceSummary`
- **THEN** the response is the source-manifest summary — namespace, phase, entity-ID format, domain
  counts, and predicate schema — and no substrate handler competes to answer it

## MODIFIED Requirements

### Requirement: Current query surfaces remain available

SemSource standalone graph mode MUST expose current SemStreams graph query
surfaces needed by SemOps and other consumers. A subject the substrate owns MUST return the
substrate's contract; SemSource's own summary is a distinct surface on a distinct subject, and the
two MUST NOT be conflated.

#### Scenario: Consumer discovers source entities by prefix

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.prefix` with a SemSource prefix
**THEN** the response uses the typed paginated prefix contract

#### Scenario: Consumer asks for graph summary

**GIVEN** SemSource has ingested source entities
**WHEN** a consumer requests `graph.query.summary`
**THEN** the response includes entity type examples and predicate summary data
**AND** it is the substrate's handler that answered, deterministically, with no SemSource handler
competing
