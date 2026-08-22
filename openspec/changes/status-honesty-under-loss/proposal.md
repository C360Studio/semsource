# Status surface honesty under delivery loss

Closes [#177](https://github.com/C360Studio/semsource/issues/177). Companion to
#176 (publish-error classification) and #175 (the flood that exposed this);
neither is in scope here.

## Why

The documented consumer gate — poll `graph.query.status` until `phase: "ready"` —
is claimable while a third of the seeded corpus never reached the graph. During
beta.161 OSH acceptance (boot A), status reported `publish_total: 77,802` and
`phase: ready` against a publisher that had delivered 42,931 and terminally
failed 34,871. Every downstream consumer that honours the gate proceeded to query
a graph missing 45% of what the headline number claimed.

Both halves are already prohibited by specs SemSource has shipped; this change is
mostly conformance, plus resolving one contradiction between two requirements.

### Compatibility posture

SemSource is currently the only reader of this status contract. No external
consumer is gating on it in anger yet, so this change takes the clean break
rather than a compatibility shim: fields are renamed where their meaning changes,
and the aggregate phase is allowed to start refusing cases it previously passed.
Recorded here because a later reader will otherwise ask why a documented consumer
contract was broken so freely — the answer is that the window to do it cheaply is
now, before the sem\* consumers depend on it.

## What Changes

**Delivery figures are rebased on publisher-confirmed counts.**

- `publish_total` SHALL be **removed and replaced**, not silently redefined. A
  field that keeps its name while changing meaning is the worst available
  outcome: every existing reader keeps parsing it and quietly gets different
  semantics, with nothing to fail against. Replacement names make the break
  legible at the point of use.
- The replacement figures are `offered_total`, `delivered_total`, and
  `lost_total`, derived from the publisher, which already knows all of them:
  `Published()` (confirmed onto the stream), `Failed()` (left the buffer, never
  arrived), `Dropped()` (refused at the buffer on overflow), and `Lost()`
  (`Failed + Dropped`). None of these reach a status surface today. `offered_total`
  counts drops as well as accepted hand-offs, so the three figures reconcile —
  see `design.md` D4.
- The source-local counters that feed `publish_total` today increment at
  `Publisher.Send()`, which returns after a buffer write and before any delivery
  outcome exists. They are not confirmation and cannot be made into it.
- **BREAKING (wire):** `publish_total` disappears from every status surface.
  Deliberate — see Compatibility posture. No reseed and no entity-ID impact.

**`ready` stops being claimable after a lossy seed.**

- `statusAggregator.anyErrored()` (`processor/source-manifest/status.go:172`)
  currently tests only `r.Phase == SourcePhaseErrored`. It SHALL widen to the
  predicate two sibling call sites in the same package already use —
  `Phase == SourcePhaseErrored || ErrorCount > 0 || LastError != nil`
  (`workbench_capabilities.go:233`, `component.go:839`).
- The aggregate therefore reports `degraded`, sticky until a clean re-pass.
  **Decided:** reuse the existing `degraded` phase rather than introduce a
  `ready_with_loss` value. The original rationale was partly compatibility, which
  the posture above retires; the decision stands on the remaining merit, which is
  that two call sites in this package already treat error-bearing sources as
  degraded and a third state would make the aggregate the odd one out again. The
  distinction a `ready_with_loss` value would encode — clean seed versus lossy
  seed — is carried precisely by the loss figures instead, where it is a count
  rather than a boolean.
- **BREAKING (behavioural):** deployments that previously observed `ready` after
  a lossy seed will now observe `degraded`. This is the point of the change —
  the gate begins refusing what it should always have refused — but it will
  surface as new failures in consumers that treat `degraded` as fatal.

## Capabilities

### New Capabilities

None. Both gaps are governed by requirements that already exist.

### Modified Capabilities

- **`entity-publish-integrity`** — *Source status reflects delivery truth* already
  requires that status "count an entity as ingested only after confirmed hand-off
  to delivery", and that entities "dropped by the publisher" be "excluded from the
  ingested/confirmed count". Entities that enqueue cleanly and then fail
  terminally during delivery are counted today, violating that scenario. The
  delta pins the ambiguous phrase "confirmed hand-off to delivery" to
  `Publisher.Published()`, so the requirement cannot be read as satisfied by a
  successful buffer write.
- **`ingestion-readiness`** — two requirements currently contradict each other and
  the delta must resolve them, not merely add to them:
  - *Ready means seeded* states the consumer gate "guarantees the initial corpus
    is fully published" — the strong reading, which the implementation does not
    honour.
  - *A seeding source reports progress, not just its endpoints* states "`ready`
    continues to mean every configured source finished its initial seed" — the
    weak reading, which the implementation does honour and which permits loss.
  - Resolved in favour of the strong reading, consistent with the ADR-0011 rule
    that a condition silently breaking a user-visible guarantee (readiness,
    seed completion, delivery) is never reported below `Warn`.
  - The same requirement's "cumulative confirmed-delivery count" for progress
    reports is conformance, not a new obligation.

## Impact

**Code.** `internal/sourcestatus.Report` (the shared contract from #188);
`processor/source-manifest/status.go` (aggregator predicate and report mapping);
all eight source components, each of which currently sets `PublishTotal` from a
local enqueue-side counter — `entitiesIndexed` in `ast-source`, `entitiesPublished`
in `cfgfile`, `doc`, `git`, `image`, `url`, `audio`, and `video`. No source is
correct today; `ast-source` differs only in the counter's name.

**Surfaces.** Every status surface carrying the shared report: `graph.query.status`,
HTTP status, MCP status tools, and the workbench capabilities route.

**Consumers.** SemSpec, SemDragon, SemOps, and SemTeams (workbench) all gate on
`phase: "ready"` before querying. They gain a correct gate and lose the ability
to proceed against a partial corpus.

**Substrate.** None. Delivery guarantees, retry, and stream behaviour are
SemStreams'; this change only reports what the publisher already knows.

**Storage.** None. No reseed, no fresh NATS storage, no entity-ID impact.

## Non-goals

- **Classifying publish errors.** Which failures are terminal versus transient is
  #176 and the `entity-publish-integrity` requirement that already covers it.
- **Reducing the failures themselves.** The 34,871 losses trace to the GRAPH
  256MiB ceiling under investigation in #178, which is blocked on SemStreams.
  This change makes loss *honest*, not rarer.
- **A new phase value.** `ready_with_loss` was considered and rejected above.
- **Changing clean-seed semantics.** A seed that loses nothing still reports
  `ready`, unchanged.
- **Recovery orchestration.** Automatic re-publication of failed entities is
  out of scope; `degraded` is sticky until a clean re-pass, and clearing it is
  operator-driven.
- **Substrate work.** Any gap found in SemStreams' delivery guarantees goes to
  `docs/upstream/semstreams-asks.md` and a GitHub issue, never a PR.
