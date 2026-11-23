# entity-identity-safety

## MODIFIED Requirements

### Requirement: One entity ID per declaration within a parse pass
Every registered parser SHALL give distinct declarations distinct entity IDs or fold same-symbol declarations into one entity, and the AST source MUST NOT publish two declarations under one ID from any parser. Every discriminator SHALL apply to every declaration of its kind, never only to colliding ones.

#### Scenario: Java overloads get distinct, stable IDs
- **WHEN** a Java class declares `visit(Text)` and `visit(Count)`
- **THEN** the parse result carries two method entities with distinct IDs
- **AND** each ID is unchanged when a third `visit` overload is added or a sibling's parameter is renamed

#### Scenario: Block-scoped TypeScript locals are not entities
- **WHEN** a test file declares `const fetcher` inside five separate test callbacks
- **THEN** the parse result carries no entity for any of those `fetcher` declarations
- **AND** the file's module-level declarations keep their current IDs

#### Scenario: A Java constructor and a same-named method differ
- **WHEN** a Java class `Foo` declares the constructor `Foo(int)` and the method `void Foo(int)`
- **THEN** the parse result carries two entities with distinct IDs

#### Scenario: Java array dimensions are part of the signature
- **WHEN** a Java class declares `m(int)` and `m(int[])`
- **THEN** the parse result carries two entities with distinct IDs

#### Scenario: Local TypeScript functions are not entities
- **WHEN** two test callbacks in one file each declare `function inner() {}` and `const helper = () => {}`
- **THEN** the parse result carries no entity for any of those declarations

#### Scenario: TypeScript accessor pairs get distinct IDs
- **WHEN** a class declares `get size()` and `set size(v)`
- **THEN** the parse result carries two entities with distinct IDs

#### Scenario: Go blank declarations are not entities
- **WHEN** a Go file declares `var _ io.Reader = (*T)(nil)`, `var _ io.Writer = (*T)(nil)` and a `const ( _ = iota; A; B )` block
- **THEN** the parse result carries no entity for any blank declaration
- **AND** carries entities for `A` and `B`

#### Scenario: A C or C++ prototype and its definition fold onto the definition
- **WHEN** a C or C++ file declares `static int f(int x);` and later defines `static int f(int x) { return g(x); }`
- **THEN** the parse result carries one entity for `f`
- **AND** that entity carries the definition's lines and its call to `g`

#### Scenario: Alternative preprocessor branches fold onto the first
- **WHEN** a C or C++ file defines `move_cursor` under `#ifdef _WIN32` and again under `#else`
- **THEN** the parse result carries one entity for `move_cursor`, with the first branch's lines

#### Scenario: Python property accessors get distinct IDs
- **WHEN** a Python class declares `@property def x`, `@x.setter def x` and `@x.deleter def x`
- **THEN** the parse result carries three entities with the instances `x`, `x-setter` and `x-deleter`, all named `x`

#### Scenario: Python overload stubs give way to the implementation
- **WHEN** a Python module declares two `@overload def f` stubs and then `def f`
- **THEN** the parse result carries one entity for `f`, with the implementation's lines

#### Scenario: A redefined Python name keeps its last binding
- **WHEN** a Python class defines `predict` twice
- **THEN** the parse result carries one entity for `predict`, with the second definition's lines

#### Scenario: A duplicate ID that reaches the AST source is kept once and surfaced
- **WHEN** a parse result from any parser still carries two entities with one ID
- **THEN** the AST source publishes only the first in parse order, before any body is offloaded
- **AND** the source status `id_collisions` count increases by one, and one aggregated warning names the file
