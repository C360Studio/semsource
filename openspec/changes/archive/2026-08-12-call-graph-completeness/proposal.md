# Proposal: call-graph-completeness

## Why

`code_impact`'s contract is "what would break if you change it," and an empty closure reads as
"safe to change." Today that answer is confidently wrong in four ways, all surfaced by the
scorecard-v4 baselines (2026-08-12):

- **Java instance calls resolve to nothing.** `rootPerm.cloneAsTemplatePermission(...)` — the
  majority call form in idiomatic Java — produces no caller edge, because the receiver's declared
  type is never consulted (#141). Measured: OSH question P01 returns `impact: {nodes: 0}` against
  three real call sites, while class-qualified static calls resolve 2/2 on the same stack. The
  early adopter is a Java shop.
- **TS/JS and Svelte emit no call edges at all** (#149) — `code_impact` is blind on the languages
  of our own workbench and of typical adopter webstacks.
- **C and C++ emit no call edges at all** (#149).
- **Go emits edges to function-typed parameters** (`stat(...)` inside `decideLifecycleActions`),
  dangling edges that fail graph-query resolution at ERROR level on every seed and could count a
  phantom callee in a closure (#143).

The pre-tag bar (rack-and-stack agreed 2026-08-12) is that the misleading-answer class is closed
or honestly disclosed per language. The governing policy, confirmed against
`cpp/parser.go`'s measured doctrine ("3% of class names collide; a wrong edge is worse than a
missing one"): **fewer edges, more determinism** — conservative resolution with ties dropped,
no LLM edge synthesis ever (a guessed edge is a fabricated fact), full C++ semantic resolution
deferred to a future scip-clang/build-integration decision rather than approximated badly.

## What Changes

- **Java receiver resolution (#141):** instance-method calls through local variables, parameters,
  and fields resolve the receiver's *declared* type and walk the existing
  `declaringClass` supers machinery; unresolvable or ambiguous receivers are DROPPED, never
  guessed. OSH P01 flipping from `miss` to `correct` is the built-in acceptance proof.
- **Go parameter-call suppression (#143):** the Go call indexer skips call edges whose callee
  identifier binds to a function-typed parameter or local variable in the enclosing scope.
- **TS + Svelte call edges (#149, main slice):** deterministic call-edge extraction — direct
  function calls bound through the existing import maps and `tssyntax` machinery, plus
  conservative method resolution (own-class/`this.` only), everything else skipped. Svelte reuses
  the TS pass over script blocks.
- **C direct calls (#149, if admissible):** name-bound direct calls only (C has no overloading);
  function-pointer and macro-generated calls skipped.
- **Explicit non-goals recorded in the spec:** C++ call edges (deferred whole to the
  scip-clang/build-integration decision — the conservative subset ships only if it clears the
  same wrong-edge bar as inheritance resolution), dynamic dispatch/reflection in every language,
  and any model-assisted edge synthesis (ruled out as policy, not scope).
- Scorecard re-runs on both corpora close the change: dogfood 26/26 unchanged, OSH P01 → correct.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `code-call-graph`: caller-edge coverage becomes per-language contract — which call forms
  resolve, which are deliberately dropped, and the no-guessing rule (ties/unknown receivers emit
  nothing). Java gains declared-type receiver resolution; Go loses parameter-callee edges;
  TS/Svelte/C gain their deterministic subsets; C++'s absence becomes a stated requirement rather
  than an accident.

## Impact

- `source/ast/java/calls.go` (receiver resolution), `source/ast/golang/` (parameter suppression),
  `source/ast/ts/`, `source/ast/svelte/`, `source/ast/c/` (new call passes), shared conservatism
  helpers as needed.
- No transport, entity-identity, or query-surface changes; edges use the existing
  `code.relationship.calls` predicate.
- Consumers: `code_impact`/`code_context` answers gain callers on four languages; issues
  #141/#143/#149 close; scorecard OSH baseline improves; #142 (gradle titles) is deliberately
  NOT in this change (separate mechanical fix).
