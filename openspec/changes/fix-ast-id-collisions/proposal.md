# One entity ID per declaration in the AST parsers

Approved-by:

Track: design (adds Requirements to three capabilities, changes entity identity, and adds a
publish-path guard ahead of the body offload).

## Why

Distinct declarations in one file can map to **one** entity ID (#329). A parser-level probe found
56 colliding IDs and 148 lost declarations on dogfood, and 2,264 IDs and 45,792 lost declarations
on OSH core. Every collision is within a single file. graph-ingest merges same-key writes by
(predicate, source) set and keeps the sets an arrival doesn't carry (design §Context). So a
collision is worse than last-write-wins:

- **233 OSH entities are hybrids.** The surviving Java overload keeps sibling overloads' `calls`,
  `parameter`, `returns`, `exported` and doc-comment statements.
- The code body is chosen by a goroutine race in `bodiesForResult`. That is the #324 R1 flip.

## What Changes

- **Guard (#333):** a parser test over every registered parser, plus a runtime check in
  ast-source **before** the body offload. A duplicate ID in one parse result is counted and
  surfaced, never stored last-write-wins.
- **Java (#330):** methods and constructors carry a discriminator derived from their erased
  parameter types, applied to every callable (the C++ `overloadID` precedent). A call edge
  resolves to one overload by argument count, or to none.
- **TS/JS/Svelte (#331):** function- and block-local variable declarations are no longer
  emitted. Module-level and class-member declarations keep their IDs.
- **Go (#332):** `var _ = …` blank declarations are not emitted.
- **Vendored bundles (#334, owner ruling 2026-11-22):** `*.min.js`, and a `.js` file whose
  `.min.js` sibling exists, are excluded by default. A new per-watch-path `include_vendored` setting opts back in.
- One clean storage break plus reseed for all of it (rule 1). #324 closes at R1 36/36 after it.

## Non-goals

- Re-ingest never removing dropped predicates: that is #336, which isn't identity-breaking.
- Python, C and C++ collision fixes. The guard measures them; any fix is a follow-up issue.
- Any change to graph-ingest's merge semantics. They behave as documented, so nothing goes upstream.

## Consumers

SemTeams and SemDragon read code entities and call edges through graph query, MCP and GraphQL.
semspec runs SemSource and reads the same contracts. All of them see new Java callable IDs and
fewer TS local entities after the reseed. The release note names the break.

## Capabilities

### Modified Capabilities

- `entity-identity-safety`: one ID per declaration per parse pass, guarded at test and runtime.
- `code-call-graph`: Java calls to overloaded methods resolve to one overload or none.
- `ast-source-configuration`: vendored and minified JavaScript is excluded by default.

## Impact

`source/ast/{java,ts,svelte,golang}`, `source/ast` guard test, `processor/ast-source`
(runtime guard before `bodiesForResult`, status counter), `handler/excludes.go`, scorecard R1
evidence, the release note, and the SETUP 03A storage-break notes.
