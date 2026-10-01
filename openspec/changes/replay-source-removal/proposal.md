# Durable source-removal projection

## Superseded direction — 2026-10-01

The owner withdrew architectural approval of this private recovery design and authorized removal in
[PR #223](https://github.com/C360Studio/semsource/pull/223). The approved
[corrective change](../remove-private-source-recovery/proposal.md) introduces no replacement recovery
system. This proposal remains historical evidence; it no longer authorizes new journal/receipt/fence
or replay work. Source-removal projection and positive reactivation are explicitly deferred.
See the [reassessment record](../../../docs/testing/private-recovery-retirement/audit.md).

## Why

Issue #215 is a retained-graph correctness gap. Desired source removal survives restart, but the
retained document and passage entities do not receive `source_removed`. The unchanged pre-fix probe
at `fa5c250` passes 9/10 checks and captures no automatic lifecycle request. The frozen SETUP 03A
comparison at `75a17f7d6297c3fa18102f3e09f8a0a4a10750bf` remains historical evidence.

SemSource owns source intent, source scope, and its lifecycle projection. SemStreams continues to own
entity storage, query pagination, mutation CAS, and lifecycle primitives. This is product work; it
neither repairs the substrate nor changes the pinned dependency.

## What changes

- Persist current removal intent before the disabled desired envelope and successful removal receipt.
- Repair interrupted desired writes and graph projection from a retained operational journal at boot
  and during bounded periodic reconciliation, after the old producer is retired.
- Enumerate exact, independently owned source scope across every opaque-cursor page. Refuse ambiguous
  overlapping ownership and propagate every incomplete enumeration or mutation.
- Fence old removal work from same-handle re-add and preserve current-epoch publication evidence.
  Keep selective freshness pending at this pin: the public mutation client cannot fence a clear to
  the exact source facts checked by SemSource. Preserve vanished documents and passage history.
- Expose generation, pending/completed state, and actionable failure; validate restart, crash,
  redelivery, multi-page work, recovery, re-add, and sibling isolation with additive evidence.

Consumers are SemSpec, SemDragon, SemOps, and SemSource's MCP/HTTP/UI query clients. Their entity IDs,
content references, and query protocols remain unchanged; retained stale facts become distinguishable.

## Non-goals

- Physical graph or content purge (#210), reference-blind retention, or rewriting old entity identities.
- SemStreams broker-generation recovery, stream collision repair, SemEngine extraction or cutover.
- Live component replacement, source discovery (#216), or distributed active/active source ownership.
- Changing providers, served models, dimensions, preprocessing, module pins, or frozen comparison data.

## Impact

The change adds a SemSource-owned operational journal and producer publication receipts. The frozen
pin cannot prove that every accepted source input was applied: parked messages disappear from backlog
counters and no supported complete unresolved-input API exists. Automatic removal therefore repairs
the current retained set but remains pending with `applied_tail_unproven`; full #215 closure remains
blocked by [SemStreams #1444](https://github.com/C360Studio/semstreams/issues/1444). Selective freshness
also remains blocked by [SemStreams #1445](https://github.com/C360Studio/semstreams/issues/1445): matching
current publication evidence does not supply an admitted conditional reconcile capability. Inventory
and repair may observe separately sealed live batches, but withdrawal eligibility is not qualified and
no production clear or live-entity freshness is claimed. Marker success alone is not migration
qualification. The change touches
source-manifest desired-state handling, common entity publishing and source factory wiring,
supersession lifecycle projection, boot wiring, and their tests. The supported operating boundary is
one SemSource process per effective authority/configuration store; controlled replacement waits for
process exit. Concurrent processes sharing that authority are not qualified by this change.
