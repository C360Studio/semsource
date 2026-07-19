# Design: Go Call-Graph Recall

## Context

The audit proved the Go call graph is structurally hollow, in three distinct layers:

1. **Parser (edge production, semsource-owned).** `source/ast/golang/parser.go`
   `callNameToEntityID` resolves an unqualified call by building an ID against the *caller's*
   file path — correct only when the callee lives in the same file; a sibling-file callee
   yields an inert ID that matches no entity. Package-qualified calls (`pkg.Fn()`) become
   `external:<importPath>.<Fn>` markers even when the import path is this repo's own module.
   The sibling-scan machinery to fix this already exists for type references: `imports.go`
   `packageTypes` scans a package directory once (cached behind a name+size+mtime signature)
   and maps names to their defining file and kind (#46).

2. **Seed resolution (framework recall, product selection).** The graph-index NAME_INDEX
   keys on the case-folded name *by design* ("folding the key gives case-insensitive
   recall; the original-case name rides in the stored value for exact-case ranking").
   `Engine.Fuse` then feeds **every** resolved entity into `computeImpact` — so unexported
   lookalikes (`systemSlug` ×2) entered the `SystemSlug` closure. Case-exactness is a
   product policy, and the product owns the seam: code-context constructs the
   `fusion.RetrievalClient` (`fusionnats.New`) it hands the engine.

3. **Impact response shape (framework facet).** `fusion.Impact` is `{nodes, files,
   truncated}` — counts only. But the engine's *relations* facet already names neighbors:
   `WantRelations` expands each seed node's reverse roles (`caller`, `extended_by`,
   `implemented_by`, `referenced_by`, `embedded_by`) into `Ref{Name, Path, Line}`, capped at
   `maxRelationsPerNode` (12) per role. The lens (`source/fusion/lens/code`) and the verb's
   default `Want` set are both semsource-owned.

Constraint: no wrong edges, ever. `external:` markers and inert IDs are honest; a guessed
resolution is not. And the substrate boundary holds: closure computation stays in the
semstreams engine; semsource changes edge production, lens/want configuration, and its own
client wrapper.

## Goals / Non-Goals

**Goals:**

- Unqualified Go calls resolve to sibling-file definitions in the same package (byte-matching
  the definition's entity ID).
- Package-qualified Go calls whose import path maps into the same indexed source root resolve
  to the defining entity; genuinely external imports keep `external:` markers.
- Symbol-mode seed resolution for context/callers/callees/impact is byte-exact on the display
  name; lookalikes contribute nothing.
- The impact response names at least the direct dependents of each seed (bounded, documented).
- Q7 (SystemSlug impact) re-runs and grades correct; a SanitizeInstance-style cross-package
  case is pinned by an integration test.

**Non-Goals:**

- Method-call resolution (`x.Method()` where `x` is a value) — needs type checking; stays
  inert as today.
- Qualified *type* references (`pkg.Type` in signatures/fields) — this change is calls;
  cross-package type-reference parity is a follow-up if graded evidence demands it.
- Type conversions (`pkg.Type(x)` parses as a CallExpr) — the funcs scan won't find a
  `FuncDecl`, so they stay `external:`/inert; conservative and honest.
- Func-typed vars (`var Fn = func(...)`) as callees — not `FuncDecl`s, stay unresolved.
- Per-seed impact closures or framework Impact-facet changes (ask filed instead, see D6).

## Decisions

### D1: One package scan serves types and funcs

Extend the existing per-directory scan (`imports.go`) so `pkgTypesEntry` carries both the
type map and a `funcs map[string]string` (name → defining relPath), harvested from
`FuncDecl`s with `Recv == nil` in the same single `parser.ParseFile(..., SkipObjectResolution)`
pass. Same cache, same name+size+mtime signature, same `_test.go` exclusion. The scan is
refactored to be addressable by directory (not just "directory of this file") so D2 can reuse
it for arbitrary in-repo packages.

`callNameToEntityID` then resolves an unqualified call through the funcs map first: found →
`NewCodeEntity(..., TypeFunction, name, definingRelPath).ID` (the same-file case degenerates
to today's behavior, so this is a strict superset); not found → current fallback (inert if
undefined — dot imports and shadowed names keep producing no edge rather than a wrong one).

*Alternative considered:* a separate `pkgFuncsCache`. Rejected — it would double the
directory parses and duplicate the signature/invalidation logic for no isolation benefit.

### D2: In-repo import mapping via nearest go.mod, verified by the defining scan

For a qualified call whose alias resolves in `importMap`, find the nearest `go.mod` walking
up from the *calling file's* directory to `repoRoot` (per-directory cache on the Parser,
validated by a size+mtime signature of the `go.mod` so a module rename during watch doesn't
serve stale mappings). If `importPath == modulePath` or has `modulePath + "/"` as a prefix,
map it to the corresponding directory under the module root and look up the callee in that
directory's funcs scan (D1):

- found → the definition's entity ID (byte-match: same org/project/repoRoot, defining
  relPath, `TypeFunction`);
- not found (excluded dir, generated-elsewhere, func-typed var, nonexistent) → the
  `external:` marker exactly as today.

Resolution *requires* the defining `FuncDecl` to be present in the scanned package — that is
the no-guessed-edges guarantee. Nested modules are safe by construction: a nested module's
import path either matches the root prefix and maps to the true directory, or doesn't match
and stays external.

