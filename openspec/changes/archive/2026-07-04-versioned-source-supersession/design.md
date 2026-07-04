# Design: Versioned-Source Supersession

## Context

Open item #1 (PR #39) gives each indexed version a **distinct** `system` ID segment
(`entityid.ScopedSystemSlug(project, version)`), so versions coexist as disjoint subgraphs. This change
**relates** them. Two facts from #1 shape the design:

- The version is **folded into** the `system` slug and `SystemSlug`-capped → **not** recoverable by
  parsing the ID.
- For the same reason, the **project** (source identity) is folded in too → also not recoverable by
  parsing the ID.

So neither "which version is this?" nor "which entities belong to the same source?" can be answered from
the entity ID. Both must come from **triples**.

## Goals / Non-Goals

**Goals:** deterministic (tier-0, no model) cross-version correspondence and supersession edges over code
entities; end-to-end without a caller (index two versions → get lineage); additive and retention-safe.

**Non-Goals:** renames/moves (tier-1 embeddings), semantic summaries (tier-2), the "current" ranking
marker (#3), changeset modeling (#4), any deletion/retirement. Non-code sources.

## Decisions

### D1. Emit two triples at ingest: version + source identity

At ingest (ast-source), each code entity carries, in addition to today's triples:

- **`code.artifact.version`** — the raw registered version string (from the `version` config field, #1).
  Emitted only when `version` is non-empty (backward compatible — version-less entities carry neither).
- **`code.artifact.project`** — the **version-independent** source identity (the raw `project`). This is
  the signal that groups *versions of the same source*; without it, sibling versions can't be found
  (the project is not recoverable from the ID — see Context).

Both follow the existing `code.artifact.*` family and the presence-marker pattern of task #38. Emission
lives in the ast-source triple builders, not the correspondence pass.

*Alternative considered:* emit only the version triple and have the pass be **caller-driven** (told which
two scopes to compare, e.g. by a future refresh-on-bump trigger). Rejected for v1: it makes #2 unable to
do anything standalone, and defers value onto an item not yet built. The source-identity triple is a
cheap, self-describing enabler.

### D2. Correspondence + supersession is a new graph-level `processor/` component

The pass must see **both** version scopes at once, which ingestion (one scope per watch path) cannot. It
is code-semantic (path/name/type/package matching) — **product-shaped, ours**, not SemStreams substrate.
So: a new `processor/` component (working name `supersession`) following the semstreams component pattern
(config.go / component.go / factory.go).

*Alternatives considered:* (a) extend ast-source — can't see sibling scopes without cross-scope queries,
couples ingestion to graph reads; (b) reuse semstreams `graph-clustering` — that's substrate and
cluster-generic, not code-correspondence; bending it violates the Product Boundary. Rejected.

### D3. Discovery + grouping algorithm (deterministic, O(n) not O(n²))

1. Enumerate candidate code entities via `graph.query.prefix` (paginated), reading their
   `code.artifact.project`, `code.artifact.version`, `code.artifact.path`, name, `CodeType`,
   `CodePackage`, `code.artifact.body` triples.
2. **Group by `project`** → the set of versions of one source.
3. Within a project, **key each entity** by the version-independent tuple `(path, name, type, package)`
   using a hash map. Entities sharing a key but differing in `version` are **corresponding** across
   versions.
4. **Order** each correspondence group's members by version (D4) and emit supersession edges (D5).

Grouping is hash-based (linear), not pairwise (quadratic) — matters at the 10k-entity scale (#430).

### D4. Version ordering = semver-aware, with a documented fallback

Supersession is directional ("newer supersedes older"), so versions need a total order. Use a
**semver-aware comparator** (`v1.9.0 < v1.10.0`). Non-semver "versions" (branch names, commit SHAs) have
no semver order → **fallback to the entity `IndexedAt` timestamp** of first index; if still ambiguous,
treat the pair as **incomparable → coexist with no supersession edge** (never guess a direction).

### D5. Supersession edges: adjacent, directional, idempotent, retention-safe

- Predicate **`code.lineage.supersedes`** (relationship triple; subject = newer entity ID, object =
  older entity ID). Emit the inverse **`code.lineage.superseded_by`** for query convenience.
- Emit **adjacent-version** edges over the currently-known version set (chain: v1.8→v1.9→v1.10), not
  transitive-to-latest — simpler and re-derivable.
- Edges are **deterministic triples** (fixed subject/predicate/object). *As-built correction:* the
  graph-ingest merge does a bare `append` with **no de-duplication**, so re-emitting an existing edge
  would duplicate it — the pass is made idempotent by **pre-diffing** desired edges against the triples
  read during enumeration (`diffNew`), publishing only the delta. The pass **never removes** anything
  (retention-first); a version inserted between two existing ones adds new adjacent edges and leaves
  prior edges (acceptable staleness, never data loss).
- Writes go through the entity-publish path; the BaseMessage envelope (semstreams ADR-055) is applied by
  the publisher. Edge-only payloads are **not** re-`StampClass`ed — the target entity is already
  classified, and re-stamping would append a duplicate class triple.

### D6. "Changed" is a property of the correspondence, from body-hash

A corresponding pair is **changed** when its verbatim-body content hash differs, **unchanged** when
identical. *As-built correction:* the hash is carried by **`code.body.key`** (the offloaded body's
`code:<sha>`), falling back to `code.artifact.hash` — the `code.artifact.body` predicate named in an
earlier draft does not exist. Represented by a companion triple **`code.lineage.change =
changed|unchanged`** on the newer entity (omitted, never guessed, when a body hash is unknown), so "what
changed between v1.9 and v1.10" is a direct query over supersedes edges filtered to `changed`.

## Risks / Trade-offs

- **Non-semver versions have no total order** → Mitigation: timestamp fallback, else coexist without an
  edge (D4). Never emit a guessed direction.
- **Scale** (mass re-scan at 10k entities) → Mitigation: hash grouping (D3), paginated prefix queries,
  and run on a bounded trigger (Open Question OQ1), not per-file.
- **Intermediate-version insertion leaves a skip edge** → Mitigation: adjacent edges are additive and
  re-derivable; acceptable under retention-first. A future compaction is out of scope.
- **Correspondence false match across sources** → Prevented by grouping on `project` first (D3 step 2).

## Migration Plan

Purely additive: new triples + new component. No existing IDs or triples change; version-less sources are
untouched (D1). Rollback = disable/remove the component; the emitted triples are inert (no consumer
breaks). Predicates are new vocabulary entries.

## Open Questions

- **OQ1 — Trigger:** when does the pass run? Candidates: on a version scope completing its initial index
  (reactive), periodic, or on-demand (config/API). Recommendation: reactive on index-complete for v1,
  falling back to periodic; confirm against how sources are registered/refreshed.
- **OQ2 — Predicate names:** `code.artifact.version` / `code.artifact.project` / `code.lineage.supersedes`
  / `code.lineage.superseded_by` / `code.lineage.change` — confirm naming against `source/ast/predicates.go`
  conventions and the vocabulary registry before implementing.
- **OQ3 — Same-source detection when project names collide** (two genuinely different repos both named
  `utils`): is raw `project` sufficient, or does source identity need the org/root too? Likely fold `org`
  into the grouping key; confirm in tasks.
