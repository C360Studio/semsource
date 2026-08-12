# Proposal: semstreams-beta160-migration

## Why

SemStreams `v1.0.0-beta.160` is the intentionally breaking migration checkpoint for the graph and
framework foundation refactor — no shims, no aliases, no state migration, fresh NATS storage
mandated. Staying on beta.159 leaves us off the supported foundation, and adopting also inherits
the fixes for four of our five upstream asks (#600 body TTL — our own release blocker, #601 title
embedding, #602 silent truncation, #597 fusion drop), which unblocks our next SemSource beta tag.
The compile-probe inventory (2026-08-12, in `design.md`) shows the break is real but bounded:
70 compile errors in three clean clusters, plus config-strictness and behavioral layers the
compiler cannot see.

## What Changes

- **Pin `v1.0.0-beta.160`** (transitively nats.go v1.52.0) and start all deployments on freshly
  provisioned NATS storage — no retained-state migration exists upstream, and our graph is fully
  re-derivable from source, so adoption is `down -v` + reseed.
- **Port-declaration migration ×8** — every source processor's `component.PortDefinition` moves
  from the removed flat `Type`/`Subject`/`StreamName` fields to the strict envelope (typed
  `config` with `kind`); JetStream inputs declare explicit `stream_name` + `subjects`;
  graph-query request families become named required `graph.query/v1` outputs where used.
- **Governance rewrite** — `pkg/ownership` is deleted upstream. The standalone ownership
  bootstrap (registry, heartbeater, `BindAndHeartbeat`) is replaced by local
  `pkg/projection.Contract` intent; `ModeReplaceOwned` → `projection.ModeReconcile`; mutation
  outcomes `entity_not_found` / `revision_mismatch` / `commit_unknown` are handled as distinct
  results with no framework retry.
- **graphstatus reader** — `OpenCatalogBucket` → `OpenCatalogReader` (we are a reader of
  `GRAPH_STATUS`, which is a retained surface).
- **Config-surface strictness** — top-level `version` bump on every changed config file; outer
  `services.<name>.enabled` flags; removal of message-logger/metrics inner flags from shipped
  configs; strict flow validation must pass at startup.
- **Behavioral proofs** — append no longer auto-creates entities (regression-test our
  relationship appends that may target not-yet-born entities, e.g. supersession); req/reply
  consumers classify `response_too_large` as a result-size failure, not a timeout;
  `StorageReference.StorageInstance` resolves only via the exact-named registry entry.
- **Test-layer migration** — retired query wrappers in e2e (`SearchGraph*`, `NATSQuerier`) and
  any test doubles over deleted APIs.
- **Migration proof by instrument** — full-stack fresh bring-up plus a three-arm scorecard rerun
  against the 2026-08-09 baseline (same questions v3, so comparable): recall regressions are
  migration defects; doc-band improvements are expected from the #601/#602 fixes.

**Non-negotiable (operator constraint, 2026-08-12): the migration leaves NO shims, NO deprecated
code, NO transitional aliases in the tree.** Delete-don't-deprecate; a final grep gate proves no
retired upstream symbol survives anywhere outside `openspec/changes/archive/`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `semstreams-governance`: the explicit SemStreams target moves to `v1.0.0-beta.160`, and the
  "standalone mode boots ownership before graph-ingest" requirement is replaced — the ownership
  substrate no longer exists; SemSource declares local projection-contract intent and uses typed
  CAS mutations with distinct failure outcomes.

Other specs (`entity-publish-integrity`, `ingestion-readiness`, `compose-deployment`,
`runtime-configuration`) are verified during apply and receive deltas ONLY if requirement-level
behavior changes; implementation-detail churn (reader constructors, port envelope syntax) does
not warrant spec edits.

## Impact

- `go.mod`/`go.sum`; 8 source processors (`component.go` + `config.go` each);
  `internal/governance/`; `internal/graphstatus/`; `configs/*.json`; e2e tests; possibly
  `cmd/semsource/run.go` service wiring (compiles today — verified against config strictness in
  apply).
- Deployment: fresh NATS storage everywhere; Compose already pins `nats:2.12-alpine`.
- Consumers to notify (fresh-storage + any query-shape changes): semspec (canary), semteams,
  Open Sensor Hub early adopter.
- Out of scope: adopting agentic/trajectory/tool-discovery surfaces we do not use; upstream #603
  (impact facet, still open); product features #137/#138.
