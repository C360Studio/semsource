# Design: Multi-language reference-ID parity (Java / TS / Go)

## Context

Three tree-sitter/`go/ast` parsers (`source/ast/java`, `.../ts`, `.../golang`)
each build type-dependency edges (`extends`, `implements`, `references`, `embeds`)
by calling a language-local `typeNameToEntityID(name, filePath)`. Every one of
them constructs the target as:

```go
instance := ast.BuildInstanceID(filePath, typeName, ast.TypeType)   // hard-coded TypeType
entityid.Build(p.org, PlatformSemsource, lang, p.project, "type", instance) // raw p.project
```

The definition builds its own ID via `ast.NewCodeEntity` / `NewScopedCodeEntity`,
which (a) uses the real kind segment (`class`/`interface`/`struct`/`enum`) and
(b) runs `project` through `entityid.SystemSlug`. So a reference and its
definition disagree on two segments (kind, system) and, across files, on the path
segment too. This is the same class of ID-construction mismatch #43/#44 fixed for
Python; it is unfixed in the other three languages.

The parsers are **per-file** (`ParseFile(ctx, filePath)`), published incrementally
and re-run on watch events — the same constraint #44 worked under. Entity IDs are
intrinsic and deterministic, so a per-file resolver produces the *same* target ID
the definition will get, order-free.

## Goals / Non-Goals

**Goals**
- Every resolved `extends`/`implements`/`references`/`embeds` edge in Java, TS,
  and Go targets the definition's own entity ID (byte-identical), same-file and
  cross-file.
- Reuse #44's *pattern* per language; do not build a cross-language "resolver"
  that would encode four import systems.
- Keep resolution per-file / per-directory and filesystem-driven — no
  whole-project index, no parse-order dependency.

**Non-Goals** — see proposal. Notably: no call graph; no unknown-kind cross-file
field/param resolution (inert, not wrong); no star-import / re-export / dynamic /
namespace-package / tsconfig-paths resolution.

## Decisions

### D1 — One construction path: build every reference through `NewCodeEntity`
Replace each `typeNameToEntityID`'s ad-hoc `entityid.Build(...)` with
`ast.NewCodeEntity(p.org, lang, p.project, kind, name, definingRelPath).ID` — the
*identical* call the definition makes. **Why:** this is the #43/#44 lesson made a
rule and it fixes the `SystemSlug` (bug 2) and kind-segment (bug 1) mismatches for
free, because both sides now run the same code. The three inputs the resolver must
supply are `kind`, `name`, and `definingRelPath`; everything else is fixed per
parser. *Alternative rejected:* patching the raw `entityid.Build` call in place —
it would still drift from `NewCodeEntity` the next time that path changes.

### D2 — Kind from syntactic position for hierarchy edges
Where the grammar fixes the target kind, pass it explicitly:
- Java: class `extends` → `TypeClass`; class/interface/enum `implements` →
  `TypeInterface`; interface `extends` → `TypeInterface`.
- TS: class `extends` → `TypeClass`; class `implements` → `TypeInterface`;
  interface `extends` → `TypeInterface`.