*Alternative considered:* reading only `repoRoot/go.mod`. Rejected — monorepos with Go in a
subdirectory would silently lose all cross-package resolution; walk-up costs one cached stat.

### D3: Local-object guard before treating a selector as package-qualified

`extractFunctionCalls` sees `u.Name()` and `pkg.Fn()` identically. Today a local variable
shadowing an import alias merely produces a bogus-but-inert `external:` marker; with D2 it
could produce a *wrong real edge*. The main parse does not use `SkipObjectResolution`, so the
parser's (deprecated-but-functional) object resolution links idents to same-file
declarations: when the selector base `*goast.Ident` has `Obj != nil` it is a local
var/param/named declaration, not a package — skip import lookup and keep the method-call
path. A package-level shadow in a *sibling* file leaves `Obj == nil` and can still slip
through; accepted as ultra-rare (requires alias collision AND a same-named exported func in
the target package) and documented here.

### D4: Exact-match symbol seeds via a product-side RetrievalClient decorator

Wrap the `fusion.RetrievalClient` code-context already constructs with a decorator that, for
`ResolveModeSymbol` only, batch-fetches the resolved IDs (`Entities`, ≤ resolveLimit=40) and
keeps only entities whose display name (`DcTitle` — the bare identifier for functions,
methods, and types alike) equals the query byte-exactly. NL and prefix modes pass through
untouched. Zero survivors → the engine's normal miss path (ready+absent with `did_you_mean`)
— for a case-sloppy query that is the honest answer, not a lookalike closure.

Verb split: context/callers/callees/impact fuse through the exact-match engine; **search and
file keep folded recall** (search is a discovery surface; case-insensitive recall there is a
feature, and #84's ranking work is what fixed its grades). Two engine values built at Start
over the two clients; `fuse()` picks by verb.

*Alternative considered:* filtering framework-side (NAME_INDEX or engine). Rejected — the
folded index is a deliberate framework design with exact-case reserved for ranking;
exactness-as-a-filter is product policy and belongs at the product seam.

*Alternative considered:* comparing the query against the ID's instance segment. Rejected —
method instances are receiver-scoped (`NewScopedCodeEntity`), so bare method names would
never match; `DcTitle` is uniformly the bare identifier.

### D5: Dependents are named by the relations facet, not a new impact shape

Add `fusion.WantRelations` to the impact verb's default `Want` set. Each seed node then
carries named, located refs for every reverse role the lens defines — callers, subclasses,
implementers, referrers, embedders — computed by the engine, capped at 12 per role. Counts
(`impact.nodes/files/truncated`) are unchanged and stay closure-honest. The 12-per-role bound
is documented in the MCP `code_impact` tool description; closure-level truncation remains
labeled by `impact.truncated`. The fusion-gateway delta's "truncation-labeled" wording is
amended to match: bound documented, closure truncation labeled, per-role truncation markers
tracked upstream (D6).

*Alternative considered:* a product-side one-hop walk appending a `dependents` field to the
response envelope. Rejected — it duplicates the engine's edge-walking with a second code path
that can drift from the lens, for marginally more than WantRelations already provides.

### D6: Framework ask for first-class dependent naming

File in `docs/upstream/semsource → semstreams-asks.md`: the Impact facet naming direct
dependents (bounded, per-role truncation labeled) as a first-class field, so every fusion
consumer gets it without widening `Want`. Framework-shaped (generalizes beyond code), not
blocking: D4/D5 ship the product behavior now, and the graded bar (Q7) is met without it.

## Risks / Trade-offs

- [`goast.Object` is deprecated (Go 1.22+)] → still populated by `parser.ParseFile` without
  `SkipObjectResolution`; if a future Go release removes it, the guard degrades to today's
  behavior (selector treated as qualified), never to a new wrong edge — and the pinning tests
  catch behavioral drift.
- [Sibling-file package-level shadow of an import alias can still produce a wrong in-repo
  edge (D3 residual)] → requires a naming collision revive would flag plus a same-named
  exported func in the shadowed package; accepted and documented, no type-checker in scope.
- [Exact-match filter costs one extra `Entities` batch per symbol resolve] → bounded by
  resolveLimit (40), one NATS round-trip; the engine re-fetches the survivors (simple over
  clever; a pass-through cache is a later optimization if profiles demand).
- [Search behavior diverges from context/impact on case] → intentional (D4 verb split);
  documented in the spec delta so the asymmetry is contract, not accident.
- [Existing graphs lack the new edges until re-indexed] → periodic reindex emits them;
  release notes state that impact answers improve after the first reindex.
- [`WantRelations` on impact grows the default response] → bounded (12/role × the lens's
  roles × ≤40 ranked seeds, display-budgeted by the engine); callers who want lean counts can
  still pass an explicit `Want: ["impact"]`.

## Migration Plan

Additive, no wire or ID breaks: new call edges appear on reindex; the impact response gains
relations it could already carry on request; seed filtering tightens symbol-mode answers for
four verbs. Rollback = revert; no data migration either way.

## Open Questions

- None blocking. If the graded re-run shows method-call recall is the next dominant miss
  class, that becomes its own change (type-checked resolution is a different cost tier).
