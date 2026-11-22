# entity-identity-safety

## ADDED Requirements

### Requirement: One entity ID per declaration within a parse pass
No AST parse result SHALL carry two entities with the same entity ID. Every registered parser MUST give distinct declarations distinct IDs, and the AST source MUST NOT publish two declarations under one ID.

#### Scenario: Java overloads get distinct, stable IDs
- **WHEN** a Java class declares `visit(Text)` and `visit(Count)`
- **THEN** the parse result carries two method entities with distinct IDs
- **AND** each ID is unchanged when a third `visit` overload is added or a sibling's parameter is renamed

#### Scenario: Block-scoped TypeScript locals are not entities
- **WHEN** a test file declares `const fetcher` inside five separate test callbacks
- **THEN** the parse result carries no entity for any of those `fetcher` declarations
- **AND** the file's module-level declarations keep their current IDs

#### Scenario: Go blank declarations are not entities
- **WHEN** a Go file declares `var _ io.Reader = (*T)(nil)` and `var _ io.Writer = (*T)(nil)`
- **THEN** the parse result carries no entity for either declaration

#### Scenario: A duplicate ID that reaches the AST source is kept once and surfaced
- **WHEN** a parse result still carries two entities with one ID
- **THEN** the AST source publishes only the first in parse order, before any body is offloaded
- **AND** the source status `id_collisions` count increases by one, and one aggregated warning names the file