**Why:** these are exactly the edges that feed `code_impact`'s subclass/implementer
closures, and their kind is unambiguous from position — no target parse needed.
Python (#44) resolved base classes as `TypeClass` for the same reason; this
generalizes it to the interface case the other languages have. *Alternative
rejected:* resolving kind by parsing the target — needed only for unknown-kind
references (see D4), not for these.

### D3 — Per-language cross-file resolver (imports/package → file)
Each language maps a referenced name to the *defining* file, then D1 builds the ID
against that file's repo-root-relative path (normalized exactly as `ParseFile`
normalizes a parsed file's path — the D4 rule from #44):

- **Java** — parse `import a.b.C;` into `simpleName("C") → FQN("a.b.C")`; resolve
  an unqualified referenced name via that map, else assume same-package. FQN →
  `filepath.Join(parts...) + ".java"` under `repoRoot`; same-package →
  `<referrer package dir>/<Name>.java`. The referrer's package dir is
  `path.Dir(relPath)`. Probe the filesystem; `ok=false` ⇒ leave inert.
- **TS/JS** — parse `import { Base } from './base'` / `import Base from './base'`
  into `localName → moduleSpecifier`; resolve only **relative** specifiers
  (`./`, `../`) against the importer's dir, probing `base.ts`, `base.tsx`,
  `base.js`, `base.jsx`, `base.mts`, `base.cts`, `base.mjs`, `base.cjs`, then
  `base/index.*`. Bare specifiers (`react`) stay `external:`/inert.
- **Go** — a bare local type is same-package = same directory. A memoized
  per-directory sibling scan (`go/parser` over the referrer's dir, lightweight —
  only top-level `type` decls) yields `name → (definingRelPath, kind)`. Package-
  qualified `pkg.Type` keeps today's `external:` behavior.

**Why:** import/package→file semantics genuinely differ (Java package==dir tree,
TS module specifiers, Go same-package-across-files); a shared resolver would be a
lie. All three are cheap filesystem probes, no cross-file parse state beyond Go's
bounded per-directory memo. *Alternative rejected:* whole-project symbol table —
breaks incremental watch, and unnecessary given deterministic IDs.

### D4 — Go carries kind *and* file from the sibling scan; Java/TS carry kind from position
Go embeds/references have no positional kind (an embedded local type may be a
struct or an interface), so Go's resolver returns the real kind discovered by the
sibling scan and D1 uses it. Java/TS hierarchy edges get their kind from D2 and the
file from D3. For an unresolved name, all three fall back to today's behavior
(same-file `TypeType` guess or `external:`/`builtin:` marker) — **inert, never a
wrong edge** (the graph engine drops unresolved targets).

### D5 — Same-file kind table for unknown-kind references (Java field / TS param / return)
For unknown-kind references that resolve to a **same-file** definition (a Java
field whose type is a class defined in the same file), build a per-file
`name → CodeEntityType` table from the file's own definitions and use it so the
edge connects. Cross-file unknown-kind references stay inert (D4). **Why:** the
same-file table is free (the parser already has every definition in hand) and
removes a common dangle without any resolver; cross-file unknown-kind needs a
target parse (a step toward the rejected index) so it stays a non-goal.

### D6 — Per-file resolver state, serialized by the existing ast-source lock
The import/name maps live on a `Parser` field refreshed each `ParseFile` (as
Python's do). ast-source drives a reused `Parser` from two default-on goroutines
(watcher + periodic reindex); #44 already serialized this with a per-`pathWatcher`
mutex across `ParseFile`, which covers all languages. The Go per-directory memo is
guarded the same way; a `-race` regression test covers it. **Why:** reuse the lock
#44 added rather than introduce per-parser locking.

## Risks / Trade-offs

- **[Ref/def ID mismatch — the recurring failure mode]** → Mitigation: single
  construction path (D1); every regression test asserts `ref-id == def-id` for a
  real parse, not substring — the exact gap the current tests have.
- **[Wrong kind for `implements` a class / `extends` an interface in weird code]**
  → position-driven kind (D2) is correct for well-formed Java/TS; malformed code
  yields an inert target (dropped), never a wrong edge.
- **[Java same-package resolution without imports]** → probe
  `<pkgdir>/<Name>.java`; a public class in Java lives in a file named for it, so
  the common case resolves; anything else stays inert.
- **[TS resolution — path aliases, barrels, node_modules]** → explicit non-goals;
  only relative in-tree specifiers resolve, everything else inert.
- **[Go sibling-scan cost]** → memoized per directory; bounded by files-per-package,
  parsed once and cached on the `Parser`. Invalidated implicitly per `ParseFile`
  is unnecessary — type decls are stable within a parse pass; the memo is keyed by
  directory and lives for the parser's lifetime, refreshed on watch reparse of a
  sibling (acceptably stale between events — IDs are deterministic so a stale kind
  only risks a transient dangle, never a wrong edge).
- **[Concurrent parser state]** → covered by the existing ast-source lock (D6);
  `-race` test.

## Migration Plan

Purely additive — previously-dangling edges become resolved; nothing that resolved
before changes (same-file same-kind paths untouched; unresolved names keep existing
inert behavior). No payload/ID-format change → no consumer migration, no re-index
contract change. Rollback = revert; emitted edges are triples, superseded on next
ingest. No feature flag.

## Open Questions

- **OQ1 — Go memo staleness across watch events?** Lean **accept**: the memo is
  keyed by directory; a rename/kind-change in a sibling between events could leave
  a stale `(relPath, kind)` until that directory is rescanned. Deterministic IDs
  bound the blast radius to a transient dangle (never a wrong edge). Revisit only
  if a real corpus shows churn; a per-`ParseFile` memo reset is the cheap fix.
- **OQ2 — Java nested/inner-class references (`Outer.Inner`)?** Defer: resolve the
  outer simple name; `Outer.Inner` as a referenced type stays inert for now
  (scoped-ID construction for cross-file inner types is out of scope).
- **OQ3 — extend cross-file to unknown-kind references (Java field, TS param)?**
  Deferred non-goal (D5) — needs a target parse. Same-file table covers the common
  case; revisit if impact analysis needs cross-file field-type edges.
