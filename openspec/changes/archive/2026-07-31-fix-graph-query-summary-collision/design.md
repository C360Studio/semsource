## Context

See `proposal.md` — Why for the defect and the ownership evidence. This document records the
decisions the fix turns on, and one constraint that changed the shape of the guard.

The relevant fact about the substrate: `graph-query` registers its request handlers from an
**unexported literal slice** in `setupQueryHandlers` (`processor/graph-query/query.go:30-50`). Its
`InputPorts()` reflects only the *configured* ports, and in a SemSource deployment that configuration
comes from SemSource itself (`graphQueryInputPorts()`, `cmd/semsource/run.go:707`). There is no
exported, authoritative list of the subjects the substrate actually answers.

That matters because the obvious guard — "compare SemSource's subscriptions against the substrate's
handler set" — cannot be written against the pinned dependency.

## Goals / Non-Goals

**Goals:**

- Make `graph.query.summary` answered by exactly one handler, deterministically, matching the
  documented consumer contract.
- Keep SemSource's summary payload available on NATS, on a subject it owns.
- Make the next squat fail a test rather than ship.
- Restore the `graph_summary` MCP tool that this defect forced out of `expose-graphrag-on-mcp`.

**Non-Goals** (beyond `proposal.md` — Non-goals):

- Making the guard authoritative against the substrate's real handler set. That needs an export the
  substrate does not offer; see D3.
- Migrating `sources` / `predicates` / `status` out of `graph.query.*`.

## Decisions

### D1 — `graph.query.sourceSummary`, not a new namespace

SemSource's summary payload moves to `graph.query.sourceSummary`.

*Why not a SemSource-owned namespace like `source.query.summary`.* Its three siblings —
`graph.query.sources`, `graph.query.predicates`, `graph.query.status` — are documented consumer
contracts that work correctly. Moving one of the four to a different namespace splits the family and
implies a migration of the other three that this change explicitly declines. `sourceSummary` sits
beside its siblings, reads as what it is (the *source* manifest's summary, not the graph's), and
cannot collide: the substrate defines no such subject.

*Why not keep both subscriptions during a deprecation window.* The dual subscription **is** the
defect. A window would extend the race rather than deprecate it.

*Why the break is acceptable without a compatibility shim.* There is no honest shim available. Any
SemSource handler left on `graph.query.summary` — even one returning a "moved" error — still races
the substrate's valid answer. The break is also smaller than it looks: the consumer integration guide
never advertised `summary` as SemSource's, the sole in-repo NATS caller already expects the substrate
shape, and `GET /source-manifest/summary` is untouched.

### D2 — The old subject changes shape, and that is stated loudly

After this change, `graph.query.summary` deterministically returns the substrate's `SummaryData`.
A consumer that had been receiving SemSource's `SummaryPayload` — by winning a race, not by
contract — gets a different shape and no error.

A silent shape change is the worst failure mode available, and there is no mechanism to make it loud
(see D1). The mitigation is therefore documentary and deliberate: `m5-consumer-integration.md` gains
an explicit behavior-change notice naming both shapes and the new subject, and ADR-0003's stale
ownership claim is corrected at the point where a reader would otherwise be misled.

### D3 — The guard compares against SemSource's own declaration, and says so

The ownership gate cross-checks the subjects SemSource components subscribe to against
`graphQueryInputPorts()` — SemSource's in-repo declaration of the substrate query surface, already
pinned by `run_test.go`. On today's code the intersection is exactly `{graph.query.summary}`, so the
gate reproduces the defect before the fix and passes after it.

*Limit, stated rather than hidden.* This compares SemSource's subscriptions against SemSource's
*declaration* of the substrate surface, not the substrate's real registrations. If a future semstreams
release adds a subject SemSource already claims, the gate stays green until that declaration is
updated. Two responses, both in scope:

1. `graphQueryInputPorts()` is currently **incomplete** — it omits `graph.query.byName`, which the
   substrate does serve. No SemSource component claims it, so nothing is broken today, but an
   incomplete list is a weaker guard. Add it.
2. File an upstream ask for an exported subject list, so the gate can compare against the authority
   instead of a maintained copy. Same shape as the `Strategy` ask: the value exists inside the
   package and is not exposed.

*Alternative rejected — a runtime probe.* Booting a stack and checking for multiple responders would
be authoritative, but request/reply gives the caller no way to see that a second responder existed;
it only ever sees one reply. Detecting duplicates would mean scraping subscription counts from NATS
monitoring — a lot of machinery for a check that a static comparison makes exact.

### D4 — `graph_summary` returns to the roster as a pure passthrough (later cut, see note)

The tool is restored from `expose-graphrag-on-mcp`, unchanged in intent: route to
`graph.query.summary`, return the substrate payload verbatim, add **no** retrieval disclosure. The
subject is a discovery resolver — no clustering, no retrieval strategy, no synthesis — so there is no
rung to report, and attaching a disclosure would imply a path choice that was never made.

Its test asserts the substrate's **shape**, not merely a non-error result. That is deliberate: a
shape assertion is what turns a future reintroduced collision into a failure instead of a pass, since
the competing payload would decode to an empty `SummaryData`.

> **Superseded before merge.** The tool was cut on roster grounds: no SemSource MCP tool accepts an
> entity type or a predicate, so a graph overview emits a vocabulary the agent cannot spend.
> Upstream's `summarize_graph` is actionable because its roster has `query_by_type`; ours is not.
> The collision fix is unaffected — it was a live defect independent of any tool, and this tool is
> what surfaced it. See tasks 3.5.

## Risks / Trade-offs

- **A consumer silently receives a different shape** → Unavoidable (D1/D2); mitigated by an explicit
  notice in the consumer guide and the ADR correction. Worth weighing against the status quo, where
  the same consumer already receives an unpredictable shape.
- **The guard's authority is one step removed** → Stated in the spec and the code comment, narrowed
  by completing the port list, and tracked upstream (D3). A guard whose limit is documented is much
  safer than one assumed total.
- **`sources` / `predicates` / `status` remain in the substrate's namespace** → They do not collide
  at the pinned target, and the guard now watches them. The exposure is recorded in the audit rather
  than fixed by a rename nobody asked for.
- **The restored tool re-enters on a subject this change just vacated** → Sequencing matters: the
  subscription is removed in the same change that restores the tool, and the ownership gate runs in
  the same CI pass, so the tool cannot ship against a contested subject.

## Migration Plan

Breaking for any NATS consumer depending on SemSource's payload at `graph.query.summary` — believed
to be none, in-repo or documented. Migration is one of: switch to `graph.query.sourceSummary`, or use
`GET /source-manifest/summary`, which is unchanged.

Sequencing: move the subscription → add the ownership guard (proving it fails before / passes after)
→ complete the port list → restore `graph_summary` with a shape-asserting test → correct ADR-0003 and
the consumer guide → audit the three remaining subjects → file the upstream ask.

## Open Questions

- Whether the other three SemSource subjects should eventually move out of `graph.query.*`. The audit
  produces the evidence; the decision needs a consumer-migration cost that this change does not have
  to price, and no requirement here depends on the answer.
