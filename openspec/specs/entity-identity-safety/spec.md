# entity-identity-safety Specification

## Purpose
Every entity ID SemSource produces is deterministic AND valid per the graph-ingest
per-segment contract, for all languages and path/symbol shapes — sanitization is
centralized in `entityid` (SanitizeSegment: byte-stable for valid fragments,
hash-disambiguated for invalid ones) so nodes and edge endpoints always agree and
no produced entity can be silently rejected downstream (audit 2026-07-19).
## Requirements
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

### Requirement: Object-derived entity identity comes from the object key, never from a local path

An entity produced from an object store SHALL derive its identity from the object's key within the
bucket, and MUST NOT incorporate any local filesystem path used to fetch, cache, or materialize that
object. The bucket identifier SHALL supply the system segment, subject to the same normalization
every other system slug receives, and the existing explicit-project override SHALL take precedence
when configured.

This is the first identity source in SemSource where a purely local, non-intrinsic value —
a cache or temp path — is available at construction time and would produce a stable-looking ID that
silently changes between hosts, runs, or cache eviction. The requirement is therefore stated
directly rather than left implied by the general determinism contract.

#### Scenario: The same object fetched through different local locations

- **WHEN** the same bucket object is ingested on two hosts whose local cache or temp directories
  differ
- **THEN** both produce byte-identical entity IDs

#### Scenario: Re-ingest after a restart

- **WHEN** a service restarts and re-seeds a previously ingested prefix
- **THEN** every object produces the entity ID it produced before the restart
- **AND** the seed is re-derivable from the bucket alone

#### Scenario: Object keys that are not legal ID segments

- **WHEN** an object key contains `/`, spaces, or bytes outside the ID alphabet
- **THEN** the produced segments satisfy the graph-ingest per-segment contract
- **AND** they pass semstreams `ValidateEntityID`

#### Scenario: Keys that differ only past the truncation boundary

- **WHEN** two objects have keys sharing a long common prefix and differing only beyond the point
  where segment truncation applies
- **THEN** their entity IDs differ

#### Scenario: Content changes do not change identity

- **WHEN** an object at a given key is replaced with different content
- **THEN** its entity ID is unchanged
- **AND** the new content hash travels as a triple rather than as part of the ID

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
- **AND** that entity carries the definition's lines
- **AND** in C, which extracts calls, it carries the definition's call to `g`

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
- **AND** a class bound twice carries only the later body's methods

#### Scenario: A duplicate ID that reaches the AST source is kept once and surfaced
- **WHEN** a parse result from any parser still carries two entities with one ID
- **THEN** the AST source publishes only the first in parse order, before any body is offloaded
- **AND** the source status `id_collisions` count increases by one, and one aggregated warning names the file

