# git-submodule-ingestion Delta

## Purpose

Repos that link git submodules ingest completely (submodule working trees are
materialized on clone and kept in sync on pull), loudly (an unmaterialized
submodule tree is surfaced on source status, never silently absent), and with
sane identity (entities from a submodule carry the submodule's own project
scope and gitlink-SHA version, so a shared submodule dedups across consumer
repos instead of forking under each parent's identity).

## ADDED Requirements

### Requirement: Remote clones and pulls materialize submodules

When SemSource clones a remote repository source, submodule working trees
declared by the repository SHALL be materialized (recursively, shallow where
possible) at the commits its gitlinks pin. When SemSource updates a previously
cloned repository and a gitlink has moved, the submodule working tree SHALL be
brought to the new pinned commit before ingestion proceeds. Materialization
SHALL be on by default with a per-source opt-out.

#### Scenario: A remote repository with submodules is cloned

- **WHEN** a remote repository source whose tree declares submodules is cloned
- **THEN** every declared submodule working tree (including transitive ones) is
  populated at its pinned gitlink commit before the source seeds

#### Scenario: A pull moves a gitlink

- **WHEN** an update of a cloned repository changes a submodule's pinned commit
- **THEN** the submodule working tree matches the new pin before re-ingestion,
  and entities derived from the old pin's content are superseded per the
  existing versioned-supersession contract

#### Scenario: Materialization is opted out

- **WHEN** a repository source is configured with submodule materialization off
- **THEN** no submodule tree is fetched, and the source status reports the
  declared-but-unmaterialized submodule paths as excluded by configuration —
  distinguishable from an unexpected empty tree

### Requirement: Unmaterialized submodule trees are loud

A watched tree whose `.gitmodules` declares a submodule path that has no
materialized working tree (directory missing or empty) SHALL be surfaced on
the source status surfaces, naming the affected submodule paths. Missing
submodule code SHALL NOT be silent.

#### Scenario: A local checkout has uninitialized submodules

- **WHEN** a configured local repository declares submodules whose directories
  are empty (cloned without recursion, never initialized)
- **THEN** the source's status reports each unmaterialized submodule path, on
  every status surface that carries per-source detail

#### Scenario: Initialization clears the signal

- **WHEN** a previously unmaterialized submodule tree becomes populated and is
  ingested
- **THEN** the status signal for that path clears within one status
  aggregation pass

### Requirement: Submodule entities carry the submodule's own identity

Entities produced from files under a declared submodule SHALL be scoped to the
submodule's own project identity — not the parent watch path's project — and
code entities SHALL carry version scoping derived from the submodule's pinned
gitlink SHA. IDs remain deterministic 6-part entity IDs and valid NATS KV
keys; identical (submodule repo, pinned SHA) pairs SHALL yield byte-identical
entity IDs wherever they are linked from.

#### Scenario: A submodule symbol is ingested

- **WHEN** a code symbol inside a materialized submodule is ingested
- **THEN** its entity ID uses the submodule's project scope with version
  scoping derived from the pinned gitlink SHA, and carries no identity
  contribution from the parent repository's project or version

#### Scenario: A shared submodule dedups across consumers

- **WHEN** two repository sources link the same submodule repository at the
  same pinned SHA
- **THEN** the same file yields byte-identical entity IDs from both sources,
  merging in the governed graph rather than forking per consumer

#### Scenario: Two pins of one submodule stay distinct

- **WHEN** the same submodule repository is linked at two different pinned
  SHAs (in one parent or across parents)
- **THEN** their entities carry distinct version scopes, and a symbol present
  only at the newer pin exists only under the newer pin's version scope

### Requirement: No double ingestion across the submodule boundary

A file under a declared submodule SHALL be ingested exactly once, under the
submodule scope. The parent watch path's walk SHALL NOT attribute files under
submodule paths to the parent scope.

#### Scenario: Parent walk meets a submodule directory

- **WHEN** the parent repository's watch path is walked and enters a declared
  submodule path
- **THEN** files under that path produce no entities under the parent's scope,
  and the submodule scope's entities for those files exist exactly once

#### Scenario: A transitive submodule tree is ingested

- **WHEN** a materialized submodule itself declares a submodule (nesting depth
  beyond one)
- **THEN** the transitive tree's files ingest exactly once and are never
  attributed to the top-level parent's scope
