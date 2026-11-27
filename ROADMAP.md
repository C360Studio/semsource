# SemSource Roadmap

SemSource is in **public beta**. The current public tag is `v1.0.0-beta.12`
(2026-11-26), the third SemEngine release, on SemEngine `v0.1.0-alpha.8` plus two
graph-index fixes (untagged `3d823c8`). It is
**breaking**: upgrading from beta.11 needs fresh storage and a re-ingest, and a
`semsource.json` that sets `streams` no longer loads. See
[v1.0.0-beta.12](#v100-beta12-dogfood-hardening) and the
[beta.12 adoption note](docs/integration/beta12-adoption-note.md).

The previous tags, `v1.0.0-beta.11`, `v1.0.0-beta.10` and `v1.0.0-beta.9`, are described below for reference.

`v1.0.0-beta.9` is the onboarding release — and the **first non-breaking
release in the beta line**: no substrate cutover, no fresh storage, no
reseed; upgrade in place. Its centerpiece closes v1-blocker #184:
[`docs/QUICKSTART.md`](docs/QUICKSTART.md) takes a new user from a repo (or
several) to `phase: ready` and a correct first query on the documented
commands alone — and the document is **executable truth**: CI extracts its
marked command blocks and runs them verbatim, in order, against public
fixtures (single-repo, and multi-repo with explicit `project`/`version`
identity plus the submodule dedup demonstration ADR-0012 makes true by
construction), so doc drift fails a build instead of a user. The decision on
the one-action launcher is recorded on #184: **Docker Compose is the v1
launcher**; a bespoke launcher is deferred past v1 with the quickstart as
its executable skeleton. Alongside it, status honesty deepened (#188): the
internal status report is now ONE shared contract (`internal/sourcestatus`)
decoded strictly — the previously dropped per-source `backpressure` and
`boundaries_skipped` signals reach every status surface, so "slow vs
stalled" is answerable without logs, and the quickstart's troubleshooting is
keyed entirely to signals that exist. `semsource add ast|repo` gained
`--project`/`--version` (#189), making the version-diff registration shape a
two-command affair. With #185 (submodules, beta.8) and #184 both closed, the
remaining v1 gate at beta.9 was external: SemStreams reaching v1. The substrate
is now SemEngine, and this page does not yet restate that gate. The beta.9
scorecard stood at OSH v2 11/13 (the two reds name semstreams#823, still open)
and dogfood loose band 33/33 (`scripts/scorecard/results/`); both are
SemStreams-pin measurements.

The promise is simple: SemSource deliberately scrapes the pile of source files
and turns it into a live, governed semantic knowledge graph (SKG). Humans,
agents, and operator UIs can ask what exists, what changed, where something is
used, and whether the graph is ready without each workflow rebuilding its own
parser, cache, and graph-write rules.

This page is an honest snapshot, not a dated commitment. Items are grouped by
confidence and dependency shape. The "why" behind durable choices lives in
[`docs/adr/`](docs/adr/); non-trivial work is specced first under
[`openspec/`](openspec/).

## v1.0.0-beta.12: Dogfood Hardening

The third SemEngine release, on SemEngine `v0.1.0-alpha.8` plus two graph-index
fixes, pinned at untagged commit `3d823c8` (#428). It carries the
fixes from dogfooding beta.11 (#391) and is tracked on
[#408](https://github.com/C360Studio/semsource/issues/408). Like beta.11 it
cannot be upgraded in place. Consumers should read the
[beta.12 adoption note](docs/integration/beta12-adoption-note.md) first.

- **A third clean storage break, confined to config files.** Config dependency
  and image IDs no longer carry a version or tag (#393). A version bump updates
  one entity in place, several versions or tags in one manifest fold into one
  entity with multi-valued facts, and a Gradle `:classifier` or `@ext` stays in
  the ID so a jar and its natives variant remain two entities. IDs are never
  aliased or rewritten, so upgrading from beta.11 means fresh NATS storage and a
  re-ingest: `docker compose down -v`, then `docker compose up -d`. Code, doc,
  git, url and media IDs are unchanged.
- **The `streams` config key is removed** (#395). A `semsource.json` that sets
  it fails to load with `unknown field "streams"`. `graph_stream` replaces it:
  `{"posture": "durable"|"ephemeral", "max_bytes": <optional>}`. Subjects,
  workqueue retention, discard `new`, the one-hour age bound and one replica are
  fixed for both postures.
- **GRAPH uses file storage by default.** Under `durable`, a NATS restart no
  longer loses facts that were published but not yet applied; `ephemeral` keeps
  beta.11's memory storage. An integration test restarts the same NATS container:
  graph-ingest applies every message after the restart under `durable`, and the
  messages are lost under `ephemeral`. Measured cost on an 8,439-entity
  self-seed: the median drain is +0.5 s (+3.8%) against `ephemeral`, on one
  machine with three seeds per posture and a run-to-run spread larger than the
  difference. A NATS server still holding the beta.11 memory GRAPH stream
  refuses boot on the storage drift; `docker compose down -v` (already required)
  or `nats stream rm GRAPH` clears it.
- **Config-file entities go stale with their manifest** (#393). Deleting a
  manifest marks every entity it declared stale and re-creating it clears the
  mark; this runs for `watch: true` sources only. A dependency a manifest stops
  declaring is kept and not yet marked stale
  ([#421](https://github.com/C360Studio/semsource/issues/421)).
- **Fixes that need no action.**
  - #399: a full GRAPH stream at boot no longer crash-loops; the manifest and
    predicate broadcasts retry in the background.
  - #400: a `repo` source's `index_interval` reaches its `ast` child.
  - #401: the periodic reindex skips unchanged folder and repo publishes.
  - #402: the core image is also tagged `v1.0.0-beta.N`, as the UI image is.
  - #405: ADR-0003 no longer says runtime add/remove republishes the manifest.
  - #406: Compose gives semsource a 40 s stop grace period; a graceful stop
    under an index backlog still exits 1 (upstream, semengine#768).
  - #419: watched paths flush once they go quiet, capped at five delays.
  - #424: a config-file lifecycle pass retries on `revision_mismatch`.
  - #377: the fusion tools expose the substrate's `merge` answer.
  - Also internal: the `task verify` target (#410) and an image-source test
    fixture race (#412).
- **Known open items.** After a warm restart of a large corpus, graph-index can
  take about 45 minutes to reach `index.ready` (#403, upstream: semengine#750 and
  semengine#765). Graceful stop exit codes under a backlog wait for semengine#768
  (#406).
  Marking a removed dependency stale is #421.
- **Evidence.** The acceptance proofs are the per-PR tests and gates. The
  scorecard regrade on the released beta.12 image (#387) and the dogfood
  measurement on the mini (#407) follow the tag.

## v1.0.0-beta.11: Identity Hardening

The second SemEngine release, on SemEngine `v0.1.0-alpha.8` (#378). Like beta.10
it cannot be upgraded in place. Published 2026-11-25 and tracked on
[#389](https://github.com/C360Studio/semsource/issues/389). Consumers should
read the [beta.11 adoption note](docs/integration/beta11-adoption-note.md)
first.

- **A second clean storage break.** Entity IDs change again and the GRAPH
  stream's retention changes, so upgrading from beta.10 means fresh NATS storage
  and a re-ingest: `docker compose down -v`, then `docker compose up -d`. IDs are
  never aliased or rewritten. The identity breaks since beta.10: one ID per code
  declaration (#329, then Python, C and C++ in #338); TypeScript
  `declare module` members keyed by their raw specifier (#340); config-file
  entities keyed by their declaring file (#348), one entity per same-coordinate
  declaration within a manifest with image IDs that hash the raw reference
  (#375), and POM `<module>` IDs that hash the raw name (#384). The
  [migration note](docs/migration/ast-entity-identity.md) covers the code IDs.
- **Configs whose sources share an entity-ID system no longer boot.** ast, docs
  and object-store roots (#349), media roots of one kind (#366), git sources
  (#354) and config-file roots (#348) that would mint IDs under one org and
  system are refused by `semsource validate`, `semsource run`, boot and runtime
  `add_source`. The error names both sources and the remedy. A persisted entry
  that blocks boot comes out with `semsource remove --manifest <instance>` while
  the service is stopped.
- **Substrate.** SemEngine `v0.1.0-alpha.8` at Tier 0. Tier 1 neural search was
  qualified on alpha.7 with semembed 0.2.1 (#327) and stays opt-in
  (`graph.semantic_search`). alpha.8 requires NATS `max_pending` 512 MiB: the
  bundled Compose NATS sets it (#357), and an operator running their own NATS
  must too (`configs/nats.conf`).
- **A GRAPH stream that drains.** GRAPH uses workqueue retention, so a message
  goes once graph-ingest has applied it (#160). A SemSource-only restart within
  the hour no longer refills the stream and leaves a large corpus `degraded`;
  the OSH warm-restart proof passed on #394's build (#388). A NATS server still
  holding the beta.10 stream refuses boot and names the retention drift. GRAPH
  is transport only, and external JetStream consumers on it are unsupported.
- **Re-ingest removes what a source stopped emitting** (#336). The ast, doc, url,
  image, audio, video, object-store and config-file sources opt in (#346, #384);
  git does not. It acts only on content the process knows changed, never on the
  first pass after a start.
- **Consumer contracts.** Natural-language resolves are refused as
  `not_enabled`, HTTP 501, when no search index runs, on every fusion verb and
  lens (#291). Source status gains a per-source `id_collisions` count (#329).
  Ast sources accept `index_vendored_js` (#329).
- **Evidence.** The acceptance floor is still the graded scorecard. After the
  AST identity change, Tier 0 on alpha.6 scored R1 36/36 rank-stable and OSH
  13/13, with every dogfood verdict equal to beta.10 (#324,
  [record](scripts/scorecard/results/SUMMARY-fix329-tier0.md)). The Tier 1 arm
  on alpha.7 with semembed 0.2.1 scored dogfood 33/36, OSH 13/13, setup03a's
  neural lane 72/72 and provider faults 15/15
  ([record](scripts/scorecard/results/SUMMARY-tier1-alpha7.md)). The regrade
  moved to the beta.12 image (#387).
- **Also in this tag.** Java calls inside anonymous and local class bodies no
  longer bind to the enclosing class (#339); AST folder `contains` sets stay
  complete under watch edits (#347); a git source keeps its configured URL when
  it re-ingests after a HEAD change (#350), and a failed re-ingest is retried
  (#355); media collision refusals name the media remedy (#369); the summary
  status no longer races on its degraded flag (#278).

## v1.0.0-beta.10: The First SemEngine Release

This is the substrate cutover from SemStreams to SemEngine
([ADR-0013](docs/adr/0013-substrate-cutover-to-semengine.md), tracked on
[#237](https://github.com/C360Studio/semsource/issues/237)). Unlike beta.9 it
cannot be upgraded in place. Consumers should read the
[SemEngine adoption note](docs/integration/semengine-adoption-note.md) first.

- **A clean storage break.** Entity IDs reorder to
  `{org}.{platform}.{system}.{domain}.{type}.{instance}` and SemEngine mints and
  retains the platform authority, so existing IDs are never aliased or
  rewritten. Upgrading means fresh NATS storage and a re-ingest from source;
  for Compose, `docker compose down -v` then `docker compose up -d`. SemSource
  refuses to boot over a populated `semstreams_config_<org>_<stem>` bucket. Do
  not delete that bucket by hand to get past the refusal.
- **Tier 0 on SemEngine `v0.1.0-alpha.6`, embeddings off.** Structural queries
  plus tier 0 word search (`graph-lexical`, on by default through
  `graph.lexical_search`). Neural search (Tier 1,
  [#234](https://github.com/C360Studio/semsource/issues/234)) and LLM community
  summaries ([semengine#485](https://github.com/C360Studio/semengine/issues/485))
  qualify separately, after the tag.
- **Consumer contracts change.** The `/ws` export, status figures, MCP
  `graph_search`, the fusion envelope (contract v2), disabled-capability errors
  (`not_enabled`, HTTP 501), runtime `add_source`/`remove_source` (applied after
  the next restart), and git credentials (`SEMSOURCE_GIT_TOKEN` only) change.
  A deployment that ran `main` with `git_token` in `semsource.json` before #309
  should rotate the token: the config bucket keeps earlier revisions.
  Tool names, the `graph.query.*` verbs SemSource serves, the
  `/source-manifest/status` readiness gate (`phase: "ready"`), and the GraphQL
  route do not. The table in
  [ADR-0013](docs/adr/0013-substrate-cutover-to-semengine.md#consumer-contracts-v100-beta9--the-first-semengine-tag)
  is the before/after list.
- **Evidence.** The acceptance floor is the Tier 0 graded scorecard, not "tests
  pass". On alpha.6: dogfood 32/36, OSH 12/13, R1 36/36 rank-stable, and the
  setup03a BM25 lane passing
  ([port ledger](docs/upstream/semengine-port-ledger.md)). On the released
  beta.10 image the verdicts and bytes match, but R1 is 35/36: two tail results
  of one correctly answered question swap with ingestion order (#324;
  [record](scripts/scorecard/results/SUMMARY-beta10-tier0.md)). The question sets are
  newer than the ones behind the beta.9 numbers above, so the two are not
  comparable.
- **Also in this tag.** Boot-time branch and submodule discovery with a
  restart-required signal (#216); `semsource.json` edits apply on a retained
  store through an operator `config_revision` (#266); `semsource add <type>`
  applies its flags or fails (#267); a source's staleness sweep touches only
  its own entities (#286); file-path queries in the code and docs lenses
  (#299); S3-compatible document sources (#203); honest delivery figures and
  loss-aware readiness (#195); the workbench image now builds arm64 natively
  (#219).

## Current Release-Candidate Shape

- **Graph-first ingestion** of code (Go, TypeScript/JavaScript, Java, Python,
  Svelte), Markdown/docs, config files, git/repo metadata, URLs, and media
  metadata by reference.
- **Governed SemEngine publishing** with deterministic 6-part entity IDs,
  source provenance, indexing intent, ownership bootstrap, and semantic envelopes
  on graph writes.
- **Agent-ready query surfaces**: MCP source tools, HTTP/NATS source manifest
  status, GraphQL through the UI profile, and deterministic fusion tools
  (`code_context`, `code_search`, `code_impact`, `doc_context`, `code_changes`).
- **An executable onboarding quickstart**: the documented zero-to-first-query
  path ([docs/QUICKSTART.md](docs/QUICKSTART.md)) is extracted and run
  verbatim by CI against public fixtures — single-repo and multi-repo with
  explicit identity and submodule dedup — with troubleshooting keyed only to
  signals the status surface actually serves.
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
- **Live-state WebSocket export** (`/ws`) remains available in standalone mode for
  live-state consumers such as live UI updates. It watches `ENTITY_STATES` and
  emits convergent `put`/`delete`/`poisoned`/`synced` frames, not the raw event
  stream ([ADR-0001](docs/adr/0001-websocket-exports-raw-entities.md), amended by
  ADR-0013). The primary governed read contract is still graph query/MCP/GraphQL.
- **Governed workbench graph drill-down integration** uses `want: ["graph"]` on the existing
  `POST /code-context/context` route. It preserves explicit typed facts, directed edges, supplied
  evidence, graph-local truncation, opaque handles, and coherent revision semantics without adding a
  SemSource graph endpoint or GraphQL dependency. The local WebGL/Sigma renderer has synchronized
  keyboard and screen-reader surfaces.

## Recently Shipped

- `v1.0.0-beta.12` (breaking; fresh storage and re-ingest): the third
  SemEngine release, on `v0.1.0-alpha.8` plus untagged `3d823c8` (graph-index
  stop fix and warm-restart candidate, #406/#403). Config dependency and image IDs
  drop the version or tag (#393); deleting a manifest marks its entities stale
  (#393); the `streams` config key is replaced by `graph_stream` and GRAPH is
  file-backed by default (#395); a full GRAPH stream no longer crash-loops the
  boot (#399); a repo source's `index_interval` reaches its ast child (#400);
  the periodic reindex skips unchanged folder and repo publishes (#401); the
  core image is also tagged with the `v` prefix (#402); and the fusion tools
  expose `merge` (#377). See the
  [adoption note](docs/integration/beta12-adoption-note.md).
- `v1.0.0-beta.11` (breaking; fresh storage and re-ingest): the
  second SemEngine release, on `v0.1.0-alpha.8` (#378). One entity ID per code
  declaration (#329): Java methods and constructors carry a signature
  discriminator, locals and Go `init`/blank declarations are no longer
  entities, same-arity Java overload calls emit no edge, IIFE/AMD-wrapped JS
  and vendored twins index as files only, and the AST source reports any
  remaining duplicate as `id_collisions`. Python, C and C++ follow (#338):
  C/C++ prototypes, `#if` twins and redefined macros fold onto one entity,
  Python setters/deleters get distinct IDs, and `@overload` stubs give way to
  the implementation. TypeScript `declare module` members are keyed by their raw
  specifier (#340); config-file entities by their declaring file (#348, #375,
  #384). Sources that share an entity-ID system are refused (#349, #354, #366).
  GRAPH removes consumed messages (#160) and the NATS `max_pending` 512 MiB
  setting is required (#357). Re-ingest removes dropped predicates (#336);
  Tier 1 is qualified on alpha.7 (#327); nl resolves answer `not_enabled`
  without a search index (#291); R1 is rank-stable again (#324). See the
  [adoption note](docs/integration/beta11-adoption-note.md) and the
  [migration note](docs/migration/ast-entity-identity.md).
- `v1.0.0-beta.10`: the first SemEngine release (breaking; fresh storage and
  re-ingest) — Tier 0 on SemEngine `v0.1.0-alpha.6` with embeddings off, the
  reordered entity ID, `/ws` as `ENTITY_STATES` KV-watch frames, and the consumer
  contract changes listed in ADR-0013 and the
  [adoption note](docs/integration/semengine-adoption-note.md).
- `v1.0.0-beta.9`: the onboarding release (non-breaking; no reseed) — the
  executable quickstart (#184: `docs/QUICKSTART.md` command blocks run
  verbatim by CI against public fixtures, single- and multi-repo tracks
  incl. the submodule dedup demo; path-filtered Quickstart CI workflow;
  README walkthrough replaced by the link; D5 recorded: Compose is the v1
  launcher), the shared status-report contract (#188: nine duck-typed
  report copies collapsed into `internal/sourcestatus` with strict decode;
  `backpressure` + `boundaries_skipped` now on every status surface;
  signal-keyed troubleshooting), and CLI identity flags (#189:
  `add ast|repo --project/--version`).
- `v1.0.0-beta.8`: git submodule support (#185, ADR-0012) — recursive
  materialization on clone/pull with non-tip shallow fallback and per-source
  opt-out; canonical submodule identity (resolved-URL project + 12-hex
  gitlink-SHA version; cross-repo dedup by construction); per-path submodule
  states on every status surface; git-boundary skipping at every ingest walk;
  runtime expansion of per-submodule ast/docs/config components with
  parent+link-scoped instance names; host-scoped git auth header. Verified by
  hermetic + e2e + compose acceptance against a public dual-pin fixture;
  surfaced and defended against semstreams#986.
- `v1.0.0-beta.7`: SemStreams beta.161 cutover (breaking; fresh storage;
  caller-owned shutdown contexts; `websocket_path` restored), asset ingestion
  guards (#175: minified detection + symbol cap; plateau eliminated), typed
  publish-error classification with idempotent retries (#176: zero-loss broker
  pause and stream-constriction inductions), pre-publish seed liveness
  counters, attributed slow-consumer errors, and archived
  `ingest-observability` at 35/35 with live evidence.
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
  graph-index scale and readiness gaps on SemStreams; the Tier 0 graded runs on
  SemEngine alpha.6 (OSH, 29,422 entities) are the current scale evidence.
  SemSource files substrate gaps as SemEngine issues instead of hiding them
  locally.
- **Media sources are alpha.** `image`/`video`/`audio` ingestion is
  metadata-by-reference and functional, but has not been exercised recently
  and sits outside the current hardening focus; media bytes are also still
  local-filestore backed (not shared ObjectStore payloads). The hardened,
  continuously verified paths are code/docs/config/git/url.
- **Passage chunking requires a graph rebuild, not a reindex.** The substrate
  cannot clear a stored body reference in place, so parents would keep their old
  whole-file bodies through an in-place reindex. See
  [`docs/migration/doc-passage-chunking.md`](docs/migration/doc-passage-chunking.md).
- **The raw WebSocket path default is `/ws`.** `websocket_path` is honored
  again since beta.7 (SemStreams #945 restored the surface); `/ws` remains the
  default and documented contract path, and the ui profile's Caddy config
  assumes it.
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
- **Submodule expansion covers boot-configured sources.** Repo/git sources
  added at runtime via `add_source` materialize their submodules and report
  per-path states, but are not yet auto-expanded into per-submodule scoped
  components — visibly unexpanded on status, never misattributed.
- **Runtime source changes apply at the next restart.** `add_source` and
  `remove_source` persist desired state and reply with `restart_required`; the
  source starts or stops on the next boot, not live
  ([ADR-0013](docs/adr/0013-substrate-cutover-to-semengine.md)).
- **The graph is retention-first.** Safe, reference-complete deletion for genuine
  mistakes/churn is a future lifecycle feature, not the default behavior.

## Next

### Workbench Release Discipline

- Carry each verified immutable workbench pin into the matching SemSource release notes and keep
  future tag publications subject to the same registry/local/Compose/runtime evidence gate.
- Keep the former SemTeams profile note as historical evidence only; it creates
  no current SemSource compatibility obligation.

### Query Reliability And Scale

- Re-check graph-index write amplification and query-index readiness on
  SemEngine (first found on SemStreams), and file what remains as SemEngine
  issues, so `phase: ready` means consumers can reliably query large repos.
- Track the GraphQL capabilities route/responder mismatch until the substrate
  contract is aligned, then advertise the surface.
- Validate tier-2 semantic/instruct summaries and local/global/summary search as
  first-class options rather than experimental tier toggles.

### Packaged Local Experience

- Keep the UI-free backend/MCP stack as SemSource's default deployment for
  embedded use by SemTeams, SemSpec, SemDragon, SemOps, and other consumers.
- The zero-to-first-query path is documented and continuously proven: the
  [quickstart](docs/QUICKSTART.md) is executed verbatim by CI
  (`onboarding-quickstart` capability, beta.9).
- **Decided (#184, 2026-08-17): Docker Compose is the v1 launcher.** A
  bespoke one-action launcher (`add-one-action-local-start`) is deferred
  past v1, not rejected — the quickstart's CI-proven command sequence is its
  executable skeleton whenever it is picked up.
- Make the released path independent of sibling repository checkouts and a local
  JavaScript toolchain; UI activation remains explicit.

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
  churn, depending on SemEngine index cleanup and cascade primitives.
- **Beyond same-LAN**: packaged TLS and deployment hardening for exposed
  deployments.

## Tell us what matters

Early-adopter feedback reorders this list. If a capability here (or one that isn't)
would unblock you, open an issue on
[GitHub](https://github.com/C360Studio/semsource/issues) — real consumer pressure
is how we prioritize.

## Release detail by tag (agent reference)

Moved verbatim from `CLAUDE.md` when the repository adopted ateam (#238); the
narrative above is the public summary, this list keeps the per-tag mechanics.

The list below is the shape up to the latest public tag, beta.12:

1. `v1.0.0-beta.12` is the third SemEngine release — **breaking**:
   semengine `v0.1.0-alpha.8` plus untagged `3d823c8`, version-free config
   dependency and image IDs, the
   `streams` config key replaced by `graph_stream` (file-backed GRAPH by
   default; a beta.11 memory GRAPH stream refuses boot), and stale-marking of a
   deleted manifest's entities. Acceptance is the PR-level proofs (#393, #395,
   #399–#402, #406, #419, #424, #377); the scorecard regrade (#387) and the mini
   dogfood measurement (#407) follow the tag. See
   `docs/integration/beta12-adoption-note.md`.
2. `v1.0.0-beta.11` is the second SemEngine release — **breaking**: semengine
   `v0.1.0-alpha.8`, one entity ID per declaration across code and config files,
   refusals for sources that would share an entity-ID system, and GRAPH workqueue
   retention (a beta.10 GRAPH stream refuses boot). Acceptance was the OSH
   warm-restart proof on alpha.8 (#388); the scorecard regrade moved to the
   beta.12 image (#387). See `docs/integration/beta11-adoption-note.md`.
3. `v1.0.0-beta.10` is the first SemEngine release — **breaking**: Tier 0 on
   semengine `v0.1.0-alpha.6` (embeddings off; `graph-lexical` word search on
   by default), a substrate cutover with zero SemStreams imports, and a clean
   storage break (reordered entity IDs, `semengine_config_<org>_<stem>`, boot
   refusal over the old bucket). Acceptance was the Tier 0 graded scorecard on
   alpha.6 (dogfood 32/36, OSH 12/13, R1 36/36). Neural search (04C, #234)
   qualifies after the tag. See ADR-0013 and
   `docs/integration/semengine-adoption-note.md`.
4. `v1.0.0-beta.9` is the onboarding release — the first **non-breaking**
   tag in the beta line (still SemStreams `v1.0.0-beta.161`; no fresh
   storage, no reseed). It closes v1-blocker #184: `docs/QUICKSTART.md` is
   executable truth — CI extracts its `quickstart:single|multi`-marked
   command blocks and runs them verbatim against the public semdev-test
   fixtures (`test/e2e` doc-driver; path-filtered Quickstart workflow), so
   doc drift fails a build. D5 recorded on #184: Docker Compose is the v1
   launcher; `add-one-action-local-start` deferred past v1. It also ships
   the shared status-report contract (#188: `internal/sourcestatus.Report`
   replaces nine duck-typed copies, strict aggregator decode; per-source
   `backpressure` + `boundaries_skipped` now on every status surface) and
   `semsource add ast|repo --project/--version` (#189). New capability
   `onboarding-quickstart`; modified `ingestion-readiness` and
   `cli-onboarding`.
5. `v1.0.0-beta.8` is the git-submodule release (#185, ADR-0012), still on
   SemStreams `v1.0.0-beta.161` (no substrate cutover). Repos with submodules
   ingest completely (clone/pull materialize declared trees at their gitlink
   pins, shallow-first with full-fetch fallback; `submodules: false` opts out),
   loudly (per-path states — materialized / unmaterialized /
   excluded_by_config / declared_but_absent / beyond_cap — on every status
   surface), and with canonical identity (project = resolved-URL slug,
   version = 12-hex gitlink SHA riding `WatchPathConfig.Version`; same pin
   from any parent → byte-identical entity IDs; instance names are
   parent+link-scoped). Ingest walks never cross nested git working trees
   (`internal/gitboundary`); expansion runs at the composition root
   (`internal/subwatch`, boot-configured sources only — runtime adds are
   loud-but-unexpanded). Breaking only for graphs that had accidentally
   ingested submodule code under parent identity (fresh reseed). Known
   upstream dependency: semstreams#986 (config-watcher burst race; SemSource
   defends with a confirm re-put).
6. `v1.0.0-beta.7` targeted SemStreams `v1.0.0-beta.161` — the post-beta.160 reliability and
   lifecycle-control slice on top of
   the intentionally breaking graph/foundation refactor checkpoint (no
   upstream shims; fresh NATS storage mandated on adoption, so deployments upgrade via
   `docker compose down -v` + reseed — the graph re-derives from source). beta.161 makes
   shutdown caller-owned-context: `LifecycleComponent.Stop(ctx)` replaces `Stop(timeout)`,
   `StopAll` takes a bounded context created at the composition root, and stored lifecycle
   contexts are prohibited. It also restores WebSocket path configurability
   (`websocket_path` is honored again; `/ws` stays the default contract path) and attributes
   slow-consumer errors to the responsible subscription. beta.7 adds the asset ingestion
   guards (minified assets never parse into symbols; per-file symbol cap) and typed
   publish-error classification (transients retry idempotently via `Nats-Msg-Id` inside a
   90s budget; measured zero-loss on broker-pause and stream-constriction inductions).
   Ports use the strict
   `PortDefinition` envelope (typed `config` with `kind`); the ownership substrate is deleted —
   SemSource declares local `projection.Contract` intent (named reconcile groups `source` +
   `lifecycle` in `graph/contract.go`) and supersession mutates through the typed CAS client;
   framework component configs defer to beta.160 DefaultConfigs (a declared `ports` section
   replaces defaults wholesale); readiness stays on the `GRAPH_STATUS` KV bucket via
   `OpenCatalogReader`. The `graph.query.searchGraph` and `graph.query.summary` operations
   SURVIVE the query-contract closure (the removals were aggregate Go wrappers and shared
   agentic tool registrations).
7. Core ingestion, governed entity publishing, source manifest/status, fusion
   tools, version diffs, and consumer query integration are present.
8. The default Compose profile is UI-free: `docker compose up` resolves only
   NATS and SemSource (semembed sits behind the `semantic` profile). In deployment documentation, "headless" means
   this omitted workbench profile; it is not the removed `mode: "headless"`
   configuration value, which must continue to fail validation.
9. The opt-in `ui` profile now belongs to SemSource and serves the local `ui/`
   workbench. This is a breaking replacement for the former sibling SemTeams UI
   mapping; SemTeams owns its packaging and consumes unchanged SemSource HTTP,
   MCP, NATS, GraphQL, and governed graph contracts.
10. Release use requires `SEMSOURCE_UI_IMAGE=<tag>@sha256:<digest>` and no sibling
   checkout or host Node.js. Local development is explicit through
   `docker-compose.ui-dev.yml` or `task ui:smoke:dev`; `task ui:e2e` uses the
   lockfile-matched container runner.
11. OpenSpec task 7.3 is complete. Trusted `main` UI publish/smoke jobs verified
   revision `25b2816d14a147c1d6eb7b54e40668b51ba3574a` at manifest digest
   `sha256:43edacf62e7908681e7bedd193d1b18f3ebe8f3de438d417c6c091517020ea20`
   for `linux/amd64` and `linux/arm64`, including registry, local, Compose-rendered,
   and running-container pin proof in
   [Actions run 29693062800](https://github.com/C360Studio/semsource/actions/runs/29693062800),
   attempt 1. All six workflow jobs passed, including `build-and-push` and
   `ui-release-smoke`; the released browser profile passed 6/6. Graph drill-down
   uses the adopted SemStreams #533 facet in `v1.0.0-beta.153` through the existing
   code-context route, with local and real-profile acceptance; GraphQL is not part
   of that slice.
12. Active follow-ups are query-index readiness/scale, GraphQL capabilities
   alignment, code/version intelligence, and federation validation (#184
   easy-button onboarding closed in beta.9; both v1 blockers were done at
   beta.9, and the v1 gate then remaining was SemStreams reaching v1; the
   substrate is now SemEngine). The former upstream
   watches semstreams#986 and #977 are both CLOSED (verified 2026-08-27);
   #977 was fixed structurally in SemStreams `v1.0.0-beta.162`, which deletes
   `internal/lifecyclejoin`. SemSource saw that race locally under
   `go test -race -tags=integration` only while pinned to beta.161; the
   SemEngine cutover retires that pin (`go.mod` requires no SemStreams module,
   ADR-0013).
