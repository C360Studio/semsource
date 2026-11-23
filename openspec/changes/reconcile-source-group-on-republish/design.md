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
`entity.ontology.class` is stamped `semsource` but sits outside group `source`, because `OwnedPredicates()` takes
`source.*`, `code.*` and a few `dc.*` extras (`graph/contract.go`). The lifecycle owner fact is
`source.lifecycle.owner`, so it is inside the group, and every handler publish carries it.

## Goals / Non-Goals

**Goals:** after a republish settles, an entity's `semsource` statements in group `source` are exactly the ones the
last publish carried. The read surfaces don't change. No new component and no upstream change are needed.

**Non-goals:** see the proposal. The main ones are #347, ownership machinery, and the upstream issue.

## Decisions

### D1. The publisher reconciles after the stream write applies, off the drain loop

The stream publish still carries what reconcile can't: the storage reference, the message type, and the `entity.*`
statements outside the group. After `PublishToStreamWithMsgID` succeeds, `publishOne` queues a reconcile job, which a
single worker runs off the drain loop so a slow mutation lane never delays delivery. The job's `Reconcile` carries:

- `Contract: SourceEntityContract().Name`, `Group: GroupSource`, `EntityID: payload.ID`, `ExpectedRevision` (below);
- `Desired`: the payload's statements whose predicate is in that contract's `source` group and whose source is
  `semsource` or empty, an empty source stamped `semsource` and a zero timestamp the envelope's (fixed at millisecond
  precision), matching what graph-ingest stores for the stream arrival. The envelope time is the wall-clock
  millisecond, never earlier than the previous publish's; only a repeat of one entity within a millisecond moves it
  on by 1 ms, so no two publishes of an entity share it and a burst does not run ahead of the clock;
- `Metadata.Source: semsource` and no metadata timestamp, which the client would refuse against a statement's own.

**Alternatives rejected by the ruling:** reading back and diffing, producer memory of past emissions, ownership.

**Ordering (amended 2026-11-23 after review).** A stream write is acknowledged when stored, not when applied. A
reconcile that commits before an older queued write of the same entity lets that write restore a predicate the
reconcile removed (#346 review: two publishes in flight, e.g. an edit during a seed backlog). graph-ingest applies one
entity's writes in arrival order (`keyed_ingest.go`, SemStreams ADR-072). So the job reads the entity until the stored
state holds every desired statement (this write applied, hence every older one), then replaces the group fenced at
that read's revision. A lost compare-and-set re-reads. A job is skipped once a later publish of the entity is queued.

**Outcomes.** The job's budget is 30 s, with 5 s per request and a 20–500 ms backoff between reads. A full queue
(4,096), a read or replace error, or a write not applied within the budget is counted on `reconcile_failed_total`; the
next edit repairs it. A job abandoned at Stop is not counted. The reconcile never changes `Published`, `Failed` or
`Lost`. Queued jobs count as in flight for `WaitSettled`.

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

### D4. Reconcile only content the producer knows changed (owner ruling, task 5.2)

Measured on OSH core (5.1): reconciling every republish made a warm restart settle in 294–377 s instead of 46–49 s,
with status `degraded` (seed timeout), and it removed nothing, because the corpus had not changed.

The ruling ([#336 comment 7104692071](https://github.com/C360Studio/semsource/issues/336#issuecomment-7104692071)) is
a bounded form. ast-source reconciles a file's entities only when this process knows the file's content changed:

- a watch edit;
- a reindex of a file whose in-memory hash differs from the one this process recorded.

The seed, and a reindex of a new or unchanged file, publish with `SkipReconcile`, the fault guard's flag.

**Known gap:** a file edited while semsource was down keeps its dropped predicates until its next edit. The upstream
per-message complete-set intent (semengine#730) closes it. When that ships, the local reconcile and this bound come out.

**Reconcile-before-publish is rejected.** Reconcile equality compares timestamps (semengine alpha.7
`processor/graph-ingest/canonical_mutations.go:613-618`), and every republish stamps a new one, so it would never be
`unchanged`. That means about two entity writes per republish (read from code, not measured).

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

None. D4 was decided by the owner's ruling on task 5.2.
