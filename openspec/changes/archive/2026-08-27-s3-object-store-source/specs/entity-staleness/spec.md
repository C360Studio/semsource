# entity-staleness Specification

## ADDED Requirements

### Requirement: The staleness pass accepts liveness evidence from sources whose artifacts are not files

The staleness lifecycle pass SHALL accept an explicit set of absent artifact paths from the source
that enumerated them, as an alternative to checking a filesystem root. A source whose artifacts are
not files — an object store answers only when listed — has no path to stat, and its own completed
enumeration is the authoritative liveness evidence.

A request carrying an absent set MUST NOT also carry a filesystem root: two liveness oracles that
disagree resolve silently, and the visible symptom is entities that will not retract or will not
return.

Every path the request does NOT name MUST be treated as present, so an artifact that reappears
clears its marker on the same pass that observes it rather than waiting for a re-seed.

An empty absent set is a valid assertion — the enumeration completed and nothing is gone — and MUST
be distinguishable, over the wire as well as in memory, from a request making no liveness claim at
all. The two lead to opposite outcomes: the first marks nothing, the second marks every entity in
scope.

#### Scenario: A source states which artifacts are gone

- **WHEN** a source triggers the staleness pass with the set of paths its completed enumeration
  found absent
- **THEN** every in-scope entity carrying one of those paths is marked stale with the given reason
- **AND** entities carrying any other path are left alone

#### Scenario: A document's passages are marked with it

- **WHEN** the absent set names the path of a document that has passage entities
- **THEN** the document entity and all of its passage entities are marked stale together

#### Scenario: An artifact reappears

- **WHEN** a previously marked entity's path is absent from the set a later completed pass supplies
- **THEN** its staleness marker is cleared

#### Scenario: An empty absent set is not a source removal

- **WHEN** a source supplies an absent set containing no paths
- **THEN** no entity is marked stale
- **AND** the pass is not treated as the removal of the source itself

#### Scenario: Two liveness oracles in one request

- **WHEN** a request carries both a filesystem root and an absent set
- **THEN** the request is rejected rather than resolved by precedence
