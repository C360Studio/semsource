# Design: one entity ID per declaration

## Context

The evidence is in #329 ([step 0](https://github.com/C360Studio/semsource/issues/329#issuecomment-7078792676), reviewed by `graph-event-reviewer`). The pin is semengine `v0.1.0-alpha.6`.

**graph-ingest's same-key contract** on `graph.ingest.entity`:
1. Writes to one ID run one at a time on one lane, in stream arrival order, as a compare-and-set read-modify-write (`processor/graph-ingest/keyed_ingest.go:18-22`, `entity_writes.go:170`).
2. Each (subject, predicate, source) set in the arrival replaces the stored set with that key. **Stored sets the arrival doesn't carry are kept** (`graph/helpers.go:26-36`; `entity_writes.go:224-233`).
3. A set is skipped only if it is strictly older than the stored set. Equal timestamps apply (`graph/helpers.go:66`). `MessageType` and `StorageRef` are taken only when no set was older (`entity_writes.go:243-248`).
4. A statement with no timestamp takes the envelope's (`statement_metadata.go:60-96`). That is wall-clock milliseconds stamped at drain time (`internal/entitypub/publisher.go:456`). Code statements carry none, so sequential publishes usually tie, and arrival order decides.

**What a collision does today:**
- One `publishParseResult` publishes in parse order (`processor/ast-source/component.go:783-808`), through one FIFO drain loop. The last declaration's sets win, and every predicate only an earlier declaration carried survives. Replaying through `ReplaceBySource` gives **233 hybrid entities on OSH**: 112 `calls`, 82 doc comments, 64 `parameter`, 61 `exported` and 33 `returns` residue statements. Dogfood has 0 hybrids, because its colliding locals carry uniform predicate sets.
- `bodiesForResult` fills a map keyed by entity ID from concurrent workers (`processor/ast-source/bodystore.go:115-165`). Every colliding declaration publishes the one body whichever goroutine attached last (`component.go:794`). This alone explains the #324 flip.

The races are ours, in identity. graph-ingest is behaving as documented.

## Goals / Non-Goals

**Goals:** no parse result carries two declarations under one ID, for any registered parser. A regression is caught by a test, and at runtime is surfaced rather than merged. Call edges stay correct: one right overload, or none.

**Non-goals:** #336 (removing dropped predicates), Python/C/C++ fixes beyond measuring them, and any change to graph-ingest.

## Decisions

