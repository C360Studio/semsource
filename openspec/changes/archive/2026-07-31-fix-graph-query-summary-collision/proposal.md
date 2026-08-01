## Why

`graph.query.summary` has **two live handlers inside a single SemSource process**:

| Handler | Location | Returns |
| --- | --- | --- |
| SemSource `source-manifest` | `processor/source-manifest/component.go:264` | `SummaryPayload` — namespace, phase, entity_id_format, domains, predicates |
| SemStreams `graph-query` | `processor/graph-query/query.go:45` | `graph.SummaryData` — entity-type aggregation, predicate counts |

Both subscribe with plain request/reply and no queue group, so a requester receives whichever
handler replies first and discards the other. The two payloads are entirely different shapes. **Any
consumer calling `graph.query.summary` on a SemSource deployment today gets a race.**

This is SemSource's defect, and the documented contract already says so. The consumer integration
guide (`docs/integration/m5-consumer-integration.md`) lists `graph.query.summary` among the
structural graph subjects "served by `graph-query`", and names only `status`, `sources`, and
`predicates` as source-manifest's. ADR-0003 claims source-manifest owns `summary` as well, and is the
stale outlier. The product boundary is unambiguous: SemStreams owns `graph.query.*`.

So removing SemSource's subscription **aligns behavior with the documented contract** rather than
breaking it — today's consumers are receiving SemSource's shape by luck, not by contract.

**No test catches this, and none can as written.** The only in-repo NATS caller,
`internal/governance/live_graph_integration_test.go:763`, already expects the substrate shape — and
it starts `graph-query` alone, never `source-manifest`, so the collision cannot arise there. It took
booting a real stack (the core-profile compose smoke, during `expose-graphrag-on-mcp`) to see
source-manifest win and the substrate lose.

## What Changes

- **`source-manifest` stops subscribing to `graph.query.summary`** and serves its `SummaryPayload`
  on `graph.query.sourceSummary` instead — beside its siblings `sources`, `predicates`, and
  `status`, and unable to collide because the substrate defines no such subject. `GET
  /source-manifest/summary` is unchanged and remains the primary path.
- **A regression guard** pinning that no subject SemSource subscribes to is also claimed by the
  substrate, so the next squat fails a test instead of shipping. This is the request/reply analogue
  of the existing GRAPH-stream subject-overlap pin.
- **The record is corrected.** ADR-0003's ownership claim gets a note; `m5-consumer-integration.md`
  documents `graph.query.sourceSummary` and states plainly that `graph.query.summary` now
  deterministically returns the substrate's shape — a behavior change for anyone who had been
  receiving SemSource's.
- **The `graph_summary` MCP tool is restored.** It was implemented and withdrawn during
  `expose-graphrag-on-mcp` for exactly this reason; with the subject uncontested it routes
  deterministically to the substrate's graph summary.
- **The three remaining squats are audited.** `sources`, `predicates`, and `status` also sit inside
  `graph.query.*`. They do not collide today because the substrate defines no such subjects; the
  audit records that and whether any is at risk, without moving them.

## Non-goals

- Moving `graph.query.sources`, `graph.query.predicates`, or `graph.query.status`. They are
  documented consumer contracts that work correctly today; renaming them would be a breaking change
  bought for tidiness rather than correctness. The audit records the exposure; a decision to migrate
  the whole family is a separate change.
- Changing `SummaryPayload`'s shape or the `/source-manifest/summary` HTTP route.
- Any semstreams change. The substrate is behaving correctly — it owns the namespace.
- Queue groups or any other scheme for sharing a subject between two handlers. Two different payload
  shapes on one subject is not a load-balancing problem.

## Consumers

Any consumer calling `graph.query.summary` over NATS. In-repo there is exactly one, and it already
expects the substrate shape. External consumers (SemSpec, SemTeams, SemDragon, SemOps) following
`m5-consumer-integration.md` also expect the substrate shape. A consumer that had come to depend on
SemSource's payload — which the guide never advertised — must move to `graph.query.sourceSummary` or
the HTTP route.

## Capabilities

### Modified Capabilities

- `semstreams-governance`: SemSource's subject claims must not overlap the substrate's, and every
  request subject it serves must have exactly one handler in a running deployment.
- `advertised-surface-coverage`: the subject-overlap guard needs named test evidence, and the
  restored MCP tool needs coverage that would fail if its subject became contested again.
- `graphrag-access`: the roster regains a graph-overview tool, now that the subject it needs is
  answered by exactly one handler.

## Impact

- `processor/source-manifest/component.go` — the subscription subject constant and its handler.
- `processor/mcp-gateway/` — restore the `graph_summary` tool and its tests.
- `cmd/semsource/run.go` + `run_test.go` — the graph-query port declaration already names
  `graph.query.summary` correctly; the pinning test grows the new SemSource subject.
- `docs/adr/0003-programmatic-source-add-api.md` — ownership-claim correction note.
- `docs/integration/m5-consumer-integration.md` — new subject, and the behavior-change notice.
- `docs/upstream/semstreams-asks.md` — not an upstream ask; the substrate is correct here.
- Archived change `expose-graphrag-on-mcp` task 8.2 is what this resolves.
