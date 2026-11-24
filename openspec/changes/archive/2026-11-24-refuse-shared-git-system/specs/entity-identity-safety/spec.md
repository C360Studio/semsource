## MODIFIED Requirements

### Requirement: Sources that would share an entity-ID system are refused
SemSource SHALL refuse a source set in which two ast-source roots, two document roots (doc-source paths and objectstore-source buckets, which mint one `web` document ID space), or two media roots of one kind (image-, audio- or video-source paths; each kind mints its own entity types, so roots of different media kinds never collide), or two git-source repositories (each rooted at its `repo_path`, else its `repo_url`, under the branch-scoped repo slug its handler mints), would mint entity IDs under the same org and system, unless the collision includes a submodule pin and every root in it has the same parse scope (languages, excludes and vendored-JS indexing), and every non-pin root in it declares the pin's project and version and is the top level of a checkout whose HEAD is the pinned SHA. Two sources that build one component instance from different roots SHALL be refused whatever their systems, and a runtime add SHALL NOT re-enable a name that boot discovery owns from a different root. The refusal MUST happen before any colliding component publishes or any colliding configuration is written: at `semsource validate` and `semsource run` over the file's sources, at boot over the retained and discovered sets before discovery writes them, at a runtime `add_source` request, and at ast-source and doc-source component validation. The error SHALL name each colliding source and the shared system. Entity identity SHALL NOT change: the root is not added to the ID.

#### Scenario: Two ast sources under one project are refused
- **GIVEN** a config with ast sources at `./a` and `./b`, both with `project: svc` and no version
- **WHEN** `semsource validate` runs
- **THEN** it fails with an error naming both sources and system `svc`
- **AND** `semsource run` refuses to start with the same error

#### Scenario: Different versions of one project are admitted
- **GIVEN** ast sources at `./v1` and `./v2`, both with `project: lib`, with versions `1.0` and `2.0`
- **WHEN** the config is validated
- **THEN** no collision is reported

#### Scenario: A doc source with several roots under one project is refused
- **GIVEN** a docs source with paths `./a` and `./b` and `project: handbook`
- **WHEN** the config is validated
- **THEN** it fails with an error naming both roots and system `handbook`

#### Scenario: A runtime add that collides is refused and not persisted
- **GIVEN** a running deployment with an ast source under project `svc`
- **WHEN** an `add_source` request adds another ast source at a different path with `project: svc`
- **THEN** the reply carries error code `validation_failed` naming the existing instance
- **AND** the desired manifest is unchanged

#### Scenario: Submodule pins of one SHA are admitted
- **GIVEN** two parent repos that each link the same submodule at the same gitlink SHA
- **WHEN** boot discovery expands both into submodule pins
- **THEN** no collision is reported and both pins start

#### Scenario: A standalone source at a pin's identity shares the pin's IDs
- **GIVEN** a parent repo whose submodule is pinned at gitlink SHA `b1256521ee39`
- **AND** a standalone ast source for the same code with the pin's project and `version: b1256521ee39`
- **AND** the standalone's checkout HEAD is `b1256521ee39` and it indexes the pin's languages
- **WHEN** boot plans both
- **THEN** no collision is reported and both write the same entity IDs

#### Scenario: A standalone at a pin's identity that differs from the pin is refused
- **GIVEN** a submodule pin at gitlink SHA `b1256521ee39` indexing `go`
- **AND** a standalone ast source with the pin's project and `version: b1256521ee39` whose checkout HEAD is another commit, or which indexes other languages
- **WHEN** boot plans both
- **THEN** it fails with an error naming both sources and what differs

#### Scenario: A docs source and an object-store source under one project are refused
- **GIVEN** a docs source with `project: handbook` and an object-store source with `project: handbook`
- **WHEN** the config is validated
- **THEN** it fails with an error naming both sources and system `handbook`

#### Scenario: A discovered branch that collides with a retained runtime source is refused before it is written
- **GIVEN** a retained runtime-added ast source at `/retained/standalone` with `project: svc-feature`
- **AND** boot discovery finds a worktree at `/discovered/worktree` with `project: svc` on branch `feature`, building the same instance
- **WHEN** boot admits discovered sources
- **THEN** boot refuses, naming both roots
- **AND** no discovered configuration is written and the retained source is unchanged

#### Scenario: Two operator sources at one project and version still collide
- **GIVEN** ast sources at `./a` and `./b`, both with `project: lib` and `version: 1.0`, and no submodule pin
- **WHEN** the config is validated
- **THEN** it fails with an error naming both sources

#### Scenario: Same-SHA pins that index different languages are refused
- **GIVEN** two parent repos that link the same submodule at one gitlink SHA, one indexing `go` and the other `python`
- **WHEN** boot discovery expands both into submodule pins
- **THEN** boot refuses, naming both pins and their languages, before any discovered configuration is written

#### Scenario: A subdirectory of a checkout at the pinned SHA is not the pin's checkout
- **GIVEN** a submodule pin at gitlink SHA `b1256521ee39`
- **AND** a standalone ast source with the pin's project and version rooted at a subdirectory of a checkout whose HEAD is `b1256521ee39`
- **WHEN** boot plans both
- **THEN** it fails with an error naming both sources and saying the root is not a checkout's top level

#### Scenario: A runtime add cannot take over a discovered name from another root
- **GIVEN** boot discovery owns `ast-source-svc-feature` from `/discovered/worktree`, and an operator has removed it
- **WHEN** an `add_source` request would build `ast-source-svc-feature` from `/retained/standalone`
- **THEN** the reply carries error code `validation_failed` naming the discovered root
- **AND** the desired manifest is unchanged

#### Scenario: Two object-store sources that build one instance from different roots are refused
- **GIVEN** an object-store source for bucket `b-x` and another for bucket `b` with prefix `x`
- **WHEN** the config is validated
- **THEN** it fails with an error naming both roots and instance `objectstore-source-b-x`

#### Scenario: Two discovered pins that build one instance from different parents are refused before they merge
- **GIVEN** two local git parents at different paths with one project, each linking the same submodule at one gitlink SHA, one indexing `go` and the other `python`
- **WHEN** boot discovery plans both pins
- **THEN** boot refuses, naming both pin roots, before any discovered configuration is written

#### Scenario: A pin member with a different parse scope is refused
- **GIVEN** a submodule pin and a standalone source at its identity, at the pinned HEAD with the pin's languages
- **AND** the standalone sets `index_vendored_js: true` while the pin does not
- **WHEN** boot plans both
- **THEN** it fails with an error naming both sources and the differing option

#### Scenario: Two image sources that reach one root are refused
- **GIVEN** an image source with paths `/m/x` and `/m/b`, and another image source with path `/m/b`
- **WHEN** the config is validated
- **THEN** it fails with an error naming both sources and system `b`

#### Scenario: An image and an audio source over one root are admitted
- **GIVEN** an image source and an audio source, both with path `/m/b`
- **WHEN** the config is validated
- **THEN** no collision is reported

#### Scenario: Two git repositories with one directory name are refused
- **GIVEN** git sources at `/a/core` and `/b/core`, with no URL and no branch slug
- **WHEN** `semsource validate` runs
- **THEN** it fails with an error naming both sources and system `core`
- **AND** the error tells the operator to give each a distinct repo URL or directory name
- **AND** `semsource run` refuses to start with the same error

#### Scenario: One repository on two branch slugs is admitted
- **GIVEN** git sources at `/r/core` with branch slugs `main` and `feature`
- **WHEN** the config is validated
- **THEN** no collision is reported

