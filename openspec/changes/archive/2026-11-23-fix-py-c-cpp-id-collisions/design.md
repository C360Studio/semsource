# Design: fold same-ID declarations (#338)

## Context

Owner rulings are on #338 (comment 7115830103, 2026-11-23). The runtime guard from #329 (`processor/ast-source`, `id_collisions`) stays as the backstop and is not changed.

## Decisions

1. **One fold helper, `ast.FoldDeclarations(entities, replaces)`.** It walks a parse result in order and keeps one entity per ID at the position of the first occurrence. `replaces(candidate, kept)` decides whether a later same-ID entity takes the slot. The dropped entity's `Contains` IDs are appended to the survivor's, and every survivor's `Contains` is deduplicated, because the file and class entities list a child ID once per declaration. No other fact is merged: merging calls or doc comments across `#if` branches would fabricate a combination no build compiles.
2. **C/C++ policy: definition beats declaration, otherwise first.** A definition is a `function_definition` or a node with a `body` field, recorded per entity when the parser builds it. Prototypes stay emitted when no definition exists in the file, so a header-only API still indexes. Running the fold inside `ParseFile` also fixes the C++ base-class resolution, which treats two entries with one ID as ambiguous.
3. **Python policy: non-stub beats stub, the last non-stub wins.** A stub is a def decorated `@overload` or `@<module>.overload`. Among non-stubs the later binding replaces the earlier, as Python's runtime does. *Amended at review (#356):* a rebound class keeps only the later body's members. Before the fold, the parser drops the methods and `Contains` of every class that does not survive, so decision 1's union adds nothing for Python. C/C++ keep the union.
4. **Python accessor discriminator.** Read from the decorator's syntax tree (an `attribute` whose object is the method's own name), so spacing, parentheses and comments don't matter (*amended at review, #356*). Applied to every setter and deleter, never only on collision (the #331 precedent), so adding a setter never re-identifies the getter. The getter is the property itself and keeps the plain name; `Name` stays `x` for all three.

## Risks

- A C++ overload set in a `.h` read by the C parser keeps only the first overload. The parser cannot tell C++ from C there (routing's comment says why it doesn't sniff). The operator declares `cpp`.
- `#if` twins keep the first branch's calls and signature. The other branch's calls are lost, as they are today under the runtime guard.
