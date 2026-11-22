# One entity ID per declaration in the AST parsers

Approved-by:

Track: design (Requirements in three capabilities, entity identity, a publish-path guard).

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
  parameter signature (kind, type text, array dimensions), applied to every callable (the C++ `overloadID` precedent). A call edge
  resolves by argument count across the class hierarchy, to one overload or none.
- **TS/JS/Svelte (#331):** declarations local to a function, method or static block are no longer
  emitted, in any form. Accessor pairs and static members are always discriminated.
- **Go (#332):** blank `var _`, `const _` and `func _` are not emitted. Several `init` per file:
  owner ruling Q4.
- **Vendored bundles (#334, owner ruling 2026-11-22):** an `X.js` whose `X.min.js` sibling exists
  becomes file-entity-only, like `.min.js` already is. Opt back in with `index_vendored_js`.
- One clean storage break plus reseed for all of it (rule 1). #324 closes at R1 36/36 after it.

## Non-goals

- Re-ingest never removing dropped predicates: that is #336, which isn't identity-breaking.
- Python, C and C++ collision fixes. The runtime guard covers them, and task 2.1 measures them;
  any fix is a follow-up issue.
- Any change to graph-ingest's merge semantics. They behave as documented, so nothing goes upstream.

## Consumers

SemTeams and SemDragon read code entities and call edges through graph query, MCP and GraphQL.
semspec runs SemSource and reads the same contracts. All of them see new Java callable IDs and
fewer TS local entities after the reseed. The release note names the break.

## Capabilities

### Modified Capabilities

- `entity-identity-safety`: one ID per declaration per parse pass, guarded at test and runtime.
- `code-call-graph`: Java calls to overloaded methods resolve to one overload or none.
- `asset-ingestion-guards`: unminified twins of minified assets are indexed as files.

## Impact

`source/ast/{java,ts,svelte,golang}`, `source/ast` guard test, `processor/ast-source`
(runtime guard before `bodiesForResult`, status counter), `processor/ast-source/assetguard.go`, scorecard R1
evidence, the release note, and the SETUP 03A storage-break notes.
