# Proposal: one entity ID per Python, C and C++ declaration (#338)

Approved-by: Coby Leuschke (2026-11-23) sha256:57967321fe4e

## Why

The `fix-ast-id-collisions` break (#329) made Go, Java, TypeScript, JavaScript and Svelte emit one entity ID per declaration and left Python, C and C++ to the AST source's runtime guard. The guard keeps the first declaration in parse order and counts the rest on `id_collisions`, so no hybrid entity is written, but the declaration it keeps is often the wrong one: a C prototype instead of its definition, a Python `@overload` stub instead of the implementation, and a Python property setter disappears.

Measured 2026-11-23 (#338, step 0 comment): 1,996 colliding IDs in a Python site-packages tree and 2,659 in the llama.cpp source. The largest C/C++ class, a prototype and its definition in one file, was missing from the issue's list.

## What changes

- **C and C++** fold every set of same-ID declarations in one file into one entity. A definition beats a prototype or forward declaration; otherwise the first in source order supplies the facts. The folded entities' `contains` children are unioned. The ID itself is unchanged, so no ordinal enters identity (rule 1). This covers prototype + definition, `#if`/`#else` twins, redefined `#define`s, and C++ overloads in a `.h` read by the C parser (later overloads are deliberately not emitted; declaring `cpp` keeps them).
- **Python** gives `@x.setter` the instance `x-setter` and `@x.deleter` the instance `x-deleter`; the getter keeps `x`. `@overload` stubs are not emitted when an implementation exists in the same scope; a stub-only name keeps its first stub. A redefined name keeps its last binding, which is what Python executes.
- The collision guard test covers Python, C and C++ with a fixture per class.

## Impact

- **Identity-breaking** for Python property setters and deleters (new IDs). C, C++ and other Python IDs keep their string, but which declaration's facts sit under them changes. Clean storage break plus reseed, batched with the next break (rule 1).
- Code: `source/ast/fold.go` (new), `source/ast/c/parser.go`, `source/ast/cpp/parser.go`, `source/ast/python/parser.go`, `source/ast/idcollision_guard_test.go`, fixtures under `source/ast/testdata/idcollisions/`.
- Spec: `entity-identity-safety` "One entity ID per declaration within a parse pass" now names every registered parser.
