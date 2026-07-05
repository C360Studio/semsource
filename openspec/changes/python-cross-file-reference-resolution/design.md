# Design: Python cross-file reference resolution

## Context

The Python AST parser is **per-file**: `ParseFile(ctx, filePath)` parses one file
and emits its entities, published incrementally and re-run on watch events. There
is no project-wide symbol table. `typeNameToEntityID(typeName, filePath)` resolves
a referenced type name to an entity ID **against the current file's path** — after
task #43, a bare same-file name resolves correctly to `NewCodeEntity(...,TypeClass,
name, filePath).ID`, matching how the definition builds its own ID.

The gap: when `typeName` is defined in **another** module (imported), building the
ID against the referrer's `filePath` yields an ID that no entity owns → the
`extends` / `references` edge dangles. The parser already extracts import
statements (`extractImports`), but only as flat strings; it does not use them to
locate the defining module.

Constraints:
- **Preserve the per-file incremental/watch model** — no whole-project two-pass.
- **Entity IDs are deterministic and intrinsic** — a reference ID computed in file
  A must byte-match the definition ID computed when file B is parsed, regardless of
  parse order (this is what makes order-independent resolution possible).
- The #43 failure mode was an **ID-construction mismatch** (`.type.` vs `.class.`,
  raw project vs `SystemSlug`). Any new construction path must reuse the definition
  path exactly or it will dangle the same way.

## Goals / Non-Goals

**Goals**
- Resolve an imported type name in `class X(Base)` / annotations to the ID of
  `Base`'s definition in its own module, so the edge connects.
- Keep resolution per-file and filesystem-driven (no global index, no parse-order
  dependency).
- Shape the Python resolver so the same *pattern* (imports → name map → module →
  file → definition ID) is a clear template for #45 (call graph) and #46
  (multi-language), even though each language implements its own.

**Non-Goals** — see proposal. Notably: no two-pass index; no star-import /
re-export / dynamic resolution; no Java/TS/Go here; class-only resolution kept.

## Decisions

### D1 — Import-driven, per-file (not a project index)
Resolve using **the current file's own imports** plus a filesystem lookup, not a
global definition index. **Why:** a two-pass project index would break the
incremental/watch model (every file event would need a project re-scan) and add
stateful complexity for a correctness feature. Because entity IDs are intrinsic
and deterministic, a per-file resolver produces the *same* target ID the
definition will get — the edge resolves once both entities are in the graph, in
any order. *Alternative rejected:* whole-project symbol table (complete but
model-breaking; also unnecessary given deterministic IDs).

### D2 — Parse imports into a `localName → importBinding` map
Build, per file, a map from the locally-bound name to its origin:
`import a.b` (binds `a`), `import a.b as c` (binds `c`), `from a.b import N`
(binds `N` → module `a.b`, origin `N`), `from a.b import N as M` (binds `M`),
`from . import x` / `from ..p import y` (relative, with level). **Why:** Python
binds names in several shapes; the reference resolver needs the *origin module +
original name* for each bound name. This replaces the current flat string list for
resolution purposes (the string list can stay for the `Imports` facet).

### D3 — Filesystem module→file resolver, rooted at the source root
Map a dotted module to a file by probing the ingested tree: `a.b.c` →
`<root>/a/b/c.py` else `<root>/a/b/c/__init__.py`. Relative imports resolve
against the current file's package dir, walking up `level` parents. **Why:** the
parser already has `repoRoot`; a filesystem probe is cheap, needs no cross-file
parse state, and matches Python's package layout for the common case. *Alternative
rejected:* emulating full `sys.path`/site-packages resolution — over-scoped; we
only resolve **in-tree** modules (third-party/stdlib stay `external:`).

### D4 — Build the target ID through the definition's own construction path
Once the defining file is known, build the ID as
`NewCodeEntity(org, "python", project, TypeClass, originName, <relPath of that file>).ID`
— the identical call the definition makes. **Why:** this is the #43 lesson made a
rule. The `relPath` must be normalized exactly as `ParseFile` normalizes the
defining file's path (relative to `repoRoot`), or the segment won't match. Tests
assert `ref-id == def-id` for a real two-file parse, not just substring.

### D5 — Python resolver is language-local; the pattern is the shared asset
The import parser + module→file resolver live in `source/ast/python/` (Python
import/package semantics don't transfer). #45 (Python call graph) reuses this
Python resolver directly; #46 (Java/TS/Go) reuses the **pattern**, each with its
own module/package resolver. **Why:** honesty over premature abstraction — a
single cross-language "resolver" would encode four different import systems.
(Corrects the proposal's "shared machinery" phrasing: shared *pattern*, per-language
*impl*.)

### D6 — Class-only resolution retained; unresolved stays inert
An imported name is resolved as a class (`TypeClass`), consistent with same-file
behavior. An imported name that is actually a function/var, or a module that
doesn't resolve in-tree (third-party, star-import, dynamic), yields an
inert-dangling id (or the existing `external:` marker) — the engine drops
unresolved targets, so the result is "no edge," never a wrong edge. **Why:** keeps
the change strictly additive and bounds risk; correctly typing imported non-class
references would need the target's parse result (a step toward the rejected index).

## Risks / Trade-offs

- **[Ref/def ID mismatch — the #43 failure mode]** → Mitigation: one construction
  path (D4); regression tests assert `ref-id == def-id` for an actual two-file
  parse; live-validate against httpx (cross-module `from ._client import BaseClient`).
- **[module→file resolver wrong for src-layout / namespace pkgs / re-exports]** →
  Mitigation: resolve in-tree from `repoRoot` for the common package layout;
  anything unresolved is inert-dangling (never wrong). Namespace packages (PEP 420,
  no `__init__.py`) and `__init__.py` re-exports are explicit non-goals.
- **[Parse-order: target file not yet ingested]** → Mitigation: intrinsic
  deterministic IDs — the edge resolves whenever both entities exist, order-free.
- **[Filesystem stat per reference cost]** → Mitigation: probes are cheap and
  bounded by references-per-file; a per-parser `module→relPath` memo can be added if
  profiling shows it matters (not up front).

## Migration Plan

Purely additive — previously-dangling cross-file edges become resolved; nothing
that resolved before changes (same-file path is untouched; unimported names keep
existing behavior). No payload/ID-format change, so no consumer migration and no
re-index contract change. Rollback = revert; already-emitted edges are just triples
and are superseded on the next ingest. No feature flag needed.

## Open Questions

- **OQ1 — namespace packages (PEP 420, no `__init__.py`)?** Lean **defer**: probe
  `pkg/mod.py` and `pkg/__init__.py` only; a bare-namespace `pkg/` with no
  `__init__.py` stays unresolved (inert). Revisit if a real corpus needs it.
- **OQ2 — `__init__.py` re-exports** (`from ._client import BaseClient` in a
  package `__init__`, then `from httpx import BaseClient` elsewhere)? **Out of
  scope** (proposal); the second form resolves to `httpx/__init__.py`, which does
  not *define* `BaseClient` → inert. Accept for now; a re-export follow-up can
  read `__init__` bindings later.
- **OQ3 — memoize module→file?** Defer until profiled.
