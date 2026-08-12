# Design: call-graph-completeness

## Context

See `proposal.md` — Why. The governing doctrine is already codified and measured in this repo:
resolution is per-file/filesystem-driven, FAILS INERT, and "a wrong edge is worse than a missing
one" (`cpp/parser.go`'s 3%-collision measurement; the current-truth `code-call-graph` spec).
Every decision below is an application of that doctrine to a new language or call form. Evidence
for each gap lives in issues #141/#143/#149 (2026-08-12 comments) and the scorecard-v4 OSH
baseline (P01: `impact:{nodes:0}` against three real call sites).

## Goals / Non-Goals

**Goals:** close the misleading-empty-closure class deterministically; make coverage a stated
per-language contract; prove the Java fix with the instrument that caught it (OSH P01).

**Non-goals:** C++ call edges (whole language deferred to a scip-clang/build-integration
decision — its conservative subset ships only if a Meshtastic-style measurement clears the
wrong-edge bar); dynamic dispatch, reflection, callbacks-through-frameworks anywhere;
cross-repo resolution; any model-assisted inference (prohibited by spec, not just descoped).

## Decisions

### D1 — Java receivers resolve through a declared-type table, ties dropped

`java/calls.go` already builds per-class method tables and a supers walk (`declaringClass`,
nearest-declaring-ancestor, ties dropped). Extend the per-method scope with a declared-type
table: parameters and locals from their declaration nodes, fields from the enclosing class's
field declarations (walking supers for inherited fields uses the same ancestor cap). A call
`x.m(...)` binds `x` in that table; a hit resolves `m` via `declaringClass(typeOf(x), m)`.
Generic type variables, `var` declarations whose initializer type is not a plain in-tree class
instantiation, unresolvable types, and ties all DROP the edge. No flow analysis: a reassigned
variable uses its declared type — Java's static type is the honest bound, and when the dynamic
type differs the declared type's declaring ancestor is still the method contract being called.

### D2 — Go parameter/local function values are suppressed at emit time

The Go extractor gains a scope check before emitting a call edge: if the callee identifier is
declared as a function-typed parameter or local variable of the enclosing function (or an
enclosing closure), skip. Implementation is a small scope-stack walk over the already-parsed
declaration nodes — no type inference beyond "declared with a func type." This kills the
dangling `...function.<file>-stat` class (#143) at the source instead of teaching the graph to
tolerate it.

### D3 — TS/Svelte get a dedicated calls pass reusing the existing resolution machinery

New `ts/calls.go`, mirroring the Python/Java pass structure: walk function/method/arrow bodies
for `call_expression`; bind bare callees against (a) same-file function declarations, then (b)
the file's import bindings resolved through the existing `ts/imports.go` module→file machinery
(named imports only; default imports resolve when the target module's default export is a named
in-tree function; namespace-qualified `ns.f()` resolves through the namespace import binding).
`this.m()` resolves against the own class's method table only — no supers walk in v1 (TS
hierarchies are structural; a wrong walk is worse than none). Everything else — property chains
(`a.b.c()`), computed callees, re-export barrels beyond one hop — stays inert. The Svelte parser
already extracts script blocks through the shared tssyntax path; it invokes the same pass, so
component→module edges come for free.

### D4 — C emits name-bound direct calls only

C has no overloading and no methods: a `call_expression` whose function is a bare identifier
binds against in-tree function definitions by name. Two guards keep it inert-honest: skip when
the identifier is a declared function pointer (parameter or local — same rule as D2), and skip
when the name binds to more than one in-tree definition (duplicate statics across translation
units — the C analog of the 3% collision measurement; measure the actual collision rate on the
OSH corpus... on a real C corpus during implementation and record it in the task). Macros are
invisible post-lex and therefore self-skipping.

### D5 — Acceptance is instrument-verified, not assertion-verified

Unit tests pin each language's resolvable and deliberately-inert forms (table-driven, per the
spec scenarios). The change-level acceptance is the scorecard: OSH P01 flips `miss` → `correct`
(the exact gap that motivated #141) with the dogfood baseline unchanged at 26/26, and no new
`not found` graph-query errors during either seed (the #143 signal). Both re-runs land in
`results/` with a SUMMARY delta note.

## Risks / Trade-offs

- [Inherited fields widen the conflict net] → a local variable shadowing an inherited field of a
  DIFFERENT type now marks the name conflicted and drops call sites that resolved before the
  merge existed. Deliberate: the flattened-scope table cannot tell which declaration is live, and
  inert beats guessing. (Review finding 6 — accepted narrowing.)
- [C's index cannot see per-source configured excludes] → the registry factory carries no config,
  so a configured-excluded directory that uniquely defines a name can still produce an edge to an
  entity the ingester never creates. Dangling (loud at query resolution), never wrong-real;
  documented at defSkipDirs, which is test-pinned to handler.DefaultExcludedDirNames.

- [Java field-type tables inflate parse cost] → tables are per-file and bounded by the existing
  ancestor cap; measure seed wall-clock on OSH before/after (the 1,311 s baseline exists).
- [TS default/namespace import edge cases produce wrong bindings] → named imports first;
  default/namespace forms ship only with their inert fallbacks tested; barrels beyond one hop
  drop.
- [C name collisions across translation units] → unique-binding rule + measured collision rate;
  if the rate is material the C slice ships file-local-only and says so in the coverage contract.
- [Scope creep toward C++] → C++ is a stated non-goal; any C++ work is a new change with its own
  measurement.

## Open Questions

- Whether Svelte store-subscription call forms (`$store` autosubscriptions invoking functions)
  are worth a rule or stay inert in v1 — decide from what the ui/ corpus actually contains
  during implementation; default inert.
