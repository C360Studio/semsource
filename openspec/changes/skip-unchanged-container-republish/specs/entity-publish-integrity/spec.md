## ADDED Requirements

### Requirement: A periodic reindex does not rewrite unchanged folder and repo entities
A periodic ast-source reindex SHALL NOT publish a `folder` or `repo` entity whose statements, apart from `dc.terms.created`, equal those this process last had admitted by the publisher for that ID. A publish the publisher refused MUST NOT count as admitted, so the next pass publishes it again. The initial seed and watch-event publishes are not gated.

#### Scenario: An idle reindex publishes no containers
- **GIVEN** ast-source has seeded `a/one.go` and `a/b/x.go`
- **WHEN** a periodic reindex runs and no file changed
- **THEN** no `folder` or `repo` entity is published

#### Scenario: A changed tree republishes the affected containers
- **GIVEN** ast-source has seeded `a/one.go` and `a/b/x.go`
- **WHEN** `a/b/y.go` is added and the next periodic reindex runs
- **THEN** folder `a/b`, folder `a`, and the repo are published with `a/b/y.go` in the set

#### Scenario: A refused container publish is retried
- **GIVEN** the publisher refused folder `a`'s publish on one reindex
- **WHEN** the next periodic reindex runs with no file changed
- **THEN** folder `a` is published
