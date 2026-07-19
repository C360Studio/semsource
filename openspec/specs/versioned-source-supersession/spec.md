# versioned-source-supersession Specification

## Purpose
Versioned source is **retained and related, never deleted** (ADR-0008). Each
version of a code symbol coexists as a distinct subgraph, carrying version + source
identity triples; the `supersession` component deterministically corresponds "the
same logical symbol across versions" (version-independent identity, tier-0, no
model) and emits directional supersession lineage edges with a changed/unchanged
body-hash marker. Historical (superseded) versions are demoted in ranking so the
current one surfaces first, while everything stays retained and queryable. All of it
is additive and retention-safe — reads via graph.query.prefix, appends lineage via
the entity-publish path, and never retracts or overwrites.

## Requirements
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

### Requirement: Historical version demotion
The current version of a symbol SHALL rank above its historical (superseded) versions in retrieval,
achieved by down-ranking entities that carry a supersession-superseded relationship. Demotion is a
bounded reordering only: it SHALL NOT remove, retract, or hide any historical entity or triple, which
remain fully queryable.

#### Scenario: Current version ranks above its historical version
- **WHEN** a symbol exists at `v1.9.0` and `v1.10.0` and `v1.10.0` supersedes `v1.9.0`
- **THEN** the `v1.10.0` entity (which carries no superseded relationship) ranks above the `v1.9.0`
  entity (which is superseded) for a query matching the symbol

#### Scenario: Demoted historical version stays retained and queryable
- **WHEN** the `v1.9.0` entity is demoted as historical
- **THEN** it and all its triples remain present and returnable by graph queries — the demotion lowers
  its rank without excluding it

#### Scenario: A version-independent entity is not demoted
- **WHEN** an entity carries no superseded relationship (it is the newest, or the only, version of its
  symbol)
- **THEN** it is not demoted and ranks at its normal salience


### Requirement: Version is declarable on every registration surface

Every source-registration surface SHALL accept an explicit version declaration (config file
entries, `add_source`, expanded repo/branch sources) (with a ref-derived default for
branch/tag-expanded sources where design so decides), so the existing version-triple emission —
gated on a non-empty version — is reachable by real deployments.

#### Scenario: add_source with a version

- **WHEN** a source is registered via `add_source` with a version argument and ingested
- **THEN** its code entities carry `code.artifact.project` and `code.artifact.version` triples

#### Scenario: End-to-end supersession reachability

- **WHEN** two versions of the same project are registered with distinct versions
- **THEN** the supersession pass produces lineage edges between corresponding entities and
  `code_changes` answers for that version pair

### Requirement: Non-semver ordering is restart-stable

Cross-version ordering for non-semver versions SHALL derive from a stable, source-anchored input
(never a predicate rewritten on restart or re-ingest), so supersession direction cannot invert or
become cyclic across restarts.

#### Scenario: Restart preserves lineage direction

- **WHEN** the stack restarts and re-ingests two non-semver versions
- **THEN** the supersedes/superseded_by direction between them is unchanged
