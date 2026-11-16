## ADDED Requirements

### Requirement: A staleness sweep judges only the entities its own root produced

Every source that sweeps for staleness SHALL write an owner fact on each entity it publishes, naming the root or bucket that produced it, and a sweep carrying a liveness oracle SHALL judge only entities whose owner fact names that same root or bucket. A shared system slug SHALL NOT grant one root's sweep authority over another root's entities. A sweep request carrying a filesystem root or an absent set without an owner SHALL be refused. An entity with no owner fact SHALL NOT be marked or cleared by an oracle sweep.

#### Scenario: An AST sweep leaves a docs root's live document alone

- **GIVEN** an AST source at a root and a docs source at that root's `docs` directory, both configured with one project
- **WHEN** the AST source's periodic sweep runs while `docs/a.md` exists
- **THEN** the document and its passages carry no staleness marker

#### Scenario: Two docs roots under one project do not cross-mark

- **GIVEN** two docs roots configured with one project, one holding `a.md` and the other `b.md`
- **WHEN** each root's sweep runs while both files exist
- **THEN** neither document carries a staleness marker

#### Scenario: A sweep still marks its own missing artifact

- **GIVEN** an entity whose owner fact names a root
- **WHEN** that root's sweep runs after the entity's file was deleted
- **THEN** the entity is marked stale with the request's reason

#### Scenario: An oracle sweep without an owner is refused

- **WHEN** a lifecycle request carries a root path or an absent set and no owner
- **THEN** the pass refuses it and marks nothing
