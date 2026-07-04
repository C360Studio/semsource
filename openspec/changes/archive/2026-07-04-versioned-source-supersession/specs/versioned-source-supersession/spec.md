# versioned-source-supersession

## ADDED Requirements

### Requirement: Version and source identity on code entities
Code entities SHALL carry a version-independent source identity, and — when a version is configured —
the version, as triples, because neither is recoverable from the folded and length-capped `system`
segment of the entity ID.

#### Scenario: Entity indexed at a version
- **WHEN** a code entity is indexed from a source configured with project `semstreams` and version `v1.9.0`
- **THEN** the entity carries a source-identity triple with value `semstreams` and a version triple with value `v1.9.0`

#### Scenario: Entity indexed without a version (backward compatible)
- **WHEN** a code entity is indexed from a source with no version configured
- **THEN** the entity carries no version triple and its other triples are byte-identical to prior behavior

### Requirement: Deterministic cross-version correspondence
The system SHALL identify corresponding code entities across versions of the same source by matching
version-independent facts — source identity, path, name, type, and package — and MUST NOT rely on
parsing the entity ID.

#### Scenario: Same symbol across two versions corresponds
- **WHEN** function `Run` at `pkg/run.go` exists under source `semstreams` at both `v1.9.0` and `v1.10.0`
- **THEN** the two entities are recognized as corresponding (the same logical symbol across versions)

#### Scenario: Same path and name across different sources do not correspond
- **WHEN** two sources `alpha` and `beta` each have a function `Run` at `pkg/run.go`
- **THEN** the two entities are NOT corresponded, because grouping is scoped to a single source identity

### Requirement: Supersession edges
For corresponding entities ordered by version, the system SHALL emit a directional supersession
relationship in which the newer entity supersedes the older, additively and without removing any entity
or triple.

#### Scenario: Newer version supersedes older
- **WHEN** `Run` corresponds between `v1.9.0` and `v1.10.0`, and `v1.10.0` is the newer version
- **THEN** the newer entity carries a supersession relationship referencing the older entity ID, and the older carries the inverse relationship

#### Scenario: New symbol has no predecessor
- **WHEN** a symbol exists only in the newer version, with no corresponding entity in any older version
- **THEN** no supersession edge is emitted for it

#### Scenario: Re-running the pass is idempotent
- **WHEN** the supersession pass runs again over an unchanged graph
- **THEN** no duplicate edges are created and no existing entities or triples are removed

#### Scenario: Versions with no total order coexist without an edge
- **WHEN** two corresponding entities carry versions that are neither semver-comparable nor separable by index timestamp
- **THEN** no supersession edge is emitted and both entities remain queryable

### Requirement: Changed-versus-unchanged classification
A supersession relationship SHALL record whether the symbol changed, determined by comparing the
corresponding entities' verbatim-body hash (`code:<sha>`).

#### Scenario: Body differs marks the relationship changed
- **WHEN** corresponding entities across two versions have different body hashes
- **THEN** the supersession relationship is classified as changed

#### Scenario: Body identical marks the relationship unchanged
- **WHEN** corresponding entities across two versions have identical body hashes
- **THEN** the supersession relationship is classified as unchanged

### Requirement: Retention-safe supersession
The supersession pass SHALL NOT retract, delete, or overwrite any existing entity or triple; superseded
versions remain fully queryable.

#### Scenario: Superseded version survives the pass
- **WHEN** `v1.10.0` supersedes `v1.9.0`
- **THEN** the `v1.9.0` entities and all their triples remain present and queryable after the pass completes
