# Proposal: Source Removal Integrity

**Priority: P0** (audit 2026-07-19, findings: high ×1 live-confirmed at 20-min horizon, medium ×3)

## Why

`remove_source` currently reports success unconditionally and changes nothing observable:

1. Removing a **nonexistent** handle returns `removed:true` (live-verified) — a typo'd handle
   "succeeds" silently, so agent/curator workflows (ADR-040 SemTeams curator; ADR-0006 external
   registration) cannot distinguish success from no-op.
2. Removing a **real** handle returns `removed:true`, but the source remains in
   `source_status.status.sources` with its entities and `watching` phase indefinitely
   (live-verified at a 20-minute horizon; adds propagate in <12s). The status aggregator never
   drops removed sources (`processor/source-manifest/ingest.go:308`), leaving phantom sources that
   can permanently degrade workbench readiness.
3. Removing one expanded-repo instance erases the whole repo from the source manifest while its
   other three components keep ingesting (`processor/source-manifest/ingest.go:229`) — the
   manifest and reality diverge in both directions.

Removal is the least-exercised half of the registration surface that the sidecar/curator roadmap
depends on; today it is unverifiable by construction.

## What Changes

- `remove_source` with an unknown handle SHALL return the reserved `NOT_FOUND` error, not success.
- `remove_source` with a real handle SHALL stop the component, deregister it from the source
  manifest AND the status aggregator, and the change SHALL be observable via `source_status`
  within a bounded, documented interval.
- Removing one instance of an expanded repo source SHALL remove exactly that instance from the
  manifest; sibling instances remain listed and ingesting.
- The reply keeps its shape (`instance_name`, `removed`, `timestamp`) and gains nothing consumers
  must parse; honesty comes from the error path and the observable status change.
- Already-published graph entities remain (retention-first, ADR-0008); marking them with removal
  provenance is explicitly deferred to the `staleness-and-retraction` change (P2) — this change
  only fixes the lifecycle/observability contract.

## Capabilities

### New Capabilities
- `source-lifecycle`: add/remove registration is a verifiable contract — handles are real,
  removal is observable, unknown handles fail loudly, and the manifest mirrors the set of running
  source components at all times.

### Modified Capabilities
- `advertised-surface-coverage`: "Agent-facing MCP tools have happy-path coverage" gains the
  removal path — a real add→remove→status round-trip asserted in an automated gate (the audit
  found no gate executes any MCP `tools/call`).

## Impact

- `processor/source-manifest/ingest.go` (remove handling, manifest bookkeeping, aggregator
  deregistration), `processor/mcp-gateway/` (NOT_FOUND mapping), ServiceManager component
  stop/despawn path in `internal/sourcespawn/` and `cmd/semsource/run.go`.
- Consumers: SemTeams curator (ADR-040) and any ADR-0006 caller — behavior change is strictly in
  their favor (errors where silence was); NOT_FOUND on unknown handles is **BREAKING** only for
  callers that relied on unconditional success (none known).
- Boundary check: component stop/despawn uses semstreams ServiceManager APIs; if a needed despawn
  primitive is missing upstream, record the ask in docs/upstream/semstreams-asks.md — do not
  emulate substrate lifecycle inside SemSource.
