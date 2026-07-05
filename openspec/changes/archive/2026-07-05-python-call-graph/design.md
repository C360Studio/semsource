# Design: Python call graph

## Context

`extractFunction` (`source/ast/python/parser.go`) builds a function/method entity
with params, return type, signature, and docstring — but never inspects the body
for calls, so `entity.Calls` stays empty and no `code.relationship.calls` triple
is emitted. The Go parser already does this: `extractFunctionCalls` walks the body
for `CallExpr` and resolves each via `callNameToEntityID`. Task #44 built, for
Python, an import-binding map (`lookupBinding`) and a module→file resolver
(`moduleToRelPath`) precisely so a referenced name resolves to its defining file —
the same primitives a call resolver needs. IDs are intrinsic and deterministic, so
a per-file resolver produces the callee's real definition ID, order-free.

## Goals / Non-Goals

**Goals**
- Populate `entity.Calls` for Python functions/methods with callee entity IDs that
  byte-match the callee's definition (so the edge resolves in the graph).
- Resolve the common, unambiguous call shapes: local module-level function,
  imported function, `module.func()` via an import binding, and `self.method()`.
- Reuse the #44 resolver primitives; no new project index.

**Non-Goals** — see proposal. Notably: no class-instantiation typing, no
type-inference for `obj.method()`, no star-import/dynamic resolution, Python only.

## Decisions

### D1 — Walk the body; resolve each callee through the definition's own path
Recursively walk a function's `body` for `call` nodes; for each, resolve the
`function` child to a callee ID built via `NewCodeEntity`/`NewScopedCodeEntity`
(function/method kind) — the identical construction the callee's definition uses,
so `ref-id == def-id` (the #43/#44 rule). Dedupe within a function (a `seen` set),
matching the Go parser. **Why:** one construction path is what makes the edge
resolve; the recursive walk captures calls in nested blocks/comprehensions/nested
`def`s (attributed to the enclosing entity, which is acceptable — they execute in
its scope and nested defs are not separate entities).

### D2 — Resolve bare calls: local function, import, else inert
For `call.function == identifier name`:
- `name` is a **module-level function defined in this file** (pre-scanned into a
  per-file set) → `NewCodeEntity(..., TypeFunction, name, filePath).ID`.
- else `name` is an **import binding** (`lookupBinding`) with an origin → resolve
  the module to a file (`moduleToRelPath`); in-tree → function ID against that
  file; out-of-tree → `external:name`.
- else → **inert (emit nothing)**. A bare name that is neither local-function nor
  imported is a builtin (`len`, `print`), a class instantiation, or a star-import
  target — none should become a fabricated in-tree call edge. **Why:** emitting a
  dangling `...function.<file>-print` for every builtin call is graph noise
  (the down-rank/noise concern); silence is correct here — no edge beats a wrong
  or inert-but-emitted one. *Alternative rejected:* mark builtins `builtin:` like
  types — call edges to builtins carry no dependency signal and add volume.

### D3 — Resolve `module.func()` via the import binding
For `call.function == attribute` with `object.attribute`:
- `object` text (possibly dotted) + `.attribute` → `lookupBinding(dotted)`; if the
  head is an imported module/alias, resolve to the module file and build a function
  ID for the origin name, or `external:` for an out-of-tree module. **Why:** this
  is the call analogue of #44's dotted-name type resolution; the same
  `lookupBinding` handles `import m` / `import m as a` / `from p import m`.

### D4 — Resolve `self.method()` / `cls.method()` to the intra-class method
When the receiver is `self` or `cls` and the enclosing entity is a method (its
`scope` is `[ClassName]`), build the callee as
`NewScopedCodeEntity(..., TypeMethod, scope, method, filePath).ID` — the same
scoped ID the sibling method's definition builds. **Why:** intra-class call edges
are the highest-value method-level relations and the scope is already threaded into
`extractFunction`; a receiver of `self`/`cls` unambiguously names the current class.
Any other attribute receiver (`obj.method()`) needs the type of `obj` (inference,
out of scope) → inert.

### D5 — Per-file local-function set, refreshed per ParseFile
Pre-scan the module's top-level `function_definition` names into a per-`Parser`
set (like the #44 `imports` map), reset each `ParseFile`. **Why:** distinguishes a
bare local-function call (resolve) from a builtin/instantiation (inert) without a
target parse; serialized per source path by the existing ast-source lock (#44 D6).

## Risks / Trade-offs

- **[Callee id mismatch → dangling call edge]** → Mitigation: single construction
  path (D1); unit tests assert `call-target == callee.ID` for real parses (local,
  imported, and `self.method`), not substring.
- **[Over-attribution of nested-def calls to the enclosing function]** → Accepted:
  nested defs are not separate entities, so their calls fold into the enclosing
  function — a reasonable approximation; revisit only if nested functions become
  first-class entities.
- **[Missed calls / false silence for exotic shapes]** (decorators-as-calls,
  `functools.partial`, dynamic dispatch) → left inert (no edge), never wrong.
- **[Builtins/instantiations emit nothing]** → deliberate (D2); documented so a
  reader knows an empty `callee` for a builtin-heavy function is expected.
- **[Concurrent parser state]** → the local-function set + import map are per-file
  Parser fields, serialized by ast-source's lock (#44 D6); covered by the existing
  `-race` guard.

## Migration Plan

Purely additive — Python entities gain `code.relationship.calls` triples where
resolvable; nothing else changes. No payload/ID-format change → no consumer
migration, no re-index contract change. Rollback = revert; emitted call triples are
superseded on the next ingest. No feature flag.

## Open Questions

- **OQ1 — type class-instantiation calls (`Foo()`)?** Lean **defer**: a per-file
  name→kind table (class vs function) would let `Foo()` resolve to the class as a
  `references`/`instantiates` edge. Out of scope now; stays inert.
- **OQ2 — resolve `super().__init__()`?** Defer: `super()` needs MRO context;
  inert for now.
- **OQ3 — emit `external:` call edges at all, or drop them too?** Keep `external:`
  for imported out-of-tree calls (consistent with #44 type resolution — they show
  external-API dependencies); drop only the truly unresolvable/builtin bare calls.