1. **Guard at two layers (#333).**
   - **Test:** `source/ast` parses a fixture per class through each registered parser and fails on any duplicate ID. A second test runs the same check over the repo's own tree.
   - **Runtime:** `publishParseResult` deduplicates by ID **before** `bodiesForResult`. That one function serves seed, watch and reindex (`component.go:391,530,611`), so one place covers all three.
     - It keeps the first declaration in parse order. Every parser emits in tree-sitter child order or `file.Decls` order, never in map order, so "first" is deterministic.
     - It counts the rest on a new `id_collisions` status field and logs one aggregated warning per file.
     - Dropping beats merging, because a merge fabricates edges. The count stays out of `ErrorCount`, since the entity that is kept is valid.
     - During the initial index the same check runs across the watch path's whole result set (`component.go:389`). `SanitizePathSegment` is many-to-one: `a/b.ts` and `a-b.ts` both fold to `a-b-ts`.
   - *Alternative rejected:* failing the parse result, which would turn one parser bug into lost files.
2. **Java discriminator (#330).** Every method and constructor instance gets `-<6 hex>`, a SHA-256 prefix over:
   - the **kind**: a constructor `Foo(…)` and a method `void Foo(…)` are both `TypeMethod`;
   - the **declared parameter type text** with type arguments removed, array dimensions **kept**, varargs as `[]`, and qualifiers as written;
   - type variables resolved to their first bound, so `<T extends A> m(T)` and `<T extends B> m(T)` differ.

   Do not reuse `extractTypeReference`, because it strips array dimensions (`java/parser.go:860-864`). The hash only identifies a declaration; edges never compute it at a call site (decision 3). The C++ `overloadID` precedent (`cpp/parser.go:398-420`) applies: the discriminator goes on every callable, so adding an overload never re-identifies its siblings.
3. **Java call edges.** Overload selection spans the hierarchy. `declaringClass` (`java/calls.go:350-389`) currently stops at the nearest class that declares the *name*. Take a subclass `foo(int)` and a superclass `foo(int,int)`: `foo(1,2)` would edge wrongly to `foo(int)`. So:
   - The walk collects candidates across the ancestors. For an identical signature (an override), the nearest declaration wins.
   - The call resolves to the unique candidate whose parameter count accepts the argument count (varargs `n ≥ k-1`), **even when there is only one candidate**. Otherwise, or when the ancestor chain leaves the repo (an external type), there is **no edge**.
   - `classMembers.methods` and `hasCtor` (`calls.go:47-48`) carry each overload's parameter count, varargs flag and ID. `constructorTargetID` (`:752-760`) follows the same rule.

   This follows "Unconfirmed Java calls never produce a wrong edge" and the C++ ambiguity rule (`cpp/parser.go:354-358`). *Cost:* calls to same-arity overloads lose their edge; task 2.2 measures how many. `parameter` and `returns` edges are type references and are unaffected.
4. **TS/JS/Svelte (#331).**
   - A *local* is a declaration with a function, arrow, method or static-block ancestor. That definition keeps namespace members, whose body is also a `statement_block`.
   - **Locals are not emitted, whatever their form:** `const`/`let`/`var`, `function inner(){}`, `const inner = () => {}` (both of those are `TypeFunction`, `ts/parser.go:700`), and local classes with their methods.
   - No edge depends on them: `localFuncs` holds top-level names only (`ts/calls.go:103-129`), and a local shadows the call into inertness.
   - **Class members are discriminated always, never only on collision:** a getter and setter pair gets `-get` / `-set`, and a static member gets `-static` (`ts/parser.go:413-418,445`).
   - Go emits only package-level declarations, so this restores parity.
5. **Go (#332).** Blank declarations are skipped in every form: `var _`, `const _` (including `iota` blocks, `golang/parser.go:224-233`) and `func _()`. A blank binds nothing anyone can look up. Multiple `func init()` in one file (`:270`) take Q4's ruling.
6. **Vendored bundles (#334).** `.min.js` files are already file-entity-only under `asset-ingestion-guards` (`processor/ast-source/assetguard.go:44,96`). The OSH collisions come from their unminified **twins**: `jquery.js`, `lodash.js`, `joint.js` and `backbone.js`.
   - The asset guard also treats `X.js` as a vendored asset when `X.min.js` sits beside it: file entity only, no symbols. That matches the existing spec, which keeps a file entity and drops the symbols, rather than skipping the file outright.
   - OSH's own JS (`process_flow.js`, `connector.js`) is unaffected.
   - Opt-in is a new ast-source watch-path boolean, `index_vendored_js` (default `false`). Doc and cfgfile sources don't parse `.js`, and `handler/excludes.go` stays the every-source floor.
   - *Limit:* a `X.min.js` created after `X.js` doesn't re-trigger `X.js`, and reindex skips unchanged hashes (`component.go:602-606`). The stale symbols stay until the next lifecycle pass. That is accepted and documented.
7. **One break.** Decisions 1–6 land together under one clean storage break plus reseed (rule 1): fresh owned graph storage, no aliasing or rewriting of old IDs. The release note and SETUP 03A record it.

## Risks / Trade-offs

- Same-arity Java overloads, and calls whose ancestor chain leaves the repo, lose call edges (decision 3). That could move OSH answers. The scorecard OSH arm, currently 10/10, is the gate. Split overloads should both seed a name query through fusion's multi-seed impact walk; task 5.3 confirms it.
- Dropping TS locals removes entities someone might search for, mostly test fixtures. Dogfood R1 and the loose band gate it.
- The twin rule misses a vendored file that has no `.min.js` twin. The operator excludes it.
- The runtime guard could hide a future parser bug behind a counter. The test guard keeps the counter at zero, and the status field makes a non-zero value visible.
- The symbol cap (`component.go:772`) counts duplicates before the dedup. That is harmless.

## Migration Plan

Merge, then the release note announces the break. Operators provision fresh storage and re-ingest, as at the beta.10 cutover. No retained store is migrated in place.

## Open Questions

- **Q1 (decision 3):** if task 2.2 shows same-arity overloads carry a large share of OSH call sites, should edges also use argument types where receiver resolution already knows them? Default: no, arity only.
- **Q2 (decision 4):** should test files' locals be kept under a scope-path ID instead of dropped? Default: drop. They are fixtures, not symbols.
- **Q3 (decision 6):** the #334 ruling said "exclude". This design keeps a file entity for twins and drops only their symbols, matching `asset-ingestion-guards`. Is that acceptable? Opt-in is `index_vendored_js`.
- **Q4 (decision 5):** how should several `func init()` in one file be identified? Options:
  - (a) an ordinal in source order, `init-1` and `init-2`. Go itself runs inits in source order, but reordering them re-identifies them.
  - (b) skip `init` entirely. It is never a call target, but its own outgoing calls are lost.

  Default: (a).
