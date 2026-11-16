# Design: lifecycle sweep scoped to its producing root

## Context

The lifecycle pass (`processor/supersession/lifecycle.go`, `runLifecyclePass`) enumerates every
entity under the deployment prefix and keeps those whose entity-ID system is in
`req.Systems`. It then stats each entity's path predicate under `req.RootPath` (or checks it
against `req.Absent`). Three sources send these requests:

- ast-source: one request per watch path per sweep (`lifecycleRunRequestFor`, `component.go`
  near `:646`), and on fsnotify deletes.
- doc-source: one request per root on deletes (`triggerLifecycleRun`, `component.go:527`).
- objectstore-source: one absent-set request per completed listing (`retractionRequest`,
  `component.go:348`).

A system slug comes from `project` (plus version for AST). It is shared by every root configured
with that project, by both instances of a repo expansion, and by every path of one multi-path
instance. Nothing on an entity says which root produced it, so the pass cannot tell a foreign
entity from its own. #293 at `d1b50cf` added `path_predicate`, which separates code from
documents but not two document roots (reproduced in the #293 review).

## Goals / Non-Goals

Goals: a sweep marks or clears only entities its own root produced; the request states that
root's owner explicitly (the pass never infers scope); both #286 cases are kept tests.

Non-goals: changing entity identity; solving two roots that mint the same entity ID; owner facts
on sources that never sweep.

## Decisions

### D1. An owner fact on the entity, not a scope inferred from the ID

New predicate `source.lifecycle.owner` (registered in `source/vocabulary/lifecycle.go`; it
falls under `OwnedPredicates()` because it starts with `source.`). Object: the owner key
string. Written in the reconcile-mode `source` group, so every publish of the entity sets it
to the publishing root's key and a re-ingest repairs it.

Rejected: encoding the root in the entity ID (identity-breaking, against rule 1); a side bucket
mapping IDs to owners (a second source of truth that can drift from the graph; `entity-or-bucket`
says per-entity facts belong in triples).

### D2. Owner key = instance name plus a digest of the root

`graph.LifecycleOwner(instance, root string) string` returns
`<instance>/<first 12 hex of sha256(cleaned absolute root)>`. For objectstore the "root" is the
bucket name.

- The instance name separates an AST instance from a docs instance at the same root (repo
  expansion gives both the same local path).
- The root digest separates the paths of one multi-path instance, and two instances that end up
  with the same name.
- The digest keeps absolute local paths out of graph facts.
- Deterministic and intrinsic to configuration: restarts keep the key. A moved checkout gets a
  new key, and its seed re-publish restamps every live entity.

### D3. The request carries the owner; the pass filters on it

`LifecycleRunRequest.Owner string json:"owner,omitempty"`. With `root_path` or `absent` set it is
required, else refused (`livenessOracle`). In-scope means system in `Systems` **and** owner
fact equal to `Owner`. `path_predicate` from #293 is removed: owner scoping subsumes it, and two
required scope fields that can disagree are worse than one.

### D4. Stamp where the source knows the root

- ast-source: in `publishParseResult` (has `pw`), for every entity of the parse result.
- doc-source: where a file's payloads are built for a resolved root (`rootForPath` resolves it
  on the watch path; the seed pass iterates roots).
- objectstore-source: in `publishDocument`, with the bucket.

Hierarchy and folder entities carry no path predicate and are never judged; they need no owner.

## Risks / Trade-offs

- **Unowned entities are never judged.** An entity written before this change has no owner, so
  no sweep marks it; a file deleted while the service was down before its first restamp stays
  unmarked. Mitigation: every start seeds a full pass that restamps all live entities, and the
  SemEngine cutover already requires fresh storage plus re-ingest. Accepted for the Tier 0 tag.
- **Identity collisions flap the owner.** Two roots that mint the same ID reconcile the owner fact
  back and forth. That case was already wrong before this change (one entity for two files);
  recorded as a known limit.
- **One more triple per entity.** Bounded: a ~40-byte string on entities of three source kinds.

## Migration Plan

No data migration. Callers are all in this repository and change in the same PR. A stale build
sending the old shape is refused loudly by the pass (soft miss at the caller, logged degraded).

## Open Questions

None blocking. If an external caller of `graph.lifecycle.run` appears, it must send `owner`.
