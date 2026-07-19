# Delta: versioned-source-supersession

## ADDED Requirements

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
