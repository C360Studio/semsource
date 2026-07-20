# Design: Staleness and Retraction Markers

## Context

The audit proved live that the retention-first graph cannot distinguish a phantom from a fact:
a deleted file's entities stay served as current (`OpDelete` watch events are discarded,
`processor/ast-source/component.go`), doc edits mint a new content-hash-instance entity every
save and orphan the prior one (`handler/doc/entities.go` — 6-hex prefix, collision-prone),
`remove_source` leaves ingested entities permanently indistinguishable, and `watch:false`
"snapshots" silently reindex every 60s. ADR-0008's retain-&-relate stays: deletion by policy is
graph-unsafe; this change lands the deferred staleness/retraction *exception* as markers +
demotion, with hard deletion still gated on upstream asks (#15/#433).

Product decisions were made with the user (2026-07-19), all four as recommended: true-freeze
snapshots; path-anchored doc identity with in-place replace; retain-and-demote with unbounded
stale accumulation until upstream GC exists; async retro-marking on source removal.

Mechanism verification (pre-design): the substrate's update lane supports predicate clearing —
`graph.mutation` `UpdateEntityWithTriplesRequest.RemoveTriples` is a pure per-predicate delete
("Triples ARE clearable") — so a recreated file can shed its marker without any framework ask.
Enumeration at scale reuses the `QueryPrefixAll` pattern the supersession pass already proves.

## Goals / Non-Goals

**Goals:**

- Every query surface can distinguish live facts from retained-stale facts: a governed marker,
  negatively weighted, filterable, badgeable.
- Deletes/renames publish typed state changes within one index interval; nothing is silently
  discarded.
- Doc identity survives edits: one live entity per document path, no orphans, no 24-bit
  collisions.
- `remove_source` leaves removal provenance on retained entities (async, bounded, observable).
- `watch:false` really freezes; snapshot sources are exempt from staleness.

**Non-Goals:**

- Hard deletion / GC of stale entities (upstream #15 cascade-delete + #433 index cleanup remain
  the gate; the marker is designed so GC can later key on it).
- Doc edit HISTORY in the graph (declared versions via version-registration-surface are the
  mechanism when history is wanted; an undeclared edit is an update, not a version).
- Rename *tracking* (a rename appears as delete-at-old-path + add-at-new-path; relating the two
  is the deferred rename-tracking item from ADR-0008).

## Decisions

### D1: One marker predicate, reason-valued, weighted below history

`entity.lifecycle.stale` (source vocabulary), value = reason: `file_deleted` |
`source_removed` | `path_missing`. Registered `WithWeight(-3.0)` — strictly below
`code.lineage.superseded_by`'s −2.0: a stale fact ranks under a historical-but-alive version.
Presence-based (salience is per-predicate), cleared via `RemoveTriples` when the artifact
reappears. One predicate, one weight, one filter key for consumers; the reason rides in the
value where signals don't need it.

### D2: Graph-derived delete detection in a lifecycle pass

A `lifecycle` pass (housed in the supersession component — same enumeration machinery, same
trigger shape: NATS subject + optional interval) diffs the graph against the filesystem per
watched source: enumerate the source's entities via prefix, group by `code.artifact.path` (and
the doc path predicate), stat the path under the source root — missing → write the marker on
every entity of that path (update lane, `AddTriples`); present and previously marked → clear
(`RemoveTriples`). Graph-derived, so it is restart-proof: deletes that happen while semsource
is down are detected on the first pass after boot, with no persisted inventory to drift. The
fsnotify `OpDelete`/`OpRename` handlers additionally trigger an immediate targeted mark for the
affected path (fast path; the periodic pass is the safety net).

*Alternative considered:* per-component in-memory inventories. Rejected — misses offline
deletes, duplicates state the graph already holds.

### D3: Doc identity = path instance; edits replace in place

`handler/doc` instance becomes the sanitized path slug (exactly the code-file convention);
`sha256(content)` moves to a content-hash predicate (change detection), and the body handle
stays content-addressed in the store. An edit re-ingests the SAME entity — the substrate's
per-predicate replace updates content triples in place: one live entity per document, prior
content gone from the graph (its offloaded body lingers harmlessly, content-addressed).
**BREAKING for existing doc entity IDs**; migration = one reindex, accepted now while the
install base is small. URL/page identity (hash of canonical URL) is already stable and is
untouched.

### D4: `remove_source` retro-marks asynchronously

Removal keeps its current contract (immediate `removed:true`, aggregator drop, NOT_FOUND for
unknowns). It additionally triggers the lifecycle pass scoped to the removed source's prefix,
which writes `entity.lifecycle.stale = source_removed` markers. Contract: markers observable
within one pass interval (pinned by test, minutes not seconds); the pass is idempotent and
resumable (re-running converges — same diffNew shape supersession uses).

### D5: `watch:false` is a true freeze, and frozen sources never go stale

`watch:false` disables BOTH the watcher and the periodic reindex unless `index_interval` is
explicitly set (an explicit interval is an explicit request). The lifecycle pass skips
non-watched sources entirely: a pinned snapshot (ADR-0007 ephemeral worktree, GC'd after
ingest) is a point-in-time truth whose backing path is EXPECTED to vanish — marking it stale
would poison exactly the sidecar use case. Staleness is a property of sources that promised to
track reality. Docs updated (`add_source` description already says "one-shot snapshot" — it
becomes true).

### D6: Typed events, not silent drops

The watch paths' delete/rename arms publish the same typed state-change shape other mutations
use (satisfying typed-source-change-events): marker writes go through the update lane with the
semantic envelope; nothing infers deletion from absence-of-traffic.

## Risks / Trade-offs

- [Marker flapping on transient FS states (editor atomic-save windows)] → the fast path debounces
  (only marks on OpDelete when the path is still absent after the existing coalesce window);
  the periodic pass self-heals any residue by clearing markers for present paths.
- [Unbounded stale accumulation] → accepted explicitly (user decision); markers are the future
  GC key, and consumers filter/demote today. Upstream asks #15/#433 remain the cleanup gate.
- [Doc-ID break ripples to consumers holding doc handles] → release notes + reindex migration;
  handles were already unstable across edits (every edit changed them), so no consumer can have
  been depending on doc-handle stability.
- [Update-lane writes per deleted path could be large on mass deletes] → bounded per pass by
  the same max-entities budget supersession uses; idempotent resume covers the tail.
- [Freeze semantics change surprises existing watch:false users] → their documented contract
  was always "snapshot"; release notes call out `index_interval` as the opt-back-in.

## Migration Plan

Doc IDs: breaking, migration = reindex (documented). Freeze: behavior change matching
documentation; `index_interval` restores periodic reindex per source. Markers/lifecycle pass:
additive. Rollback = revert; markers are inert data to consumers that ignore them.

## Open Questions

- None blocking. (Marker clearing verified against the substrate's RemoveTriples; if the
  update lane's envelope requirements surprise us in implementation, that surfaces as an
  ask before shipping, not after.)
