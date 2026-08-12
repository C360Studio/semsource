# Design: semstreams-beta160-migration

## Context

See `proposal.md` — Why. The upstream release is a clean break by design (no shims, no aliases, no
state migration); the operator constraint mirrors it downstream: nothing transitional survives this
change. The canonical upstream reference is the release-attached
`migration-beta159-to-beta160.md`; its linked cutover docs (36-graph-foundation, 37-ports,
query-contract closure, post-G tag-safety) are the detailed contracts.

**Compile-probe inventory (2026-08-12, go.mod pinned to beta.160, ownership import stubbed):**
70 error lines in exactly three clusters, plus one package-level removal:

1. `pkg/ownership` gone — imported by `internal/governance/{bootstrap,predicates}.go`;
   `projection.BindAndHeartbeat` also gone; `ModeReplaceOwned` gone.
2. 8 source processors × 7 identical errors — `component.PortDefinition` lost flat
   `Type`/`Subject`/`StreamName` (both the struct literals in `config.go` and the readers in
   `component.go`).
3. `internal/graphstatus/reader.go` — `semgraph.OpenCatalogBucket` gone.

Everything else compiles, including `cmd/semsource/run.go`, supersession, source-manifest, vision,
and the UI GraphQL layer (no `similaritySearch`/`entitiesByPrefix` usage). Test binaries were NOT
probed and will surface a second, smaller list (`test/e2e/beta148_cutover_test.go` references
retired `SearchGraph*`/`NATSQuerier` wrappers).

## Goals / Non-Goals

**Goals:** land on beta.160 with zero transitional code; prove behavior with our own instruments
(gates + e2e + scorecard) rather than trusting compile-clean; keep the diff reviewable by
separating mechanical clusters from semantic ones.

**Non-goals:** adopting trajectory/tool-discovery/milestone surfaces we do not use (their removal
lists must simply grep clean); redesigning the publish path beyond what the flow contract
requires; any retrieval feature work (#137/#138 stay separate — rerunning the scorecard is proof,
not tuning).

## Decisions

### D1 — Governance: local projection intent, reconcile semantics, loud CAS failures

The deepest semantic change. `BootstrapStandalone` currently creates ownership buckets
(`OWNER_CLAIMS`/`OWNER_PRESENCE`), binds a heartbeater, and seeds predicate vocabulary with
`ModeReplaceOwned`. Replacement shape per the cutover contract:

- Declare `projection.Contract` intent locally (no registry, no heartbeat, no buckets).
- Predicate-vocabulary seeding becomes `entity.reconcile` with `projection.ModeReconcile`: read
  the exact entity + KV revision first, reconcile against it.
- CAS outcomes are distinct results, surfaced loudly: `revision_mismatch` at bootstrap means a
  concurrent writer where none should exist — fail startup with the outcome named, never retry
  silently. `commit_unknown` is ambiguous (may have landed): fail startup and instruct re-check;
  never blind-retry (double-apply risk is the reason the framework doesn't retry for us).
- **Alternative rejected:** a thin internal wrapper mimicking the old `ownership.Registry` API to
  minimize the diff — explicitly ruled out by the no-shims constraint; the wrapper would be a
  compatibility surface with no owner.

### D2 — Ports: one canonical envelope pattern, applied eight times, verified by startup

The 8 source processors get one template fix: strict `PortDefinition` envelope with typed
`config` (`kind` + JetStream `stream_name`/`subjects` for stream inputs). No helper-macro layer:
the migration writes the declarations in place, matching whatever `semstreams/processor/ast-indexer`
(the canonical reference) does on beta.160. Strict flow validation at startup is the real gate —
unit tests prove shape, the e2e fresh bring-up proves admission.

### D3 — Fresh storage is an operational note, not code

Our graph is a projection of source; there is nothing to migrate. Compose deployments adopt via
`docker compose down -v` + reseed; e2e already provisions fresh. The consumer-facing note
(semspec first — it is the canary; then semteams, OSH) states: fresh storage on upgrade, and the
retained read contracts. No code owns this; it is a documented adoption step in the PR/release
notes.

### D4 — The no-shims gate is mechanical, not honor-system

Final task runs a retired-symbol grep across the tree (excluding `openspec/changes/archive/`):
`pkg/ownership`, `ModeReplaceOwned`, `ReplaceOwned`, `BindAndHeartbeat`, `OpenCatalogBucket`,
`OWNER_CLAIMS`, `OWNER_PRESENCE`, flat `Type:`/`Subject:`/`StreamName:` in `PortDefinition`
literals, `SearchGraph`, `SummarizeGraph`, `NATSQuerier`, `StartService`, `StopService`,
`RuntimeConfigurable`. Zero hits or the change does not close. This list is written into the task
so it survives context loss.

### D5 — Behavioral deltas get regression tests, not assumptions

- **Append-no-stub:** any SemSource append whose target may not yet exist (supersession
  relations are the known candidate) gets a regression test proving the entity is born with its
  envelope first — this strengthens the existing "no auto-vivify" spec requirement rather than
  changing it.
- **`response_too_large`:** consumers of req/reply results treat it as a result-size failure
  (distinct from timeout) — covered where we already classify reply outcomes.
- **Exact store resolution:** config names the `objectstore` registry entry exactly; the
  offloaded-body e2e proves bodies resolve on the new pin.

### D6 — The scorecard is the product-proof instrument

The upstream guide requires each product to prove its own ingest/query/restart behavior. We have
a measuring instrument for exactly this: after the full-stack fresh bring-up, rerun all three
arms on questions v3 and compare per band against the 2026-08-09 baseline. Expected signature:
recall ≥ baseline everywhere (a regression is a migration defect, not noise); doc bands may
*improve* (upstream #601 title embedding + #602 truncation fixes landed since); arm A is corpus-
only and serves as the unchanged control. UNSTABLE counts matter as much as scores — the repeat
logic exists precisely for a platform under churn.

## Risks / Trade-offs

- [Config strictness rejects shipped configs at startup, not compile time] → migrate configs in
  the same commit as port declarations; e2e fresh bring-up is the gate; bump `version` on every
  touched config file (equal/older versions are ignored against KV-selected config).
- [`commit_unknown` at bootstrap could mask a landed write] → fail-loud + documented re-check
  procedure; bootstrap is idempotent-by-reconcile so a clean restart converges.
- [Hidden consumers of removed buckets (`OWNER_CLAIMS` seen in our own stack probe 2026-08-09)] →
  the no-shims grep covers bucket names too; armc-dump and scorecard touch only retained buckets
  (`EMBEDDING_INDEX`, `ENTITY_STATES`, `CONTENT`, `GRAPH_STATUS`).
- [Scorecard comparability across substrate versions] → same questions v3 + same corpus commit
  keeps the comparison valid per the README rules; the baseline stack pin (beta.159) is recorded
  in the baseline results header for honest annotation.
- [Transitive nats.go bump v1.48→v1.52 touches armc-dump] → it uses the stable jetstream API;
  compile + a live vectors smoke in the validation phase covers it.

## Open Questions

- Whether `graph.query/v1` request-family output declarations apply to our components at all, or
  only to rule-driven components (our processors publish to ingest streams; the fusion gateway is
  framework-owned) — resolved by the first strict-validation run; affects task 2 scope only.
