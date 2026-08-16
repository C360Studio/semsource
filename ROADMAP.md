# SemSource Roadmap

SemSource is in **public beta**. The current public tag is `v1.0.0-beta.6`,
running on SemStreams `v1.0.0-beta.160` (the graph/foundation refactor
checkpoint). `main` has moved to SemStreams `v1.0.0-beta.161` (the
post-beta.160 reliability and lifecycle-control slice — caller-owned shutdown
contexts, restored WebSocket path configurability, attributable slow-consumer
errors); like beta.160 it mandates fresh NATS storage on adoption.

`v1.0.0-beta.6` is the call-graph-completeness and measurement release. It is
a **breaking upgrade**: SemStreams beta.160 mandates fresh NATS storage
(`docker compose down -v` and reseed — the graph re-derives from source), and
the raw WebSocket stream is now served at **`/ws`** (the former configurable
`/graph` path is gone; any configured `websocket_path` fails validation;
SemStreams #945 tracks the surface's return). What the release adds: call-graph
edges for Java instance receivers, Go function-typed values, and export-aware
TS/Svelte resolution, plus first C support — with the per-language coverage
contract published in the README and "a wrong edge is worse than a missing one"
now spec, not folklore; a CI determinism gate that ingests a fixed corpus twice
and diffs the entity set; gradle/pom entities fully labeled in search; and a
committed three-arm scorecard (grep floor / MCP / raw cosine, two corpora) whose
honest bounds — including where the graph does *not* win — live in
`scripts/scorecard/results/SUMMARY-rc-beta6.md`.

The promise is simple: SemSource deliberately scrapes the pile of source files
and turns it into a live, governed semantic knowledge graph (SKG). Humans,
agents, and operator UIs can ask what exists, what changed, where something is
used, and whether the graph is ready without each workflow rebuilding its own
parser, cache, and graph-write rules.

This page is an honest snapshot, not a dated commitment. Items are grouped by
confidence and dependency shape. The "why" behind durable choices lives in
[`docs/adr/`](docs/adr/); non-trivial work is specced first under
[`openspec/`](openspec/).

## Current Release-Candidate Shape

- **Graph-first ingestion** of code (Go, TypeScript/JavaScript, Java, Python,
  Svelte), Markdown/docs, config files, git/repo metadata, URLs, and media
  metadata by reference.
- **Governed SemStreams publishing** with deterministic 6-part entity IDs,
  source provenance, indexing intent, ownership bootstrap, and semantic envelopes
  on graph writes.
- **Agent-ready query surfaces**: MCP source tools, HTTP/NATS source manifest
  status, GraphQL through the UI profile, and deterministic fusion tools
  (`code_context`, `code_search`, `code_impact`, `doc_context`, `code_changes`).
- **Passage-level document retrieval**: documents are ingested as a navigational
  parent entity plus one entity per structural passage, each with its own
  verbatim body. A question about one paragraph matches that paragraph instead of
  an averaged whole file, and no document text is silently dropped for sitting
  past the substrate's embedding truncation limit.
- **Domain-scoped retrieval and ontology-aware ranking** so code, docs, versions,
  and public API signals are ranked by source role and graph semantics, not only
  lexical match.
- **Versioned source retention** with supersession lineage, current-version
  ranking, and `code_changes` diffs for added, removed, and changed symbols.
- **Optional SemSource workbench implementation**: the repurposed `ui` profile
  layers the SemSource-owned source/readiness/search workbench and an explicit
  Caddy allowlist over the unchanged core. Its first immutable multi-platform image
  and released-profile compatibility evidence are published.
- **Independent core and workbench proof**: `task core:smoke` proves the default
  profile never resolves UI artifacts; `task ui:smoke:dev` exercises the local
  workbench through Caddy with containerized Playwright at desktop and narrow
  widths. The released profile also passed browser smoke 6/6 against its exact first registry digest.
- **UI image publication mechanism**: pull requests validate UI/browser/clean-image and release
  verifier contracts without publishing. Trusted `main` and release-tag pushes can publish
  multi-platform `ghcr.io/c360studio/semsource-ui` images, then verify an exact immutable manifest
  through the released profile. The first trusted `main` workflow passed all six jobs for revision
  `25b2816d14a147c1d6eb7b54e40668b51ba3574a` and manifest
  `sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`.
- **Raw graph stream export** remains available in standalone mode for
  stream-oriented consumers such as federation, fan-out, and live UI updates. The
  primary governed read contract is still graph query/MCP/GraphQL.
- **Governed workbench graph drill-down integration** uses `want: ["graph"]` on the existing
  `POST /code-context/context` route. It preserves explicit typed facts, directed edges, supplied
  evidence, graph-local truncation, opaque handles, and coherent revision semantics without adding a
  SemSource graph endpoint or GraphQL dependency. The local WebGL/Sigma renderer has synchronized
  keyboard and screen-reader surfaces.

## Recently Shipped

- `v1.0.0-beta.6`: SemStreams beta.160 cutover (breaking; fresh storage +
  `/ws`), call-graph completeness across Go/TS/Svelte/Java/Python/C with the
  README coverage matrix, CI determinism gate, red-main alerting + PR-side UI
  smoke gate, MCP schema budget gate (7 KB measured, 8 KB cap), and the
  committed two-corpus three-arm retrieval scorecard at the RC commit.
- `v1.0.0-beta.5`: the audit-hardening release (2026-07-19 top-to-bottom
  audit) — no silent entity loss, honest readiness on every surface,
  verifiable source removal, a first-run wizard whose defaults actually
  ingest, and role-aware NL retrieval ranking.
- `v1.0.0-beta.4`: SemTeams UI profile, backend-owned health envelope, Playwright
  UI smoke, and SemStreams `v1.0.0-beta.144`.
- SemStreams [#490](https://github.com/C360Studio/semstreams/issues/490) was
  resolved and adopted; the full SemSource e2e gate now passes against the
  restart-safe WebSocket metrics fix.
- SemStreams [#533](https://github.com/C360Studio/semstreams/issues/533) was resolved by
  [PR #577](https://github.com/C360Studio/semstreams/pull/577), released in `v1.0.0-beta.153`, and
  adopted by SemSource through the existing code-context HTTP contract.
- README/product docs now describe WebSocket as a useful raw stream, not as the
  main query contract.

## Known Limits

- **Same-LAN deployment focus.** No built-in TLS/reverse-proxy hardening yet; run
  exposed deployments behind your own gateway.
- **Released workbench use is exact-pin only.** The first verified pin is
  `ghcr.io/c360studio/semsource-ui:sha-25b2816d14a147c1d6eb7b54e40668b51ba3574a@sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`.
  Mutable `latest` remains forbidden as release evidence. Local development uses the explicit
  `docker-compose.ui-dev.yml` override or `task ui:smoke:dev`.
- **Graph drill-down is bounded and capability-gated.** It is offered only when the structural index
  and graph contract are ready. Truncated, incoherent, zero-revision, or stale responses cannot erase
  or overwrite newer displayed state.
- **GraphQL capabilities are not advertised.** The beta.153 graph-facet adoption uses the existing
  code-context HTTP route and does not claim or require GraphQL projection coverage.
- **Large-corpus query readiness is still being hardened.** Real dogfooding found
  graph-index scale and readiness gaps in SemStreams; SemSource tracks them
  upstream instead of hiding them locally.
- **Media bytes are local-filestore backed.** Code/doc bodies use ObjectStore, but
  image/video/audio bytes are not yet shared ObjectStore payloads.
- **Passage chunking requires a graph rebuild, not a reindex.** The substrate
  cannot clear a stored body reference in place, so parents would keep their old
  whole-file bodies through an in-place reindex. See
  [`docs/migration/doc-passage-chunking.md`](docs/migration/doc-passage-chunking.md).
- **The raw WebSocket path is configurable again on `main`.** SemStreams
  beta.161 restored the path surface (#945); `websocket_path` is honored once
  more, with `/ws` remaining the default and documented contract path. On the
  beta.6 tag (beta.160) the path stays fixed to `/ws`.
- **C++ call edges are deliberately absent.** C is supported with a
  corpus-unique binding rule; C++ resolution was deferred whole rather than
  shipping guessed edges (see the README coverage matrix — a wrong edge is
  worse than a missing one).
- **Parent document entities are still embedded from their title.** The framework
  offers no way for a producer to opt an entity out of embedding (ADR-054 Phase 1
  is deliberately lenient), so a body-less navigational node still carries a
  title-only vector.
- **Only documents are split into passages.** Code is already indexed per symbol;
  web, config, and media sources are still one entity per artifact.
- **Version diffs do not detect renames.** A renamed or moved symbol currently
  appears as a removal plus an addition.
- **The graph is retention-first.** Safe, reference-complete deletion for genuine
  mistakes/churn is a future lifecycle feature, not the default behavior.

## Next

### Workbench Release Discipline

- Carry each verified immutable workbench pin into the matching SemSource release notes and keep
  future tag publications subject to the same registry/local/Compose/runtime evidence gate.
- Keep the former SemTeams profile note as historical evidence only; it creates
  no current SemSource compatibility obligation.

### Query Reliability And Scale

- Work with SemStreams on graph-index write amplification and query-index
  readiness so `phase: ready` means consumers can reliably query large repos.
- Track the GraphQL capabilities route/responder mismatch until the SemStreams
  contract is aligned, then advertise the surface.
- Validate tier-2 semantic/instruct summaries and local/global/summary search as
  first-class options rather than experimental tier toggles.

### Packaged Local Experience

- Keep the UI-free backend/MCP stack as SemSource's default deployment for
  embedded use by SemTeams, SemSpec, SemDragon, SemOps, and other consumers.
- Add a one-action local start that detects the project, launches pinned runtime
  artifacts, actively reports ingest/index/embedding readiness, and provides
  assistant connection instructions.
- Make the released path independent of sibling repository checkouts and a local
  JavaScript toolchain; UI activation remains explicit.
- Proposed follow-on `add-one-action-local-start` is not yet created or approved;
  it depends on a released workbench artifact.

### Project Knowledge Workbench

- Complete the opt-in SemSource workbench under this repository's `ui/`; do not
  add a second browser profile or a runtime/build dependency on a sibling UI.
- Keep the selectively ported search, evidence, responsive, and accessibility
  behavior locally owned and guarded by SemSource tests. Donor UIs are reference
  evidence, not canonical dependencies.
- Lead with source status, readiness, search, evidence, and bounded materialized
  project views; keep whole-graph visualization as investigation drill-down.
- Preserve a complete UI-free path: every workbench action must use a SemSource
  backend contract also available to non-UI automation.
- Planning is active under
  [`add-opt-in-source-workbench`](openspec/changes/add-opt-in-source-workbench/);
  governed graph drill-down is implemented against the adopted SemStreams #533 contract.

### Project Knowledge Interoperability

- Consume authored OKF as provenance-qualified explanatory knowledge without
  rewriting externally owned content.
- Export bounded materialized project views as OKF with source revision, graph
  watermark, evidence hash, producer/profile version, and derived classification.
- Preview and validate OKF bundles in the workbench; evaluate a self-contained
  offline HTML viewer after the bundle contract is stable.
- Keep materialized-view, OKF, workbench packaging, and one-action activation as
  coordinated but independently verifiable OpenSpec changes.
- Proposed follow-ons are `materialize-project-views` and `add-okf-interop-mvp`,
  neither created nor approved. OKF work follows the materialized-view contract.

### Code And Version Intelligence

- **Commit-level changesets**: connect commits/PRs to the symbols they edited,
  complementing today's version-to-version `code_changes`.
- **Rename/move detection**: correspond a symbol across versions instead of
  reporting remove plus add.
- **Dependency-version lifecycle**: refresh on dependency bumps, add supersession
  edges, and keep retention bounded by policy.

## Later

- **Multi-instance federation validation**: prove multiple SemSource instances
  produce identical `public.*` IDs that merge cleanly across deployments.
- **Sidecar branch lifecycle**: dynamic repo registration and branch-aware cleanup
  for tools that index many short-lived worktrees.
- **ObjectStore backing for media**: move image/video/audio payloads from local
  filestore to shared ObjectStore references.
- **Safe retraction/deletion**: referentially complete removal for mistakes and
  churn, depending on SemStreams index cleanup and cascade primitives.
- **Beyond same-LAN**: packaged TLS and deployment hardening for exposed
  deployments.

## Tell us what matters

Early-adopter feedback reorders this list. If a capability here (or one that isn't)
would unblock you, open an issue on
[GitHub](https://github.com/C360Studio/semsource/issues) — real consumer pressure
is how we prioritize.
