## Why

SemStreams `v1.0.0-beta.159` is a declared sister-lockstep breaking wave — 92 commits and seven
breaking merges — and its tag annotation names SemSource as one of the sisters that must conform at
the tag. SemSource's current beta.158 composition will not boot against it: `cmd/semsource/run.go`
declares a `ports.outputs` entry on graph-embedding pointing at the deleted `EMBEDDINGS_CACHE`
bucket, and beta.159 rejects any graph-embedding output port at config validation. The wave's
contracts are clean-break by design — no aliases, no dual reads — so adoption also requires a
destructive graph-state cutover. Separately, SemSource pins the floating `nats:2-alpine` tag while
SemStreams runs `nats:2.12-alpine` everywhere current; the two must match, and the pin change shares
this migration's single restart window.

A compile probe against beta.159 (throwaway worktree, `go build ./...`, `go vet ./...`,
`-tags=integration`, `-tags=e2e`) is **clean at every build tag**. This migration is therefore a
configuration, runtime, and NATS-state cutover, not a source port — and it must not be planned or
reviewed as if a green build meant conformance.

## What Changes

- Pin `github.com/c360studio/semstreams v1.0.0-beta.159`, keep the module free of any `replace`
  directive, and tidy the module graph. This also closes existing spec drift: `semstreams-governance`
  still names `v1.0.0-beta.153` as the target while `go.mod` has moved through beta.157 and beta.158.
- **BREAKING**: delete the graph-embedding `ports.outputs` block from `cmd/semsource/run.go`.
  beta.159 fails component creation with `graph-embedding declares no output ports; remove
  ports.outputs`. graph-embedding's durable writes (`EMBEDDING_INDEX`, `EMBEDDING_DEDUP`, the
  `GRAPH_STATUS` readiness envelope) are direct bucket writes at `Start`, never ports.
- **BREAKING**: re-derive the framework-owned bucket inventory in `test/e2e/beta148_cutover_test.go`
  from the beta.159 catalog (`graph/kvcatalog.go`, 22 descriptors). `EMBEDDINGS_CACHE` is gone;
  `COMMUNITY_SUMMARIES`, `GRAPH_STATUS`, `STORAGE_REPORT`, `ENTITY_SUFFIX_INDEX`, and
  `GRAPH_INGEST_APPLIED_SEQ` are new. The literal list is a deliberate tripwire: it exists to stop
  the rehearsal for review rather than silently widen a deletion boundary.
- Confirm every declared KV port subject resolves to a catalog bucket. Off-catalog subjects now fail
  the owning component's `Start` and therefore boot. SemSource's four graph-index subjects
  (`OUTGOING_INDEX`, `INCOMING_INDEX`, `ALIAS_INDEX`, `PREDICATE_INDEX`) and the graph-clustering
  `COMMUNITY_INDEX` subject are all catalog-resolvable and stay as they are.
- **BREAKING**: declare bounds and a discard policy on the `GRAPH` ingest-transport stream.
  beta.159 requires every ordinary stream to state a finite `max_age`, a finite `max_bytes`, and an
  explicit `discard` — the policy was previously hardcoded to `old`, so it was never anyone's choice.
  SemSource declares `new` (refuse at the ceiling) rather than `old` (silently evict undelivered
  source facts), and exposes the knob as a `streams.GRAPH.discard` override. This was found by the
  test suite during adoption, not by the earlier audit, which had assumed the existing
  `max_bytes`/`max_age` declaration was sufficient.
- **BREAKING**: pin the Compose NATS server to `nats:2.12-alpine`, matching SemStreams' e2e, tiered,
  and testcontainers usage, and satisfying `compose-deployment`'s existing "exact version, no bare
  `latest`" pin requirement that a floating major tag does not meet.
- **BREAKING**: execute a stopped-writer, catalog-derived, destructive local graph-state cutover,
  then reseed and prove query parity and replay parity. `ENTITY_STATES` History additionally
  reconciles 3 → 1 on first boot, discarding stored revisions beyond depth 1.
- Verify — without assuming a code change — the three behavioral contracts that compile silently:
  add-lane six-field tuple deduplication, `index_not_ready` classified errors naming an unprovisioned
  bucket's owner, and graph-clustering's ownership split where `COMMUNITY_INDEX` becomes trigger-only
  and summaries land in the content-addressed `COMMUNITY_SUMMARIES`.
- Record the evidence envelope required by `docs/operations/31-sister-repo-cutover-checklist.md` and
  report adoption against SemStreams gh#753.

## Non-goals

- Adopting the caught-up readiness producers (`GRAPH_STATUS` folding, ADR-088). It is explicitly
  additive and not breaking; SemSource already watches `GRAPH_STATUS` for code-context readiness.
  Folding the remaining waits is a follow-up, not lockstep conformance.
- Changing the GraphQL read path. The `graphSummary` envelope unwrap (gh#762) is the wave's only
  GraphQL break, and SemSource reads no such path: `graph.query.summary` is consumed NATS-direct by
  source-manifest, which the adopter note states is unaffected. Adding or stripping a `.data` hop
  here would introduce a bug, not adopt a change.
- Preserving incompatible graph state, supporting mixed-version writers, or adding any compatibility
  shim, alias, or dual-read path.
- Redesigning entity IDs, predicate vocabulary, graph lifecycle, source readiness semantics, or
  component/payload composition.
- Wiping or migrating any state beyond the local Compose JetStream deployment. Shared or staging
  accounts are the operator's call and are not in this change's scope.
- Editing SemStreams. Framework gaps found during adoption become new issues in the semstreams queue
  referencing gh#753 — never a PR to that repo.

## Consumers

SemSource's own ingestion, fusion, MCP, code-context, and workbench query paths run on the migrated
substrate. SemStreams remains the authority for the governed substrate, the bucket catalog, and graph
state. SemSpec, SemDragon, SemOps, and SemTeams consume unchanged SemSource HTTP, MCP, NATS, and
GraphQL contracts — this change alters no SemSource-owned outward contract, so they receive no
handoff.

## Capabilities

### New Capabilities

None. This wave changes SemSource's substrate conformance and deployment pin, not its source-ingest
surface.

### Modified Capabilities

- `semstreams-governance`: advance the explicit SemStreams target from the stale beta.153 to
  beta.159, add the framework KV bucket catalog conformance rules (no graph-embedding output ports,
  catalog-resolvable KV port subjects, readers never create buckets), and define the beta.159
  destructive graph-state cutover.
- `compose-deployment`: require the Compose NATS server image to be an exact version pin aligned with
  the SemStreams-tested server line, replacing the floating `nats:2-alpine` major tag.
- `entity-publish-integrity`: extend "never loses entities silently" from the publisher to the
  transport beneath it — the ingest stream declares its bounds and refuses at the ceiling rather than
  evicting messages that were published but not yet ingested.

## Impact

Affects the Go module pin, the graph-embedding and NATS service blocks of `cmd/semsource/run.go`,
`docker-compose.yml`, the beta148 cutover rehearsal test's bucket inventory, and local JetStream
state, which is destroyed and rebuilt. Canonical source inputs remain the recovery authority: the
graph is rebuilt from them, and no non-graph store is touched. No Go API, exported surface, or
outward SemSource contract changes.
