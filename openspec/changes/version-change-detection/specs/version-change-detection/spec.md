# version-change-detection

## ADDED Requirements

### Requirement: Version changeset query

The system SHALL answer "what changed between two versions of a source" as a
symbol-level changeset, computed deterministically over the retained versioned-code
subgraphs with no model and no mutation of the graph.

Given a `project` and two version identifiers `from` and `to`, the system SHALL
correspond symbols across the two versions using version-independent identity
(`org, project, path, name, type, package`), compare each corresponding pair's
verbatim-body content hash, and classify every symbol as one of: `added` (present
only in `to`), `removed` (present only in `from`), `changed` (present in both with
differing body hashes), `unchanged` (present in both with equal body hashes), or
`indeterminate` (present in both but a body hash is absent or the two hashes come
from incomparable encodings). Correspondence SHALL be computed directly between the
two named versions, so any pair is supported, including non-adjacent versions.

The query SHALL be exposed over NATS request/reply (`graph.query.versionDiff`) and
HTTP, and as an MCP tool (`code_changes`). The result SHALL carry per-status counts;
by default it SHALL list the added/removed/changed/indeterminate symbols and omit the
(bulk) unchanged symbols, which remain reflected in the counts.

#### Scenario: A changed symbol is reported with before and after bodies

- **GIVEN** a symbol exists at both `from` and `to` of one project with differing
  verbatim bodies
- **WHEN** a version diff is requested for that `project`, `from`, `to`
- **THEN** the symbol is classified `changed` and (unless bodies are opted out)
  carries both its `from` verbatim body and its `to` verbatim body

#### Scenario: Added and removed symbols are classified by presence

- **WHEN** a symbol is present only in `to`
- **THEN** it is classified `added`; and a symbol present only in `from` is
  classified `removed`

#### Scenario: Identical bodies are unchanged, differing bodies are changed

- **WHEN** a corresponding symbol has equal body hashes across the two versions
- **THEN** it is classified `unchanged`; when the hashes differ it is classified
  `changed`

#### Scenario: An unclassifiable pair is surfaced, never guessed

- **WHEN** a corresponding symbol lacks a body hash on one side, or the two hashes
  come from different (incomparable) predicates
- **THEN** it is classified `indeterminate` and is not reported as `changed` or
  `unchanged`

#### Scenario: A missing version yields an honest empty, not a false diff

- **WHEN** either `from` or `to` resolves to no entities for the project
- **THEN** the response is empty with a note identifying the missing version, and is
  distinguishable from a genuine "nothing changed" result

#### Scenario: Large diffs are bounded and truncation is surfaced

- **WHEN** the changeset (with bodies) would exceed the configured symbol/byte budget
- **THEN** the response is truncated to the budget, flags itself as truncated, and
  reports how many symbols were dropped — it does not silently omit changes

### Requirement: Renames are not corresponded

Version change detection SHALL correspond symbols by exact version-independent
identity only; it SHALL NOT infer that a `removed` symbol and an `added` symbol are a
rename or move of the same logical symbol.

#### Scenario: A renamed symbol appears as a removal plus an addition

- **WHEN** a symbol is renamed or moved between `from` and `to`
- **THEN** it is reported as one `removed` (the old identity) and one `added` (the new
  identity), not as a single `changed` symbol
