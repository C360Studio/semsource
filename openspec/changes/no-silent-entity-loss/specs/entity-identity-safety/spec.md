# Delta: entity-identity-safety

## ADDED Requirements

### Requirement: Produced entity IDs satisfy the graph-ingest segment contract

Every entity ID produced by any SemSource source handler SHALL be a 6-part ID whose every segment
matches the graph-ingest per-segment contract (first byte alphanumeric; remaining bytes
`[a-zA-Z0-9_-]`), for all supported languages and all path/symbol shapes, while remaining
deterministic (same input → same ID) and collision-resistant (distinct inputs that sanitize
identically are disambiguated by a content-hash fallback).

#### Scenario: SvelteKit route component

- **WHEN** the AST source indexes `src/routes/+page.svelte`
- **THEN** the component and file entity IDs contain no `+` in any segment, pass semstreams
  `ValidateEntityID`, and are identical across repeated indexing runs

#### Scenario: Bracketed and grouped route directories

- **WHEN** a symbol is defined under `src/routes/[slug]/` or `src/routes/(group)/` or `@modal/`
- **THEN** its entity ID segments contain none of `[ ] ( ) @` and pass semstreams `ValidateEntityID`

#### Scenario: Dollar identifiers and leading underscores

- **WHEN** a TypeScript `const clicks$` or a symbol under `_examples/` is indexed
- **THEN** the produced segments start with an alphanumeric byte, contain no `$`, and pass
  semstreams `ValidateEntityID`

#### Scenario: Distinct inputs stay distinct

- **WHEN** two distinct raw symbols or paths sanitize to the same base segment
- **THEN** their final IDs differ (hash-fallback disambiguation)

### Requirement: Edge endpoints byte-match node identity

Relationship edges (contains, calls, references, and all other emitted edges) SHALL construct
their endpoint IDs through the same sanitization as node IDs, so that every edge endpoint that
refers to an indexed entity byte-matches that entity's ID.

#### Scenario: Contains edge for a sanitized component

- **WHEN** a file entity contains a symbol whose ID required sanitization
- **THEN** the `code.structure.contains` edge target equals the symbol's node ID byte-for-byte
