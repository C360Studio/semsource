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
   - **Test:** `source/ast` parses a fixture per class (Java overloads, TS scoped consts, Go `_`) through each registered parser and fails on any duplicate ID. A second test runs the same check over the repo's own tree.
   - **Runtime:** `publishParseResult` deduplicates by ID **before** `bodiesForResult`. It keeps the first declaration in parse order, counts the rest on a new `id_collisions` status field, and logs one aggregated warning per file. Keeping the first is deterministic. Dropping is better than merging, because a merge fabricates edges. The count is not added to `ErrorCount`, since the entity that is kept is valid. *Alternative rejected:* failing the parse result, which would turn one parser bug into lost files.
2. **Java discriminator (#330).** Every method and constructor instance gets `-<6 hex>`: a SHA-256 prefix of its **erased parameter type list**. Generics are erased, names are simple, varargs become `[]`. The C++ `overloadID` (`source/ast/cpp/parser.go:398-420`) hashes the parameter text instead, but a type list survives parameter renames and matches JVM signature identity. It is applied to every callable, not only colliding ones, so adding an overload never re-identifies its siblings.
3. **Java call edges.** `methodIDIn` (`source/ast/java/calls.go:740`) builds the callee ID from its name and must now pick an overload:
   - one candidate: that one;
   - several: the unique candidate whose parameter count matches the argument count, with varargs matching `n ≥ k-1`;
   - otherwise: **no edge**.

   This follows "Unconfirmed Java calls never produce a wrong edge" and the C++ ambiguity rule (`cpp/parser.go:354-358`). Constructor calls follow the same rule. *Cost:* calls to same-arity overloads lose their edge. Task 2.2 measures this before code.
4. **TS/JS/Svelte locals (#331).** A `const`/`let`/`var` declared inside a function, arrow function, method or block is not emitted as an entity. Module-level declarations, class members and named nested **function declarations** stay. A nested function whose name collides is scoped under its enclosing symbol path, following `BuildScopedInstanceID`. Go already emits only package-level variables, so this restores parity. Task 2.3 checks that no call edge targets a local that would disappear.
5. **Go blanks (#332).** `var _ T = …` is skipped. A blank binds nothing anyone can look up.
6. **Vendored bundles (#334, owner ruling).** The AST file filter skips `*.min.js` and `*.min.mjs` by default. It also skips `X.js` when `X.min.js` sits beside it, the common vendor pattern. On OSH this covers all 17 bundles and none of OSH's own JS (`process_flow.js`, `connector.js` and others). Watch paths have no `includes` today: `WatchPathConfig` has only `excludes`, at `processor/ast-source/config.go:45`. So the opt-in is a new per-watch-path boolean, `include_vendored`, default `false`, which turns this rule off. The rule lives next to `defaultExcludedDirs` (`handler/excludes.go:29`) as a file-name rule, not a directory rule.
7. **One break.** Decisions 1–6 land together under one clean storage break plus reseed (rule 1): fresh owned graph storage, no aliasing or rewriting of old IDs. The release note and SETUP 03A record it.

## Risks / Trade-offs

- Same-arity Java overloads lose call edges (decision 3). This could move OSH call-graph answers. The scorecard OSH arm, currently 10/10, is the gate.
- Dropping TS locals removes entities someone might search for, mostly test fixtures. Retrieval impact is gated by dogfood R1 and the loose band.
- The `.min.js`-sibling rule misses a vendored file that ships without a minified twin. That is accepted, and the operator excludes it.
- The runtime guard hides a future parser bug behind a counter. The test guard is what keeps the counter at zero, and the status field makes a non-zero value visible.

## Migration Plan

Merge, then the release note announces the break. Operators provision fresh storage and re-ingest, as at the beta.10 cutover. No retained store is migrated in place.

## Open Questions

- **Q1 (decision 3):** if task 2.2 shows same-arity overloads carry a large share of OSH call sites, should edges also use declared argument types where the receiver resolution already knows them? Default: no, arity only.
- **Q3 (decision 6):** is a per-watch-path `include_vendored` boolean the right opt-in, rather than a general `includes` glob list? Default: the boolean. It's the smallest surface, and no other include need is known.
- **Q2 (decision 4):** should test files' locals be kept with a scope-path ID instead of dropped? Default: drop. They are fixtures, not symbols.
