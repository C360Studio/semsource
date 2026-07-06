# Design: domain-scoped-fusion-retrieval

## Context

`processor/code-context/component.go` decodes a `fusion.Request` from the request
body (`serve`, both NATS and HTTP funnel here), picks the lens by `c.lensKind`
(`lensFor()`), and calls `c.engine.Fuse(ctx, req, lens)`. beta.141 flows
`req.Scope` through the engine to `graph.Resolve(ctx, ResolveQuery{Query, Mode,
Scope, Limit})`, where it filters NL seed resolution to entities whose ID starts
with one of the OR-matched prefixes. Symbol/prefix resolve modes ignore it.

## Decisions

### D1 — Scope is defaulted in `serve`, not the lens

`serve` is the single choke point for both transports (MCP `docContext` →
`docs.v1.context`; HTTP/NATS direct). The `fusion.Lens` interface has no scope
hook (upstream kept scope caller-set on the Request, deliberately — ADR-071), and
`lensFor()` returns a fresh `fusion.Lens` per request. So the default is applied
to `req.Scope` in `serve` keyed on `c.lensKind`, right before `Fuse`. One edit
covers every path.

Precedence: **caller `req.Scope` (non-empty) > per-lens default > none.** We only
default when `len(req.Scope) == 0`, so a caller can still request any scope,
including a cross-domain one, by setting it explicitly.

### D2 — The `{org}` segment comes free from platform identity (no config)

The scope prefix is `{org}.{platform}.{domain}` because entity IDs are
`{org}.{platform}.{domain}.{system}.{type}.{instance}`. The memory-era open
question was "how does the gateway learn the org." Reading the wiring dissolves it:

- `namespace` is a **required** top-level config field ("org identifier used in
  entity ID construction"), and `run.go` forces `Org = cfg.Namespace` onto every
  source at all three spawn sites (`sourcespawn.Options.Org`), plus the HTTP/MCP
  add-source façade. So a deployment has **one** org for all entities — an
  enforced invariant, not an assumption.
- That same value reaches the component as `deps.Platform.Org`
  (`ServiceManager` platform `{Org: cfg.Namespace, Platform: "semsource"}` →
  `ComponentManager` → `deps.Platform`). Captured in `NewComponent`.

So: `org` from `deps.Platform.Org`; the platform segment from
`entityid.PlatformSemsource` (`"semsource"`, the literal every `entityid.Build`
uses — more authoritative than `deps.Platform.Platform`, though they are equal by
construction). No new config field, and the fix is correct-by-default with zero
operator action.

When `deps.Platform.Org` is empty (standalone/unit contexts with no platform
identity), `scopeFor` returns nil → no scope → today's unfiltered behavior. Safe
fallback, never a broken prefix like `.semsource.web`.

### D3 — Per-lens domain sets are an explicit, reviewed constant (not derived)

- `docs` lens → `["web"]`. Doc/prose entities are `{org}.semsource.web.…`
  (`handler/doc` type `doc`, `handler/url` type `page`).
- `code` lens → `["golang", "python", "typescript", "javascript", "java",
  "svelte"]` — the domain segments the AST parsers stamp on code entity IDs.

**Why not derive from `ast.DefaultRegistry.ListParsers()`:** the registered parser
*name* is not the ID *domain*. Go registers as `"go"` but stamps domain
`"golang"`; the TS parser registers `"typescript"` and `"javascript"` and stamps
either by extension. A derive would silently produce wrong prefixes. An explicit
list is co-located with a "keep in sync with the AST parsers" comment; a missing
language degrades (that language falls out of code NL scope) rather than crashing,
and is caught in dogfooding.

### D4 — Scope both lenses, not docs-only

The validated symptom is asymmetric (small doc set drowned by large code set), so
docs-only scoping would fix the measured case. We scope both anyway to honor
ADR-071's per-domain isolation: `code_context` should return code, not the odd
`web`/`config`/`git` entity that happens to embed near the query. The cost is the
D3 drift risk on the code list; accepted as bounded and low-severity.

### D5 — `code` lens covers only AST-parsed languages, not all code-ish domains

`config` (go.mod/package.json), `git` (commits/authors), and `media` are ingested
domains but are **not** in the code lens scope. `code_context` is about code
symbols; those domains have their own semantics and are not what a code NL query
wants. If a future need appears, broaden the list — it is one constant.

## Risks & mitigations

- **Drift (D3):** new language added to the AST parsers but not to the code scope
  list → that language silently excluded from `code_context` NL scope. Mitigation:
  the constant carries an explicit sync note next to it; dogfooding surfaces it.
- **Over-scoping a single-language repo:** listing all six code domains on a
  Python-only repo is harmless — OR-matched prefixes that match nothing simply do
  not contribute seeds.
- **No "unscoped NL" without an explicit Scope:** acceptable for MVP; a caller who
  wants cross-domain NL passes an explicit `Scope`. Revisit if a real use case
  appears (would be a config toggle, not a contract change).

## Alternatives considered

- **`scope_prefixes` / `org` config field** (the memory's option a): rejected —
  org is already known for free (D2), so a config field would be redundant and
  push ID-scheme knowledge onto operators, and it would not be correct-by-default.
- **Source-manifest-derived orgs** (option b): rejected — single-org invariant
  (D2) makes it unnecessary complexity.
- **`Lens.SeedFilter(entity) bool` post-retrieval hook** (upstream fix direction
  2): not available; the shipped hook is request `Scope`, which pushes the filter
  into the embedding search (cheaper than over-fetch-then-filter).
