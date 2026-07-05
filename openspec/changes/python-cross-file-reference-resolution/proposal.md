# Proposal: Python cross-file reference resolution

## Why

Python type references that cross a file boundary — a base class in
`class Client(BaseClient)` or a type annotation whose type is imported from
another module — currently build their target entity ID against the **referrer's**
file path, not the file where the type is **defined**. So the `extends` /
`references` edge points at a non-existent ID and dangles: `code_impact` and the
`code_context` relations facet show no cross-module subclasses, implementers, or
referrers. Same-file references were fixed in task #43; cross-file is the
remaining gap, and real Python projects define base classes in one module and
subclass them in another (e.g. httpx `from ._client import BaseClient`).

## What Changes

- Add **import-driven cross-file reference resolution** to the Python parser.
  Using each file's own import statements (already extracted today) build a
  `local name → source module` map covering `import module`,
  `import module as alias`, `from module import Name`, `from module import Name as alias`,
  and relative imports (`from . import x`, `from ..pkg import y`).
- Add a **module-name → file-path resolver** scoped to the ingested source root:
  map a dotted module (`httpx._client`) to its file (`httpx/_client.py` or
  `httpx/_client/__init__.py`), honoring packages and relative-import level. This
  helper is deliberately reusable — task #45 (call graph) and task #46
  (multi-language parity) need the same name→definition resolution.
- When resolving a referenced type name, if it was imported, build the target ID
  against the **defining** module's file (matching how that module's definition
  builds its own ID — `NewCodeEntity` / `TypeClass`, same path), so the edge
  connects. Unimported / unresolvable names keep today's same-file behavior.
- No change to entity-ID **format**, payloads, or emitted predicates — only the
  **target** of already-emitted `extends` / `references` edges becomes correct.
  Strictly additive correctness: edges that dangled now resolve; nothing that
  resolved before changes.

## Capabilities

- **New Capabilities**: `code-reference-resolution` — how the AST layer resolves a
  referenced type/symbol name to the deterministic entity ID of its definition
  (same-file today; cross-file via imports with this change). Seeded now because
  this change is the first to formalize it; distilled from `source/ast` + verified
  against code.
- **Modified Capabilities**: none (no existing spec covers AST reference
  resolution; there is only `versioned-source-supersession`).

## Impact

- **Code**: `source/ast/python/parser.go` (`typeNameToEntityID`, import handling);
  a new import/module→file resolver (in `source/ast/python/` or a shared
  `source/ast/` helper if it generalizes cleanly). Unit tests + live validation
  against a real multi-module Python repo (httpx).
- **APIs / payloads**: none. 6-part entity IDs, predicates, and envelopes are
  unchanged.
- **Downstream consumers** (SemSpec, SemDragon, and any agent using `code_impact`
  / `code_context` over MCP): strictly better — cross-module reverse-dependency
  and relations become populated for Python. No breaking change.
- **Dependencies**: none new.

## Non-goals

- **Java / TS / Go** cross-file resolution — tracked separately as task #46; each
  has different type-kind semantics (Java `extends`→class vs `implements`→interface,
  TS real type-aliases, Go raw-project bug) and must be fixed + live-validated per
  language, not folded in here.
- **Call-graph edges** (`CodeCalls`) for Python — task #45; this change resolves
  type references (extends/references), not call sites, though it provides the
  module→file machinery #45 will reuse.
- **Star imports** (`from x import *`), **re-exports** (a name surfaced through a
  package `__init__.py` that imports it), and **dynamic** imports — out of scope;
  such references stay inert-dangling (documented), not silently wrong.
- **Whole-project two-pass indexing** — rejected: it breaks the per-file
  incremental/watch ingestion model. Resolution stays per-file, driven by that
  file's imports.
- **Third-party / stdlib** references (`external:` / `builtin:`) — unchanged;
  those are intentionally not local entities.
