# entity-publish-integrity

## MODIFIED Requirements

### Requirement: Reconcile is enabled only for handlers proven to publish complete sets
A publisher SHALL reconcile only when its handler opts in, and a handler MUST opt in only after a test proves that each of its publishes carries the entity's complete `semsource` statement set for group `source` (one writer per entity ID). The supersession publisher SHALL NOT reconcile. A publish that may share its entity ID with another writer (#348, #349) or that omits a statement after a store fault SHALL be marked `SkipReconcile`.

#### Scenario: A handler without the completeness test does not reconcile
- **GIVEN** a handler whose publisher was built without the reconcile option
- **WHEN** it republishes an entity with fewer predicates
- **THEN** no reconcile request is sent for that entity

#### Scenario: Watch-path folder republish is not reconciled
- **GIVEN** a reconcile-enabled ast-source whose root shares its system with another root (#349)
- **WHEN** a file is created or deleted under that root
- **THEN** no reconcile request is sent for any `folder` or `repo` entity

#### Scenario: A shared or faulted publish is not reconciled
- **GIVEN** a reconcile-enabled handler
- **WHEN** it publishes an entity whose ID another root or manifest can also write, or whose store write failed
- **THEN** no reconcile request is sent for that entity

## ADDED Requirements

### Requirement: AST folder and repo publishes carry their complete contains set
Every ast-source publish of a `folder` or `repo` entity from a root that does not share its system with another root SHALL carry that entity's complete `code.structure.contains` set, built from every file the watch path has indexed. A root that shares its system (#349) SHALL NOT publish a container from a watch event. A watch event MUST NOT publish a folder from the event's file alone. A watch event that adds or removes an indexed path SHALL republish the repo and each ancestor folder of that path, and an ancestor left with no indexed file SHALL be published with no `contains`. A watch modify of an already indexed path SHALL NOT republish any folder.

#### Scenario: A watch edit keeps the folder's other children
- **GIVEN** ast-source has indexed `a/one.go`, `a/b/x.go` and `a/c/y.go`
- **WHEN** `a/b/x.go` is modified and the watcher delivers the event
- **THEN** folder `a` in `ENTITY_STATES` still contains `a/b`, `a/c` and `a/one.go`

#### Scenario: A watch create adds the new file to its folder
- **GIVEN** ast-source has indexed `a/one.go`, `a/b/x.go` and `a/c/y.go`
- **WHEN** `a/new.go` is created and the watcher delivers the event
- **THEN** folder `a` contains `a/new.go` as well as its earlier children

#### Scenario: A watch delete removes the file and clears an emptied folder
- **GIVEN** ast-source has indexed `a/one.go`, `a/b/x.go` and `a/c/y.go`
- **WHEN** `a/c/y.go` is deleted and the watcher delivers the event
- **THEN** folder `a` no longer contains `a/c`
- **AND** folder `a/c` carries no `code.structure.contains`

#### Scenario: A deleted directory's folder is cleared
- **GIVEN** ast-source has indexed `a/one.go` and `a/b/x.go`
- **WHEN** directory `a/b` is removed and the watcher delivers the event
- **THEN** folder `a` no longer contains `a/b`
- **AND** folder `a/b` carries no `code.structure.contains`
