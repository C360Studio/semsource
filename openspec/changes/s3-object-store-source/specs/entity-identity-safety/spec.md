# entity-identity-safety Specification

## ADDED Requirements

### Requirement: Object-derived entity identity comes from the object key, never from a local path

An entity produced from an object store SHALL derive its identity from the object's key within the
bucket, and MUST NOT incorporate any local filesystem path used to fetch, cache, or materialize that
object. The bucket identifier SHALL supply the system segment, subject to the same normalization
every other system slug receives, and the existing explicit-project override SHALL take precedence
when configured.

This is the first identity source in SemSource where a purely local, non-intrinsic value —
a cache or temp path — is available at construction time and would produce a stable-looking ID that
silently changes between hosts, runs, or cache eviction. The requirement is therefore stated
directly rather than left implied by the general determinism contract.

#### Scenario: The same object fetched through different local locations

- **WHEN** the same bucket object is ingested on two hosts whose local cache or temp directories
  differ
- **THEN** both produce byte-identical entity IDs

#### Scenario: Re-ingest after a restart

- **WHEN** a service restarts and re-seeds a previously ingested prefix
- **THEN** every object produces the entity ID it produced before the restart
- **AND** the seed is re-derivable from the bucket alone

#### Scenario: Object keys that are not legal ID segments

- **WHEN** an object key contains `/`, spaces, or bytes outside the ID alphabet
- **THEN** the produced segments satisfy the graph-ingest per-segment contract
- **AND** they pass semstreams `ValidateEntityID`

#### Scenario: Keys that differ only past the truncation boundary

- **WHEN** two objects have keys sharing a long common prefix and differing only beyond the point
  where segment truncation applies
- **THEN** their entity IDs differ

#### Scenario: Content changes do not change identity

- **WHEN** an object at a given key is replaced with different content
- **THEN** its entity ID is unchanged
- **AND** the new content hash travels as a triple rather than as part of the ID
