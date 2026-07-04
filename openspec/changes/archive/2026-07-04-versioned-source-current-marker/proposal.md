# Proposal: Versioned-Source Current Marker & Historical Demotion

## Why

Item #2 (`versioned-source-supersession`, merged) retains every indexed version of a source as a
distinct subgraph and relates corresponding entities with `code.lineage.supersedes` /
`superseded_by` edges. But all versions still rank **equally**: a search for "what does `RankSignals`
do?" surfaces `v1.9.0`, `v1.10.0`, and `v1.11.0` side by side, so everyday "what does `X` do *now*"
queries are noisy. ADR-0008 §3 resolves this as a **ranking** concern, not storage: mark each source's
**current** version and **demote** its historical versions so they sink out of the way while staying
fully retained and queryable (for diff/audit). This is the tier-0, no-model completion of the
versioned-source lifecycle (ADR-0008 open item #3).

The mechanism is now buildable end-to-end: item #2 shipped the `code.lineage.superseded_by` edge (the
"a newer version of me exists" signal), and semstreams beta.130 (gh#441, now on `main` via #42) made
`vocabulary.WithWeight` **signed** — a negative weight is a *bounded reordering, never an exclusion*,
which is exactly "retained but sunk."

## What Changes

- **Demote historical versions** — register `code.lineage.superseded_by` with a **negative** salience
  weight. Any entity that has a newer corresponding version already carries this predicate (emitted by
  the item-#2 supersession pass), so it sinks in ranking; the current (un-superseded) entity is
  un-demoted and therefore ranks above its historical siblings. No new pass, no new edge, no new
  producer — pure vocabulary + the edges already in the graph.
- **"Current" is emergent, not stamped** — the current version needs no explicit marker: it is simply
  the version that carries *no* `superseded_by` edge, so demoting the superseded ones leaves it on top.
  This delivers the "what does `X` do now" ranking win with the smallest possible surface. An explicit
  `code.artifact.current` presence marker (positive boost + queryable filter, with the tracking-vs-
  pinned gate of ADR-0008 §3) is deliberately **deferred** to a follow-up (see Non-goals).
- **Retention-first** — additive only. Nothing is removed or retracted; a negative weight is a bounded
  reorder (never an exclusion), so historical versions remain fully queryable. No embeddings/LLM.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- **`versioned-source-supersession`** — adds a requirement for **demoting historical (superseded)
  versions in ranking** over the version lineage this capability already establishes. No existing
  requirement changes; this is additive.

## Impact

- **Vocabulary only** — `code.lineage.superseded_by` gains a negative `WithWeight` (it is currently
  registered with no weight). Single edit in `source/ast/vocabulary.go`. No predicate constants change,
  no producer/pass changes, no config.
- **Ranking** — historical entities fold a demotion into their rank score; the fusion ranker combines
  strongest-boost and strongest-demotion, so this reorders without excluding. Requires beta.132
  (signed salience — already on `main`).
- **Consumers** — SemSpec ("what does `X` do now" leads with current), SemDragon. Additive: historical
  results are still returned, just lower.
- **Depends on** item #2 (`code.lineage.superseded_by` edges) and beta.132 (signed salience), both
  merged to `main`.

## Non-goals

- **Explicit `code.artifact.current` presence marker** — the boost-current-marker half of ADR-0008 §3
  (positive weight, queryable current-filter, tracking-vs-pinned gate). Deferred: emergent "current"
  (un-superseded) already delivers the ranking outcome; an explicit marker is only needed once
  consumers want to *filter* to current or boost current above unrelated entities.
- **Demoting symbols removed in the newer version** — a symbol present only in an older version carries
  no `superseded_by` (nothing supersedes it), so it is not demoted. It is the latest of *itself*;
  source-version-level "this whole version is historical" demotion needs the explicit current marker
  above. Out of scope.
- **Deletion / eviction of historical versions** — retention-first; demotion is ranking-only. Deletion
  stays the framework-gated exception (ADR-0008 §5).
- **Ingestion-depth / retention-budget config** (ADR-0008 §4) — bounding *how much* is retained is a
  separate concern from ranking what is retained.
- **Cross-source global "current"** — there is no global current graph; each source owns its scope
  (ADR-0008 §3).
