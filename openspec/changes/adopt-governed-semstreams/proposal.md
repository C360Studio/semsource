## Why

SemOps wants to use SemSource as a source service for binary/protocol source
knowledge graph work. SemSource is blocked because it still pins
`github.com/c360studio/semstreams v1.0.0-beta.59`, while the current released
SemStreams tag is `v1.0.0-beta.114`.

This is not a routine version bump. The SemStreams range from beta.59 to
beta.114 includes the governed graph model:

- ADR-055 made graph entity birth envelope-bearing and changed
  `triple.add` / `triple.add_batch` to must-exist in beta.112.
- ADR-056 added projection contracts, ownership claims, foreign-edge claims,
  OwnerToken write leases, and optional hard owner-lease enforcement.
- Graph query and gateway surfaces gained summary, search, batch, prefix
  discovery, and typed prefix pagination.
- SemStreams now declares Go `1.26.3`; this checkout still declares and runs
  Go `1.25.3`.

SemSource already publishes a Graphable `semsource.entity.v1` payload, which is
good. But standalone SemSource does not yet bootstrap SemStreams ownership
buckets or bind SemSource projection contracts before graph-ingest starts. A
pin-only bump would leave governance in graceful-skip mode and would not be a
credible substrate for SemOps binary-source work.

## What Changes

- Move SemSource to SemStreams `v1.0.0-beta.114` and Go `1.26.3`.
- Update SemSource's manually built graph subsystem config to match current
  SemStreams graph-ingest, graph-query, graph-gateway, and metrics/query
  surfaces.
- Add a SemSource projection contract and ownership bootstrap in standalone
  mode, including OWNER_CLAIMS / OWNER_PRESENCE creation before graph-ingest
  starts.
- Decide and document the headless governance contract: host-owned ownership
  substrate, or explicit governance-disabled reporting.
- Audit all SemSource triples for cross-subject edges and must-exist ordering.
- Add compatibility and live graph smoke tests proving source payloads enter
  ENTITY_STATES with semantic envelopes and no `entity_not_found` rejections.
- Add a binary-source service proof gate for SemOps: binary by reference,
  memory-bounded handling, metadata-only graph projection, and governed
  projection ownership.

## Capabilities

### New Capabilities

- `semstreams-governance-adoption`: SemSource runs against current governed
  SemStreams and binds its source projection authority.
- `binary-source-service`: SemSource can be evaluated as a service boundary for
  binary/protocol source ingestion without putting raw binary in the graph.

### Modified Capabilities

- Standalone graph mode becomes governed by default rather than merely wiring
  graph-ingest/index/query/gateway components.
- Headless mode becomes an explicit contract with the host graph substrate.

## Impact

- `go.mod`, `go.sum`, CI, and local toolchain requirements.
- `cmd/semsource/run.go` graph and ownership boot.
- `graph/event_payload.go` and payload registration tests.
- Source handlers that emit relationship triples or media storage references.
- Standalone Docker and example configs.
- Integration docs and SemOps handoff notes.
