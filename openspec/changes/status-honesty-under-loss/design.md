# Design: status honesty under delivery loss

## Context

See `proposal.md` — Why. Two facts discovered while drafting the specs shape the
whole approach, and neither is visible from the issue:

**`error_count` is a sum, and it already contains loss.** Every source computes
it as `ingestErrors + publisher.Lost()`, with `ast-source` adding `parseFailures`
and `git-source` adding `handler.WatchErrorCount()`. It therefore conflates three
unrelated conditions: a file that could not be parsed, a generic ingest error,
and an entity that never reached the graph. This is why #177's evidence showed
`error_count` exactly equal to `entities_failed_total` — that run simply had no
parse failures.

**The publisher's counters are monotonic.** `Published`, `Failed`, and `Dropped`
are `atomic.Int64` with only `Add(1)` call sites; nothing resets them. `Lost()`
is `Failed + Dropped`, so once it is non-zero it stays non-zero for the process
lifetime.

The shared report type (`internal/sourcestatus.Report`, #188) is decoded strictly
by the aggregator: unknown fields are a loud decode failure, not a silent drop.
Producers and the shared type must therefore change together, in one commit.

## Goals / Non-Goals

**Goals:**

- Readiness degrades on *delivery loss specifically*, not on any error-shaped count.
- Loss remains distinguishable after the fact, per source, on every surface.
- The change is implementable without touching the publisher's internals — every
  number it needs is already exposed.

**Non-Goals:**

- Re-publishing or repairing lost entities (see `proposal.md` — Non-goals).
- Changing what `error_count` counts. It is specified by *Source status reflects
  delivery truth* to carry parse failures and publish rejections, and it keeps
  doing that.

## Decisions

### D1: Degrade on the loss figure, not on `error_count`

**Decision:** the aggregate degrades when a source reports non-zero *loss*, not
when it reports non-zero `error_count`.

The predicate originally proposed for this change was
`Phase == errored || ErrorCount > 0 || LastError != nil`, borrowed from
`workbench_capabilities.go:233` and `component.go:839`. Drafting the specs showed
that predicate is wrong here. Because `error_count` folds in `parseFailures`, any
real corpus containing a single unparseable file would degrade permanently — and
a `degraded` that is always on carries no information, which is the exact failure
mode ADR-0011 was written against ("volume is controlled by aggregating or by
logging transitions, never by lowering the level of a consequential event until it
disappears" — here the signal would be destroyed by over-triggering instead).

Parse failure and delivery loss are different claims. A file that cannot be parsed
produced no entities and cost the graph nothing it was promised; an entity that was
accepted and never delivered is a hole in a corpus the gate said was complete. Only
the second falsifies `ready`.

**Alternative considered:** keep the sibling predicate for consistency. Rejected —
the two sibling call sites drive a *workbench display* concern ("show this source as
unhealthy"), where over-triggering is cheap. The aggregate phase is a *gate*, where
over-triggering is as harmful as under-triggering.

### D2: Stickiness needs a per-pass baseline, not a counter test

**Decision:** each source records the publisher's loss count at the start of a pass
and compares at the end. A pass is clean when the count is unchanged across it.

`Lost() > 0` cannot express "sticky until a clean re-pass" — the counter is
monotonic, so the test would latch on the first loss and never clear, making
`degraded` permanent by construction rather than by policy. Comparing against a
per-pass baseline gives a genuine "did *this* pass lose anything" answer while
leaving the cumulative figure intact for the surfaces.

**Alternative considered:** reset the publisher's counters at pass boundaries.
Rejected — the cumulative totals are what the metrics surface and the loss figure
report; resetting them to serve the phase would corrupt both.

### D3: A pass boundary is seed completion, and a re-seed clears

**Decision:** the pass boundary is initial-seed completion. A source that finishes
its seed with loss is degraded and stays degraded until a subsequent full re-seed
or reindex completes clean. Ongoing watch activity does not clear it.

Watch-mode sources have no natural periodic pass, so tying the clear to watch
events would mean either clearing on the first quiet interval — which would let a
lossy seed be forgotten within minutes — or inventing a rolling window whose length
is arbitrary. Requiring an explicit re-pass keeps recovery deliberate and matches
the spec's "only once a subsequent pass completes with no loss".

**Trade-off, accepted:** a long-running deployment that loses entities early stays
`degraded` until someone re-seeds. That is the intended pressure — the graph really
is incomplete until then, and the loss figures say by how much.

### D4: `publish_total` is replaced by three reconciling figures

**Decision:** remove `publish_total` from `internal/sourcestatus.Report`; add
`offered_total`, `delivered_total`, and `lost_total`.

| Field | Source | Meaning |
| --- | --- | --- |
| `offered_total` | source-local counter **plus** `Publisher.Dropped()` | every entity the source handed to its publisher |
| `delivered_total` | `Publisher.Published()` | entities confirmed onto the stream |
| `lost_total` | `Publisher.Lost()` | offered but never delivered (`Failed + Dropped`) |

**Why offered, not accepted.** `Send()` returns an error when the buffer overflows,
and every source increments its counter only on a nil return — so the source-local
counter excludes drops. `Lost()` includes them. Reconciling against the raw counter
therefore fails by exactly the drop count: measured, `offered 0 != delivered 0 +
lost 3 + in-flight 1`. Adding `Dropped()` back restores the identity without
touching any increment site, because the source-local counter is exactly
`delivered + failed + in-flight`.

A drop is the publisher refusing an entity the source had. Calling that "not
offered" would hide it from the arithmetic while still counting it as lost, which
is the same class of dishonesty as `publish_total`.

**In-flight is not `Pending()`.** `Pending()` is buffer depth; an entity the drain
loop has taken but not yet resolved is in neither `Pending()` nor the terminal
counters. The identity is therefore exact only once delivery has settled, which is
what the spec says and what the tests wait for.

**Deliberate overlap:** loss appears in both `lost_total` and (via `Lost()`) in
`error_count`. This is not double-counting to be fixed — `error_count` is
specified to surface publish rejections, and removing loss from it would violate
*Source status reflects delivery truth*. The figures answer different questions.

### D5: The aggregator reads loss, not phase alone

**Decision:** `statusAggregator` gains a loss-aware condition alongside the
existing errored-phase check, and the loss-induced degradation is held in the
aggregator's existing sticky `degraded` flag rather than a new field.

The aggregator already carries `a.degraded` for seed-timeout degradation and
already clears it when every source finishes a clean seed. Loss-induced
degradation reuses that machinery with a different trigger, so no new state
concept enters the aggregate.

## Risks / Trade-offs

- **`degraded` becomes the normal state in a lossy environment** → the loss is
  real; the figures quantify it, and #178 addresses the underlying cause. If loss
  proves routine rather than exceptional, that is a signal to fix the transport,
  not to soften the gate.
- **Consumers that treat `degraded` as fatal will now fail where they passed** →
  intended, and acceptable because SemSource is currently the contract's only
  reader (`proposal.md` — Compatibility posture).
- **Strict decode means a partial rollout breaks loudly** → the shared type and
  all eight producers must land together; a producer emitting a field the shared
  type does not declare is rejected with an error log by design.
- **`accepted_total` still counts a `Send()` that succeeded into a buffer that
  later overflows** → correct by definition: it was accepted, then lost, and both
  figures say so. The reconciliation identity is what makes this legible.

## Migration Plan

1. Add the three fields to `internal/sourcestatus.Report` and remove
   `publish_total`, in the same commit as all eight producers.
2. Move the per-pass loss baseline into the seed lifecycle each source already has.
3. Change the aggregator predicate and thread the sticky flag.
4. Update the status surfaces' fixtures and the e2e assertions.

No storage migration, no reseed, no entity-ID impact. Rollback is a revert; nothing
persists in a format the previous version cannot read.

## Open Questions

- Whether `pending` should be surfaced alongside the three required figures. It
  makes the reconciliation identity checkable by a consumer rather than merely
  true, but it is a live gauge rather than a cumulative count and may read oddly
  next to the totals. Safe to decide during implementation; it changes no
  requirement and no task.
