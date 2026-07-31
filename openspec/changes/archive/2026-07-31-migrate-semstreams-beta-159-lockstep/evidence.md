# Evidence envelope — SemStreams beta.158 → beta.159 lockstep migration

Required by `docs/operations/31-sister-repo-cutover-checklist.md`. One immutable record per product.

| Field | Evidence |
|---|---|
| **Product identity** | `github.com/c360studio/semsource`, owner C360Studio, branch `migrate/semstreams-beta-159-lockstep`, evidence collected 2026-07-31 |
| **Dependency transition** | Observed: `v1.0.0-beta.158` @ `d6d3e57e60173c3534796b674b16e2ce75caeeca`. Target: `v1.0.0-beta.159` @ `8813270c5ba441286d9120cba82fbf72bdcf9a6c`. No `replace` directive; second `go mod tidy` is a no-op. |
| **Deployment identity** | Local Docker Compose, default (UI-free) profile plus the tier-2 dev overlay. NATS `nats:2.12-alpine`, JetStream on the `semsource_nats-data` volume. Compose project `semsource`. |
| **Corpus gates** | No entity-ID or predicate contract changed in this wave, so no corpus re-audit was required. Framework-owned bucket set re-derived programmatically from `graph.FrameworkOwnedBuckets()` at the target tag (22 entries) rather than transcribed. |
| **Composition** | graph-ingest, graph-index, graph-embedding, graph-query, graph-gateway, graph-clustering (tier-2 only), objectstore, source-manifest, code-context, doc-context, mcp-gateway, supersession, websocket-output. No rule-processor components, so no `pack_id` applies. Every declared KV port subject resolves to the framework catalog — asserted by `TestGraphSubsystemComponents_KVPortSubjectsResolveToFrameworkCatalog`. |
| **Package cutover** | None. The wave produced **zero** Go API breakage for SemSource: `go build ./...`, `go vet ./...`, `-tags=integration`, `-tags=e2e` were all clean at the pin with no source edits. |
| **Wipe** | **Deletion set was EMPTY.** At cutover the local Docker environment held no volumes and no containers (`docker volume ls` and `docker ps -a` both empty), so no pre-beta.159 graph state existed. Recorded as an observed empty inventory, not a performed wipe. The procedure itself is exercised end-to-end by `TestE2E_Beta148CutoverRehearsal` against a disposable account, which passes at beta.159. |
| **Reseed/rebuild** | Fresh boot on beta.159 from a canonical fixture workspace. `phase: ready`; 8 distinct entities (ast 5, docs 2, config 1). `GRAPH_STATUS` shows `graph-ingest` (`bootstrap_complete: true, bootstrap_scope: 8`), `graph-index` and `graph-embedding` all `ready: true` at revision 18. |
| **Query parity** | Canonical known-answer query `POST /code-context/context {"query":"CutoverSentinel"}` returns the symbol with deterministic handle `c360.semsource.golang.workspace.function.src-ledger-go-CutoverSentinel`, correct path/lines/body. |
| **Replay parity** | Restarted once with no intervening write; the same query returned a byte-identical node set (`diff` clean over the sorted `nodes` payload). |
| **Event consumer** | No SemSource component consumes the raw event stream in a way this wave changes. Add-lane dedup verified behaviorally instead: after a full restart and republication, `total_entities` remained 8 with per-source counts summing to 8 — distinct cardinality invariant under republication. |
| **Verification** | `task lint` PASS (revive v1.15.0, warnings fail) · `task test` 41/41 · `task test:race` 41/41 · `go test -tags=integration ./...` 42/42 · `task test:e2e` PASS (156s) · `task core:smoke` PASS · `task tier2:smoke:dev` PASS. |
| **Exceptions** | `none`. Three defects were found at runtime and fixed in this change (see below); all were SemSource's own, so no new semstreams issues were filed. |
| **Adoption reported** | Reported to semstreams gh#753 by the product owner. |

## Defects this migration surfaced

All three compiled cleanly and passed the pre-existing unit suite. None could have been caught
without running a real stack — which is why design D1 refuses a green build as conformance evidence.

1. **Ordinary-stream bounds (framework-enforced).** beta.159 requires `max_age`, `max_bytes` **and**
   `discard` on an ordinary stream. `GRAPH` declared the first two, so the pre-migration audit
   concluded no change was needed. Every source spawn failed config validation until `discard` was
   declared. Chose `new` (refuse at the ceiling) over `old` (silently evict undelivered source
   facts) — see design D7.

2. **Readiness read from a deleted subject.** ADR-083 removed `graph.index.query.status`. Three
   production call sites still requested it (`source-manifest`, `supersession`, `mcp-gateway` — the
   last found only after an unfiltered re-grep, having been truncated out of the first search). The
   result was `index: {available: false, state: "unknown"}` reported permanently while
   `GRAPH_STATUS/graph-index` carried `ready: true`, and `task core:smoke` — which blocks on
   `index.ready` — could never pass. Fixed by a shared `internal/graphstatus` reader.

3. **Fail-closed boot exposed a latent source bug.** `doc-source` and `cfgfile-source` gated ingest
   on the directory-only `IsRepoReady` while legitimately naming files (`README.md`, `go.mod`), so
   they retried forever. beta.158 brought the manager up around them (the quickstart e2e passed in
   7.75s with six such errors in its logs); beta.159's component-start barrier makes the same
   condition fail the entire boot. Isolated by running the identical test at `main`. Fixed with
   `workspace.IsPathReady`.

## Behavioral contracts verified

- **Add-lane six-field dedup** — counts invariant under republication (see Event consumer above).
- **Acquisition seam** — a reader against an unprovisioned bucket gets a retryable, classified
  not-ready error naming the owner: `framework bucket "GRAPH_STATUS" is not ready: its owner
  (graph-index/graph-embedding (readiness producers)) has not provisioned it in this deployment`.
  An off-catalog name is classified invalid instead. Covered by
  `go test -tags=integration ./internal/graphstatus/`.
- **Clustering ownership split** — with tier-2 clustering enabled, `COMMUNITY_INDEX` holds 27
  community records keyed `{level}.{entity-id}`, and summaries land separately in
  `COMMUNITY_SUMMARIES` content-addressed as `{level}.{membership_hash}` (ADR-087) carrying real
  `llm_summary` text from seminstruct.
- **GraphQL `graphSummary` unwrap** — no action, verified by absence. No `graphSummary` or
  `data.data` occurrence in Go, `ui/`, or any GraphQL document; SemSource's only
  `graph.query.summary` consumer is source-manifest over NATS-direct, which the adopter note states
  is unaffected.

## Not claimed

- `ENTITY_STATES` History 3 → 1 reconcile was **not** observed, because there was no pre-existing
  bucket to reconcile. A deployment upgrading with existing state should expect the documented
  destructive reconcile and capture any out-of-tree history first.
- The tier-2 overlay's full bring-up is gated behind `SEMSOURCE_TIER2_SMOKE_FULL=1` and is not run
  in CI; `task tier2:smoke:dev` covers composition, pins, and config wiring only.
