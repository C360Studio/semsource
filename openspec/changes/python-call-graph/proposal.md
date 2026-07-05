# Proposal: Python call graph

## Why

The Python parser extracts entities, type-dependency edges (extends/references,
task #43/#44), and signatures — but emits **no** `code.relationship.calls` edges.
So for Python, `code_context`'s `callee`/`caller` relations are empty and
`code_impact` cannot follow the call graph: "who calls `authenticate`?" returns
nothing. The Go parser already emits call edges (`extractFunctionCalls`); Python
is the gap. Task #44's import→module→file resolver is exactly the machinery a
Python call resolver needs to point a call at its callee's definition — it was
built to be reused here.

## What Changes

- Walk each function/method body for **call sites** (`call` nodes) and set the
  owning entity's `Calls` to the resolved callee entity IDs — the parser's first
  `code.relationship.calls` emission for Python.
- **Resolve the callee to its definition's entity ID**, reusing the #44 resolver:
  - **Bare call** `foo()` — a local module-level function → the function's own ID;
    an imported name → resolved to the defining module's file (function ID) or,
    for an out-of-tree module, an `external:` marker.
  - **`module.func()`** where `module` (or its head) is an imported binding →
    resolved to `func` in that module's file, or `external:` for a stdlib/third-
    party module.
  - **`self.method()` / `cls.method()`** inside a method → the sibling method in
    the current class (scoped method ID), so intra-class call edges connect.
- Callee IDs are built through the **same `NewCodeEntity`/`NewScopedCodeEntity`
  path the definition uses** (function/method kind), so `ref-id == def-id` and the
  edge resolves — the task #43/#44 rule applied to calls.
- Add `caller`/`callee` are already lens roles; this change makes them populated
  for Python. No new predicate — `code.relationship.calls` already exists and is
  walked by the fusion code lens.

## Capabilities

- **New Capabilities**: `code-call-graph` — how the AST layer extracts call sites
  from a function body and resolves each callee to the deterministic entity ID of
  its definition (local function, imported function, or intra-class method).
  Seeded now because this change first formalizes it; distilled from the Go
  parser's existing call extraction + the Python #44 resolver, verified against code.

## Impact

- **Code**: `source/ast/python/parser.go` (call-site walk in `extractFunction`;
  a callee resolver reusing `lookupBinding` + `moduleToRelPath` from
  `source/ast/python/imports.go`). Unit tests + a live cross-file call resolution
  check through the graph stack.
- **APIs / payloads**: none. `code.relationship.calls`, IDs, and envelopes are
  unchanged; only Python now emits call edges.
- **Downstream consumers** (SemSpec, SemDragon, agents over MCP): strictly better —
  Python `code_context` call relations and `code_impact` call closures become
  populated. No breaking change.
- **Dependencies**: none new.

## Non-goals

- **Class instantiation as a call** (`Foo()` constructing an instance) — a bare
  call whose target is a class, not a function, is left inert (it resolves as a
  function ID that no entity owns → dropped) rather than mis-typed. A local
  name→kind table could type it later; out of scope here.
- **General attribute calls on locals** (`obj.method()` where `obj` is a variable
  whose type is only known by inference) — left inert; type inference is out of
  scope. Only `self`/`cls` receivers resolve.
- **Star-import / re-export / dynamic call targets** — inert (documented), never a
  wrong edge, consistent with #44.
- **Java / TS / Go call graphs** — Go already emits calls; multi-language call-graph
  parity is a separate later task. This change is Python only.
- **Cross-file whole-project indexing** — resolution stays per-file/filesystem-driven
  (the #44 model), no project symbol table.
