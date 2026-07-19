# Delta: source-vocabulary-contract

## ADDED Requirements

### Requirement: Queryable entities carry a registered title predicate

Every entity type intended to be reachable by name SHALL carry the canonical title predicate
(`dc.terms.title`) stamped at ingest, registered through the canonical vocabulary — config and git
entity vocabularies gain title (and any demotion-marker) predicates as registered vocabulary, not
ad-hoc strings.

#### Scenario: Config dependency entity has a title

- **WHEN** the cfgfile source ingests a go.mod dependency
- **THEN** the resulting entity carries `dc.terms.title` with the dependency's name and is
  resolvable through the name index

#### Scenario: Markers are registered vocabulary

- **WHEN** a demotion or authority marker predicate is emitted by any source
- **THEN** that predicate is registered in the canonical vocabulary with its salience weight
