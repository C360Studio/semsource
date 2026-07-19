## 1. Package scan: types + funcs (D1)

- [x] 1.1 Extend `pkgTypesEntry` with `funcs map[string]string` (name → defining relPath),
      harvested from `Recv == nil` `FuncDecl`s in the same single directory parse; refactor
      the scan to be addressable by directory so D2 can reuse it. Proves:
      `TestPackageScan_FuncsHarvested` (sibling func mapped; method and `_test.go` func
      excluded; cache signature still invalidates on sibling edit).
- [x] 1.2 Resolve unqualified calls through the funcs map in `callNameToEntityID` — found →
      definition ID with the defining file's relPath; not found → current fallback. Proves:
      `TestGoParser_CrossFileSamePackageCall` (sibling-file call byte-matches the
      definition's entity ID; same-file behavior unchanged; undefined name stays inert).

## 2. In-repo cross-package resolution (D2 + D3)

- [x] 2.1 Nearest-go.mod module mapping: walk up from the calling file's directory to
      `repoRoot`, cache per directory with a size+mtime signature. Proves:
      `TestGoParser_ModuleMapping` (root module, nested module, no-go.mod → no mapping;
      stale cache invalidates on go.mod edit).
- [x] 2.2 Qualified in-repo calls resolve via the target directory's funcs scan; found →
      definition ID, not found → `external:` unchanged. Proves:
      `TestGoParser_CrossPackageInRepoCall` (in-repo qualified call byte-matches the
      definition; `strings.Contains` stays `external:`; conversion `pkg.Type(x)` stays
      unresolved).
- [x] 2.3 Local-object guard: a selector base ident with `Obj != nil` is never treated as a
      package qualifier. Proves: `TestGoParser_LocalVarShadowsImportAlias` (method call on a
      shadowing local emits no in-repo call edge).

## 3. Exact-match symbol seeds (D4)

- [x] 3.1 Product-side `RetrievalClient` decorator in `processor/code-context`: for
      `ResolveModeSymbol`, batch-fetch resolved IDs and keep only `DcTitle == query`
      byte-exact; NL/prefix pass through. Proves: `TestExactSeedDecorator` (lookalike
      filtered, method kept by bare name, zero survivors → empty → engine miss path,
      NL untouched).
- [x] 3.2 Verb split: context/callers/callees/impact fuse through the exact-match engine;
      search/file keep folded recall. Proves: `TestFuse_VerbEngineSelection` (impact drops
      the lookalike; search still resolves it).

## 4. Impact names dependents (D5 + D6)

- [x] 4.1 Add `WantRelations` to the impact verb's default `Want` set; update the MCP
      `code_impact` tool description with the dependents roles and the 12-per-role bound.
      Proves: `TestDefaultWants_ImpactIncludesRelations` + description assertion in the
      gateway's tool-list test.
- [x] 4.2 Amend the fusion-gateway delta wording per D5 (bound documented; closure
      truncation labeled via `impact.truncated`; per-role markers upstream) and re-run
      `openspec validate --all`.
- [x] 4.3 File the framework ask (first-class Impact dependent naming, per-role truncation
      labels) in `docs/upstream/semstreams-asks.md`, triaged framework-shaped.

## 5. Integration proof + acceptance

- [x] 5.1 Integration test over the real graph stack: index a fixture with a cross-package
      caller (SanitizeInstance-shaped), assert the impact closure includes the cross-package
      caller and the response names it in a reverse role. Proves:
      `TestIntegration_GoCallGraphImpact` in `internal/governance/`.
- [x] 5.2 Re-run graded Q7 (SystemSlug impact) against a live stack (isolated
      COMPOSE_PROJECT_NAME + high ports per the harness rule) and record the grade flip in
      the change; note reindex requirement in release notes/docs where impact behavior is
      described.

**Q7 acceptance evidence (2026-07-19, live compose stack `semsource-q7-verify` on
28080/24222/28222, branch build, mvp.json over the semsource repo):**
`POST /code-context/impact {"query":"SystemSlug"}` with `index.ready=true` returned exactly
one seed — `SystemSlug` at `entityid/entityid.go` (no case-lookalike seeds) — with
`impact = {nodes: 156, files: 43, truncated: false}` (audit baseline: same-file-only, closure
hollow, lookalike seeds merged) and NAMED callers including `ScopedSystemSlug` (the dependent
Q7 graded as missing), cross-package callers `pathToSystemSlug`, `ingestFile`,
`astComponentConfig`, `removeBranchComponents`, and the genuine wrapper-caller `systemSlug`
(a true dependent, not a leaked seed). Q7 grades correct. Reindex note added to
`docs/integration/mcp-quickstart.md`.
