# Republish removes predicates a source stopped emitting

Approved-by: Coby Leuschke (2026-11-23) sha256:c87d3b898461

Track: design (a new graph-write path, one requirement in `entity-publish-integrity`, an ADR-0008 note).

## Why

When an entity ID is republished with fewer predicates than last time, the old ones stay (#336). The repro
`TestIntegration_ReingestDropsRemovedPredicates` (`internal/governance/stale_predicate_integration_test.go`) runs the
real ast-source → graph-ingest → `ENTITY_STATES` path on semengine `v0.1.0-alpha.7`. A function edited to drop its call
and its doc comment keeps both `code.relationship.calls` and `code.doc.comment`.

It happens on every republish that emits less: a watch edit, a periodic reindex of a changed file, and a restart over a
retained store. The stream lane applies `ReplaceBySource` per carried (subject, predicate, source) set, and a predicate
the arrival does not carry is never touched.

The fix needs no upstream change. `graph/contract.go` already declares group `source` as `ModeReconcile` over
`OwnedPredicates()`, and no write path uses it. `processor/supersession/lifecycle.go` already drives the same
`projection.MutationClient.Reconcile` for the stale marker.

Ruling: [#336 comment 7100421719](https://github.com/C360Studio/semsource/issues/336#issuecomment-7100421719), amended by
[7100564191](https://github.com/C360Studio/semsource/issues/336#issuecomment-7100564191).

## What Changes

- **Reconcile after republish.** The shared publisher (`internal/entitypub`) follows a successful stream publish with
  one `Reconcile` of group `source` for that entity. Its desired set is the statements the producer just built, limited
  to the group's predicates. A group predicate with no desired statement is cleared. Births need no special path: an
  absent entity returns `entity_not_found`, which counts as nothing to clear.
- **Opt-in per handler.** Reconcile is a publisher option. A handler is switched on only after a test proves its
  publish carries the entity's complete statement set (the single-writer requirement below). The supersession publisher
  never enables it.
- **Folder and repo entities are excluded** until #347 is fixed, because the AST watch path republishes them with a
  partial `code.contains` set.
- **Single-writer requirement, stated and tested.** Each entity ID has one writer that sends its complete set for
  source `semsource` on every publish. Nothing enforces this at runtime. Review and one test per handler hold it.
- **Cost measured before the default is chosen.** The unchanged-file skip is in memory, so a restart republishes every
  entity: about 32k reconcile round trips on OSH core. Measure that before choosing "always reconcile" over a bounded
  form.
- **ADR-0008 note.** Retain-and-relate governs entities and versions. Within one entity ID, a source's statements are a
  complete set per publish.

Consumers: SemTeams and SemDragon read the corrected statements through graph query, MCP and GraphQL. The read surfaces
do not change.

## Non-goals

- #347 itself (partial folder republish on the watch path). This change only keeps folders and repos out of reconcile.
- Any read-back-and-diff, producer-side memory of past emissions, or ownership, registry or claim machinery (ruled out).
- Clearing `entity.lifecycle.owner`, `entity.ontology.class`, lineage edges (source `supersession`) or the stale marker
  (source `lifecycle`). They sit outside group `source` or under another source, so the reconcile cannot touch them.
- Entity deletion and staleness, which the lifecycle pass handles.
- The upstream SemEngine issue asking for a per-message complete-set intent on the stream lane. It is filed after this
  fix and the OSH cost number exist (rule 2: issue only).
- An identity break or reseed. IDs do not change.
