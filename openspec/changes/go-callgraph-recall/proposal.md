# Proposal: Go Call-Graph Recall

**Priority: P1** — the biggest single lever for the "answers grep cannot" value claim

## Why

For Go — the language of SemSource's first users — the call graph is structurally hollow
(code-verified in the audit, pinned by the parser's own tests):

1. Package-qualified calls (`pkg.Fn()`) become inert `external:<importPath>.<Fn>` markers **even
   for in-repo packages** (`source/ast/golang/parser.go` callNameToEntityID ~480–514); unqualified
   calls resolve only within the same file; method calls are always inert. Cross-file same-package
   resolution is explicitly deferred. Consequence: `code_context` caller relations and
   `code_impact` closures miss nearly every real cross-package dependency. The intuitive blast
   radius of `entityid.SanitizeInstance` (5 cross-package callers) is invisible. Python already
   resolves calls (#45); Go does not.
2. `code_impact` seed resolution is case-insensitive: lookalike unexported symbols from other
   packages (`systemSlug` ×2) entered the `SystemSlug` closure while the one genuine same-file
   dependent (`ScopedSystemSlug`) was missing (graded WRONG in the live interrogation).
3. `code_impact` returns counts only — `{nodes, files, truncated}` — never naming a dependent,
   despite the tool description promising "what depends on it".

## What Changes

- Go same-package cross-file call resolution: unqualified calls to symbols defined in sibling
  files of the same package resolve to real entity IDs (the mechanism `packageTypes` already
  proves for type references).
- Go cross-package in-repo call resolution: qualified calls whose import path maps to an indexed
  module in the same source root resolve to the defining entity (byte-matching the definition ID,
  per the #43/#44/#46 reference-ID parity approach); truly external imports remain honest
  `external:` markers.
- Impact seed resolution is exact-match (case-sensitive) on symbol names; ambiguity across
  packages is surfaced as multiple explicitly-labeled seeds, never silently merged.
- `code_impact` names dependents: the response includes at least the direct dependents (bounded,
  truncation-labeled), not just counts. If the fusion engine cannot hydrate dependent names with
  current facets, the framework ask goes to docs/upstream/semstreams-asks.md and the product ships
  the resolution fixes first.
- Acceptance: audit question Q7 (SystemSlug impact) re-runs and grades correct; a
  SanitizeInstance-style cross-package case gains a pinning test.

## Capabilities

### Modified Capabilities
- `code-call-graph`: "Function bodies emit resolved call edges" and "Imported callees resolve to
  their defining module" extend to Go same-package and in-repo cross-package calls;
  "Unresolvable and out-of-scope calls never produce a wrong edge" gains Go scenarios (external
  stays external; no guessed edges).
- `fusion-gateway`: impact response contract gains dependent naming + exact-match seed scenarios.

### New Capabilities
<!-- none — this lands entirely inside existing capability boundaries -->

## Impact

- `source/ast/golang/` (parser call resolution, imports mapping), `processor/code-context/`
  (impact verb), possible semstreams ask for dependent hydration in the impact facet.
- Re-index required for existing Go graphs to gain the new edges (periodic reindex handles it).
- Consumers: code_impact becomes real for Go; semspec/semdev agents get true blast radii.
- Boundary check: BFS/closure computation is semstreams fusion engine; SemSource owns edge
  production (this change) and lens/facet configuration. Dependent-naming may be framework-gated —
  tracked as an ask, not patched into the substrate.
