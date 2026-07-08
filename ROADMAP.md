# SemSource Roadmap

SemSource is in **public beta**. The current public tag is `v1.0.0-beta.4`,
running on SemStreams `v1.0.0-beta.144`.

The promise is simple: point SemSource at a project and it maintains a governed
source graph that agents and operator UIs can trust. Consumers can ask what
exists, what changed, where something is used, and whether the graph is ready
without scraping a workspace or re-parsing the same files.

This page is an honest snapshot, not a dated commitment. Items are grouped by
confidence and dependency shape. The "why" behind durable choices lives in
[`docs/adr/`](docs/adr/); non-trivial work is specced first under
[`openspec/`](openspec/).

## In The Beta Today

- **Graph-first ingestion** of code (Go, TypeScript/JavaScript, Java, Python,
  Svelte), Markdown/docs, config files, git/repo metadata, URLs, and media
  metadata by reference.
- **Governed SemStreams publishing** with deterministic 6-part entity IDs,
  source provenance, indexing intent, ownership bootstrap, and semantic envelopes
  on graph writes.
- **Agent-ready query surfaces**: MCP source tools, HTTP/NATS source manifest
  status, GraphQL through the UI profile, and deterministic fusion tools
  (`code_context`, `code_search`, `code_impact`, `doc_context`, `code_changes`).
- **Domain-scoped retrieval and ontology-aware ranking** so code, docs, versions,
  and public API signals are ranked by source role and graph semantics, not only
  lexical match.
- **Versioned source retention** with supersession lineage, current-version
  ranking, and `code_changes` diffs for added, removed, and changed symbols.
- **SemTeams UI profile**: `docker compose --profile ui up` adds the SemTeams UI
  checkout, Caddy on `:3000`, `/health`, `/source-manifest/*`, `/graphql`, and
  the raw `/graph` stream behind one origin.
- **Light UI smoke**: `task ui:smoke` starts the UI profile, runs Playwright
  against `/health`, `/source-manifest/status`, `/graphql`, and the UI shell, then
  tears the stack down.
- **Raw graph stream export** remains available in standalone mode for
  stream-oriented consumers such as federation, fan-out, and live UI updates. The
  primary governed read contract is still graph query/MCP/GraphQL.

## Recently Shipped

- `v1.0.0-beta.4`: SemTeams UI profile, backend-owned health envelope, Playwright
  UI smoke, and SemStreams `v1.0.0-beta.144`.
- SemStreams [#490](https://github.com/C360Studio/semstreams/issues/490) was
  resolved and adopted; the full SemSource e2e gate now passes against the
  restart-safe WebSocket metrics fix.
- README/product docs now describe WebSocket as a useful raw stream, not as the
  main query contract.

## Known Limits

- **Same-LAN deployment focus.** No built-in TLS/reverse-proxy hardening yet; run
  exposed deployments behind your own gateway.
- **UI profile depends on a sibling checkout.** The default `UI_CONTEXT` is
  `../semteams/ui`; the profile is SemSource-owned plumbing, while SemTeams UI
  owns app behavior and SemTeams-only routes.
- **SemTeams-only UI routes are best-effort here.** SemSource proves the shared
  source/graph/status path; SemTeams UI tracks graceful degradation for routes
  such as `/teams-dispatch/*`.
- **GraphQL capabilities are not advertised.** SemStreams beta.144 still routes a
  GraphQL capabilities query to `graph.query.capabilities`, but the graph-query
  responder is not registered yet.
- **Large-corpus query readiness is still being hardened.** Real dogfooding found
  graph-index scale and readiness gaps in SemStreams; SemSource tracks them
  upstream instead of hiding them locally.
- **Media bytes are local-filestore backed.** Code/doc bodies use ObjectStore, but
  image/video/audio bytes are not yet shared ObjectStore payloads.
- **Version diffs do not detect renames.** A renamed or moved symbol currently
  appears as a removal plus an addition.
- **The graph is retention-first.** Safe, reference-complete deletion for genuine
  mistakes/churn is a future lifecycle feature, not the default behavior.

## Next

### Operator UI Confidence

- Broaden UI-profile smoke only where SemSource owns the contract: source status,
  graph gateway reachability, search/readiness signals, and raw stream plumbing.
- Keep SemTeams UI feedback in `docs/integration/semteams-ui-profile-feedback.md`
  and upstream issues, without absorbing product routes into SemSource.
- Make readiness and source-manifest signals easier for UIs to display without
  knowing SemStreams internals.

### Query Reliability And Scale

- Work with SemStreams on graph-index write amplification and query-index
  readiness so `phase: ready` means consumers can reliably query large repos.
- Track the GraphQL capabilities route/responder mismatch until the SemStreams
  contract is aligned, then advertise the surface.
- Validate tier-2 semantic/instruct summaries and local/global/summary search as
  first-class options rather than experimental tier toggles.

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
