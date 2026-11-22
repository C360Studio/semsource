# Migration: one entity ID per code declaration

Code entities used to collide: two declarations in one parse could get the same entity ID. The usual cases
were Java overloads, block-scoped TypeScript locals and Go `_` declarations. SemEngine graph-ingest merges same-key
writes per (predicate, source) set, so a collision did not keep the last declaration. It stored a **hybrid**:
predicates from both declarations, with a body picked by a race. After this change every declaration in a parse has
its own ID, or none at all ([#329](https://github.com/C360Studio/semsource/issues/329), OpenSpec change
`fix-ast-id-collisions`).

**This is an identity break. Provision fresh storage and re-ingest.** Existing IDs are not aliased or rewritten
(AGENTS.md rule 1).

## How to migrate

As at the beta.10 cutover: stop the stack, delete the graph state, and start again. Seeding is the first pass of
the normal loop, so there is nothing to export or replay.

```bash
docker compose down -v      # -v removes the NATS volume holding graph state
docker compose up -d
```

Wait for `/source-manifest/status` to report `phase: "ready"` before querying.

An in-place re-ingest is not enough. A hybrid whose ID survives keeps the other declaration's predicates, because
re-ingest never removes a predicate a source stops emitting
([#336](https://github.com/C360Studio/semsource/issues/336)). The corpus would look migrated while still serving
hybrids.

## What changes in identity

- **Java methods and constructors** carry a signature discriminator on every declaration, overloaded or not:
  `...-ModulePermissions-cloneAsTemplatePermission-c6e67c`. It hashes the kind plus the parameter type text, with
  type arguments erased, array dimensions kept, and type variables reduced to their first bound. Renaming a
  parameter or adding a sibling overload does not change it.
- **TypeScript, JavaScript and Svelte** accessor pairs and static members get distinct IDs. Module-level IDs are
  unchanged.
- Every other code ID is unchanged in shape. Docs, config, git and URL entities are not affected.

## What you will no longer see

These are deliberate, and each was ruled on #329.

- **Locals are not entities.** Function-scoped TS/JS/Svelte declarations, local functions included, no longer
  appear, and test-file locals go too. Namespace members are kept.
- **IIFE-, AMD- and UMD-wrapped JavaScript indexes as a file only.** Everything inside the wrapper function is
  local, so such a file contributes its file entity and nothing else.
- **Go `func init()` and blank declarations are not entities.** This covers `var _`, `const _`, `func _` and
  `type _`. A function called only from `init` loses that caller edge. A *method* named `init` is still indexed.
- **Same-arity Java overload calls have no edge.** A call binds to the one in-repo overload that accepts its
  argument count. When two overloads take the same count, no edge is emitted rather than a guessed one. A unique
  in-repo match still links when the receiver's hierarchy leaves the repo. On OSH, before a later fix restored
  some edges, this kept 91.9% of Java `calls` edges (23,319 → 21,432).
- **Unminified twins of minified assets index as files only.** A vendored `X.js` next to `X.min.js` is a file
  entity. Set `index_vendored_js: true` on the source in `semsource.json` to index it in full again.

## What you gain

- No hybrids: on the 5.1 probe, colliding IDs went from 56 to 0 on dogfood and from 2,264 to 0 on OSH core, with
  zero hybrids after.
- A runtime guard: if a duplicate still reaches the AST source, it is published once, and the source's status
  reports it as `id_collisions`.
- Rank-stable results across restarts and source order: the tier 0 R1 check is 36/36, where beta.10 scored 35/36
  ([#324](https://github.com/C360Studio/semsource/issues/324);
  [record](../../scripts/scorecard/results/SUMMARY-fix329-tier0.md)).

## Known follow-ups

- Python, C and C++ collisions are guarded but not fixed: [#338](https://github.com/C360Studio/semsource/issues/338).
- Predicates a re-ingest drops are never removed from a retained entity (#336, above).
- A Java anonymous-class wrong edge: [#339](https://github.com/C360Studio/semsource/issues/339).
