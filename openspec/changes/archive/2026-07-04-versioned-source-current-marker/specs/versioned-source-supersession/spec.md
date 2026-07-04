# versioned-source-supersession

## ADDED Requirements

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
