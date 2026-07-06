# Tasks: Domain-scoped fusion retrieval (ask #16)

## 1. Wire per-lens default scope (additive)

- [x] 1.1 In `processor/code-context/component.go`, capture the org in
      `NewComponent` from `deps.Platform.Org` into a new `org string` field on
      `Component`.
- [x] 1.2 Add per-lens domain constants: `codeScopeDomains = {"golang", "python",
      "typescript", "javascript", "java", "svelte"}` and `docScopeDomains =
      {"web"}`, with a comment tying them to the AST parser domains + a
      keep-in-sync note (design D3). Note the ID domain is NOT the parser
      registration name (Go registers as "go", stamps "golang").
- [x] 1.3 Add `func (c *Component) defaultScope() []string`: returns nil when
      `c.org == ""`; otherwise `{org}.{PlatformSemsource}.{domain}` for each
      domain in the lens's set (`code` vs `docs` by `c.lensKind`). Uses
      `entityid.PlatformSemsource` for the platform segment (design D2).
- [x] 1.4 In `serve`, after decoding `req` and before `Fuse`, set
      `req.Scope = c.defaultScope()` only when `len(req.Scope) == 0` (respect a
      caller-provided scope — design D1).

## 2. Tests

- [x] 2.1 Unit (`processor/code-context/scope_test.go`): `TestDefaultScope`
      (table-driven) — `defaultScope()` returns the exact `web` prefix for `docs`,
      the six code-language prefixes for `code`, and nil when org is empty.
      `TestServeDefaultsScopeWhenEmpty` / `TestServeRespectsCallerScope` /
      `TestServeNoScopeWithoutOrg` drive the REAL engine over a Scope-capturing
      graph, asserting the value that reaches `Resolve`. `TestNewComponentCaptures
      Org` proves org comes from `deps.Platform.Org`.
- [x] 2.2 Integration (`//go:build integration`,
      `processor/code-context/scope_integration_test.go`,
      `TestIntegration_DomainScopedRetrieval_OnTheWire`): over REAL NATS, a
      component driving the REAL fusion engine + REAL fusionnats client emits the
      per-lens default scope onto the actual `graph.query.semantic` request
      (`docs`→web, `code`→six langs) and passes a caller scope through verbatim.
      Deterministic (stubs status+semantic; no embedding subsystem). PASS with
      `-race`. **Boundary note:** the scope FILTER application (candidate
      prefix-match in `graph-embedding.findSimilarEntities` /
      `graph.MatchesAnyIDPrefix`) is framework code — validated in semstreams and
      the live httpx dogfood — so this test owns the semsource seam (correct scope
      selected per lens, reaching the wire), matching the harness convention of
      keeping the NL path out of the governance stack ("validated separately").

## 3. Docs

- [x] 3.1 In `docs/upstream/semstreams-asks.md` #16, marked **RESOLVED (beta.141)
      + ADOPTED**, noting the adoption site, org sourced from platform identity
      (single-org invariant), and the test boundary.

## 4. Gates

- [x] 4.1 `go build ./...`, `go test ./...` green; `go test -tags=integration
      ./processor/code-context/ ./internal/governance/` green (code-context
      integration PASS with `-race`).
- [x] 4.2 `task lint` zero warnings (revive v1.15.0), gofmt, go vet.
- [x] 4.3 `openspec validate domain-scoped-fusion-retrieval` green; `/opsx:verify`
      before archive.
