# Design: reconcile group `source` on republish

## Context

Pin: semengine `v0.1.0-alpha.7`. Main `33b86a7`.

**Stream lane (`graph.ingest.entity`).** Each (subject, predicate, source) set in an arrival replaces the stored set
with that key, unless it is strictly older. Equal timestamps apply. Stored sets the arrival doesn't carry are kept
(`graph/helpers.go:26-37`, `ReplaceBySource`). A statement with no timestamp takes the envelope's, which
`internal/entitypub/publisher.go:456` stamps at drain time with source `semsource`. That kept-set rule is the #336 defect.

**Mutation lane (`Reconcile`).** `projection.MutationClient.Reconcile` reads the entity authoritatively, then sends one
request/reply. graph-ingest's `reconcileCandidate` (`processor/graph-ingest/entity_writes.go:354-367`) replaces the
named source's statements of the group's predicates with `Desired`, clears listed predicates that have no desired
statement, and keeps every other source's statements. When the stored statements already equal `Desired` in every field,
it reports `unchanged` and writes nothing. The write is a compare-and-set on the revision that was read.

**Writers and their sources.** Every handler stamps `semsource`. Supersession lineage edges are `supersession`
(`processor/supersession/edges.go:9`), and the stale marker is `lifecycle` (`processor/supersession/lifecycle.go:29`).
`entity.lifecycle.owner` and `entity.ontology.class` are stamped `semsource` but sit outside group `source`, because
`OwnedPredicates()` takes `source.*`, `code.*` and a few `dc.*` extras (`graph/contract.go`).

## Goals / Non-Goals

**Goals:** after a republish settles, an entity's `semsource` statements in group `source` are exactly the ones the
last publish carried. The read surfaces don't change. No new component and no upstream change are needed.

**Non-goals:** see the proposal. The main ones are #347, ownership machinery, and the upstream issue.

## Decisions

### D1. The publisher reconciles after the stream publish, and the stream publish stays

The stream publish still carries what reconcile can't: the storage reference, the message type, and the `entity.*`
statements outside the group. After `PublishToStreamWithMsgID` succeeds, `publishOne` calls `Reconcile` with:

- `Contract: SourceEntityContract().Name`, `Group: GroupSource`, `EntityID: payload.ID`;
- `Desired`: the payload's triples whose predicate is in `OwnedPredicates()`, each stamped with source `semsource` and
  the envelope's timestamp, matching what graph-ingest stores for the stream arrival;
- `Metadata.Source: semsource` and the same timestamp.

**Alternatives rejected by the ruling:** reading back and diffing, keeping producer-side memory of past emissions, and
ownership or claim machinery.

**Ordering.** The two writes converge in either arrival order. If the stream lands first, reconcile clears the dropped
predicates; with identical stamping the common case is `unchanged`. If reconcile lands first, it replaces the group
with the same statements, and the stream arrival then applies the same sets at an equal timestamp.

**Outcomes.**

- `entity_not_found`: a birth whose stream write hasn't landed yet. Nothing is stale, so it is benign and skipped.
- Revision mismatch: a concurrent write to the same ID. It is counted and logged at Debug and not retried; the next
  pass repairs it.
- Any other error is counted separately from stream delivery and never fails the entity. The statements did reach the
  graph; only the removal is outstanding.

The reconcile result never changes `Published`, `Failed` or `Lost`. Source status reports delivery, and the stream
write delivered.

### D2. Reconcile is opt-in per publisher, and folder and repo types are excluded

`entitypub.WithReconcile(mutator)` turns reconcile on. A handler passes it only once its completeness test (D3) is
green. The supersession publisher never passes it, because its payloads are partial by design (lineage edges only).
Inside the publisher, payloads of AST type `folder` or `repo` (`source/ast/predicates.go:103-104`) skip reconcile
until #347 is fixed.

The exclusion carries a `// shortcut: folder/repo excluded, remove with the #347 fix` marker in code. The #347 fix
deletes it.

### D3. The single-writer requirement is held by tests, not runtime checks

The fix is correct only if each entity ID has one writer and that writer sends the entity's complete `semsource`
statement set on every publish. Nothing enforces this at runtime. Each handler gets a test that publishes an entity
twice, with the second publish built from a source that emits less, and asserts that the second payload carries every
group predicate the handler still produces for that entity. A handler with a partial-publish path (like #347) stays off
until that path is fixed.

### D4. Cost is measured before choosing the default

The unchanged-file skip is in memory (`processor/ast-source/component.go:178`, `:230`), so a restart republishes every
entity. On OSH core (~32k entities) that is ~32k extra read + request/reply pairs. Task 4 measures restart wall clock
and the applied versus `unchanged` counts with reconcile off and on. If the overhead is acceptable, reconcile runs on
every republish. If it isn't, the bounded form is decided with the number in hand. One example would be skipping
reconcile on the first publish after a restart when the content hash matches. That choice goes to the owner and is
not pre-decided here.

### D5. ADR-0008 gets a note, not a new ADR

Retain-and-relate governs entities and versions. Within one entity ID, a source's statements are a complete set per
publish. This changes behaviour, not identity, so no storage break is needed.

## Risks / Trade-offs

- **A partial publish now deletes good data.** Before, a partial publish only failed to remove stale statements. After,
  it removes valid ones. D2's opt-in and D3's tests are the guard. #347 is the known instance.
- **`unchanged` depends on matching graph-ingest's stamping field for field** (source, timestamp, confidence,
  datatype). If they differ, every republish writes once more, which is correct but costs more. Task 4 reports the
  applied/unchanged split, so a mismatch shows up.
- **Doubled round trips on restart** until the upstream complete-set intent lands. After that the workaround is removed.

## Open Questions

- The bounded form (D4), only if the OSH number calls for it. Owner decision.
