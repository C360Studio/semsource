# Durable source-removal projection

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
- Fence old removal work from same-handle re-add. Clear `source_removed` only for exact current entities
  whose new producer published durable receipts, preserving vanished documents and passage history.
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

The change adds a SemSource-owned operational journal and producer publication receipts. It touches
source-manifest desired-state handling, common entity publishing and source factory wiring,
supersession lifecycle projection, boot wiring, and their tests. The supported operating boundary is
one SemSource process per effective authority/configuration store; controlled replacement waits for
process exit. Concurrent processes sharing that authority are not qualified by this change.
