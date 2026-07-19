# Design: Source Removal Integrity

## Context

Live-verified: `remove_source` returns `removed:true` for nonexistent handles, and a genuinely
removed source remains in `source_status` with its entities and `watching` phase at a 20-minute
horizon (adds propagate in <12s). The status aggregator never drops removed instances
(`processor/source-manifest/ingest.go:308`); removing one expanded-repo instance erases the whole
repo from the manifest while siblings keep ingesting (`:229`).

## Goals / Non-Goals

- Goals: removal is real (component stops), observable (status reflects it within a bounded
  interval), and honest (unknown handle → NOT_FOUND). Manifest mirrors the running set.
- Non-goals: retracting/marking already-published entities (deferred to staleness-and-retraction);
  changing the AddReply/RemoveReply wire shapes beyond the error path.

## Decisions

- **D1 — existence check before ack**: the remove handler resolves the handle against the manifest
  registry; unknown → reserved `NOT_FOUND` error code (already defined in the reply contract,
  currently unreachable). Idempotency concern (double-remove) is handled by returning NOT_FOUND on
  the second call — explicit beats silently-true; ADR-040 curator retries can treat NOT_FOUND
  after a prior success as success.
- **D2 — full deregistration path**: remove = stop component via the ServiceManager
  despawn/stop API + delete manifest entry + deregister from the status aggregator (drop its
  last-report record) in that order; status reflects removal no later than the next aggregation
  tick (documented bound). If the ServiceManager lacks a usable stop primitive, the gap is logged
  as a framework ask and the component is at minimum quiesced (watch loops cancelled via its
  context) — observability does not wait on upstream.
- **D3 — instance-scoped manifest bookkeeping**: manifest entries keyed by instance, not by parent
  source; removing an expanded-repo instance removes exactly that entry. The repo-level view (if
  any) derives from surviving instances.

## Risks / Trade-offs

- [Consumers relying on unconditional success] none known; NOT_FOUND is the documented reserved
  code. → Release note.
- [Stop primitive availability] D2 fallback keeps the contract observable regardless.

## Migration Plan

Ship; no data migration. Rollback = revert.

## Open Questions

- none blocking.
