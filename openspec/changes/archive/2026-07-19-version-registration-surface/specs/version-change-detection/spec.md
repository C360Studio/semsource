# Delta: version-change-detection

## ADDED Requirements

### Requirement: Version diff distinguishes storage failure from absence

`code_changes`/versionDiff body hydration SHALL report storage/resolve failures distinctly from
"no body exists for this entity", so a consumer can tell an incomplete answer from a complete one.

#### Scenario: Objectstore failure is visible

- **WHEN** body hydration fails for a changed symbol due to a storage error
- **THEN** the response marks that symbol's body as unavailable-due-to-error, not as empty

### Requirement: Version diff is provably reachable

An automated end-to-end proof SHALL ingest two versions of a fixture project through a real
registration surface and assert a correct `code_changes` answer through MCP, so the feature's
reachability cannot silently regress again.

#### Scenario: Two-version fixture answers correctly

- **WHEN** the fixture's two versions are ingested and `code_changes` is called for the pair
- **THEN** added/removed/changed/unchanged counts match the fixture's known diff
