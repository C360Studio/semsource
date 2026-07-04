# ADR-0008: Versioned-Source Retention & Supersession — A Temporal Knowledge Graph

> **Status:** Proposed | **Date:** 2026-07-04
> **Supersedes two withdrawn drafts of this ADR:** (1) a *fact-contribution provenance ledger* — over-engineered; solved a cross-branch-unification problem we do not have; (2) a leaner *versioned-source lifecycle / scoped retraction* — still deletion-centric, and a graph cannot safely delete by policy. Both were killed by adversarial review + use-case analysis; see "Rejected alternatives".
> **Revises:** ADR-0007's branch-deletion debate (Options A/B/C) — replaced by **retain + relate, don't delete**.
> **Absorbs:** task #36 (dependency-version lifecycle) — "refresh-on-bump" becomes *index the new version + add supersession edges*, additive, no deletion.
> **Framework-gated only for the rare deletion exception (OFF the critical path):** [semstreams#433](https://github.com/C360Studio/semstreams/issues/433) (index cleanup on delete) + a referential-cascade-delete primitive (candidate ask). Neither gates the retention core.

## Context

SemSource ingests dependencies, the working repo, docs and configs into a knowledge graph that
SemSpec and coding agents query. The driving new need (task #36): when a dependency **bumps**, capture
the change so an agent can ask *"what changed between `beta.129` and `beta.130`?"* — and so a paid,
hours-long SemSpec run can later **prove which graph view it used** (ADR-0007 §5).

This ADR arrived at its model by discarding two earlier ones. That history is the argument, so it is
kept short and explicit:

1. **Supersession is a relationship, not a deletion.** For the dependency-upgrade use case, the
   *relationship between versions* is the value. Deleting `beta.129` when `beta.130` lands destroys the
   ability to answer the very question the feature exists for. Every "retire the old version" design
   optimizes for throwing away the payload.
2. **A graph cannot safely delete by policy.** Reference-blind eviction (NATS TTL / MaxBytes /
   discard) removes a node without knowing what edges point at it → **orphaned edges** → integrity
   loss. Even an *explicit* delete leaves dangling assertions on the referrer (the triple `B —calls→ A`
   lives on `B`; deleting `A` does not rewrite `B`). Deletion is structurally hostile to a related
   graph, so it cannot be the default.

The model therefore **inverts**: **retain and relate by default; "current" is a ranking concern;
deletion is a rare, graph-aware exception for mistakes and churn, off the critical path.**

### Grounded facts (verified in `semstreams@v1.0.0-beta.129/130` + semsource)

- **Versions are already distinct subgraphs.** `entityid.SystemSlug` maps a versioned path
  `…/semstreams@v1.2.3/…` → `semstreams-v1-2-3`; the version lives in the `system` ID segment
  (`entityid/entityid.go:49`). `v1.9` and `v1.10` share no entity IDs.
- **Cross-version correspondence is deterministic for the common case — via triples, not ID parsing.**
  Two versions of the same symbol have identical IDs *except* the `system` segment, but (per open item
  #1, now implemented) the version is **folded into** `system` and `SystemSlug`-capped, so it is **not**
  cleanly recoverable by string-parsing the ID. Correspondence instead matches on the **version-
  independent triples** every entity already carries — `code.artifact.path`, name, `CodeType`,
  `CodePackage` — with the version supplied by an explicit **version triple**. Differing
  `code.artifact.body = code:<sha>` → "changed".
- **Bodies dedup by content hash in ObjectStore** — unchanged symbols across versions share one blob;
  retention's marginal cost is entities + triples, not code bytes.
- **Seeds already exist:** the git handler builds commit + branch entities; triples carry
  `Timestamp` / `ExpiresAt`; a presence-marker + **signed** predicate salience (task #38 +
  beta.130 / semstreams#441) is exactly the "boost current, demote historical" mechanism.
- **Deletion is doubly incomplete in the framework:** #433 (query indexes not cleaned on delete) *and*
  referential integrity (referrer triples not cascaded). So deletion is framework-gated regardless of
  transport.

## Decision

### 1. The graph is temporal — retain versioned sources, relate them by supersession

Each versioned source (a dependency at a version; a coherent checkpoint) is a distinct, ID-scoped
subgraph, and versions **coexist**. On a new version: index it as a new scope, **add supersession
edges** relating corresponding entities across versions, and **move the "current" marker** (§3).
**Nothing is deleted.**

### 2. Supersession & changesets are tier-0 deterministic; models are upper-tier enhancements

Be honest about what each capability costs:

| Capability | Tier | Mechanism | Model? |
|---|---|---|---|
| "Same symbol across versions" (stable name/path) | **0 — code** | match version-independent triples (path/name/type/pkg) + a version triple | No |
| "What changed v1.9 → v1.10" | **0 — code** | body-hash set-diff over the two scopes | No |
| Changeset → touched symbols (commit/PR entity) | **0 — code** | git-diff hunks → stored symbol line-ranges → edges | No |
| Rename / move correspondence | **1 — embeddings** | body/signature similarity (semembed) | Yes (minority of changes) |
| Semantic change summary | **2 — LLM** | seminstruct over the diff | Yes (far later) |

The valuable core is **tier-0 deterministic code — no model.** Models only buy rename-tracking and
prose summaries, both additive and deferrable; they must not gate the core.

**Load-bearing prerequisite (open item #1 — DONE):** intentional version scoping so each version gets a
**distinct** `system` segment (`entityid.ScopedSystemSlug` = `SystemSlug ∘ VersionScopedSlug`). This is
what makes "versions coexist as distinct subgraphs" true. It deliberately does *not* make the version
recoverable from the ID (folded in and capped) — which is *why* correspondence is triple-based (above),
not ID-parse-based. Emitting the **version triple** that correspondence keys on is part of open item #2.

### 3. "Current" is a per-source ranking marker — not storage, not global

There is **no global "current" graph.** Each source owns its scope; a cross-source query returns each
source's current-marked view, and a consumer narrows by filtering to the sources it cares about.

- The marker is only meaningful for **tracking** sources (a watched repo's HEAD, a doc/URL's last
  fetch). A **pinned** `dep@version` is *self-current* and carries no marker.
- Mechanism: a low-cardinality presence marker (`…current`, the task-#38 pattern) that ranking
  **boosts**; non-current versions are **demoted** via beta.130's signed `PredicateSalience`. Historical
  versions stay **retained and queryable** (for diff/audit) but sink out of the way of everyday
  "what does `X` do now" queries. This is the soft-demote idea from an earlier draft, resurrected for
  the right reason: current-vs-historical ranking, not staleness-before-deletion.

### 4. Retention is bounded by ingestion depth, never by eviction

Reference-blind eviction on the live graph is **rejected** (it orphans edges — see Rejected
alternatives). Growth is bounded at the **write boundary**, per source:

| Source | "Current" moves? | Retention depth |
|---|---|---|
| Working repo (tracking `main`) | Yes — HEAD | **Shallow** — current only; **git is the archive** (also the anti-churn discipline) |
| Dependency `@version` (pinned) | No — self-current | Retain the versions actually indexed / upgraded across |
| Doc / URL / config | Yes — last fetch | Current + optional prior-on-change |

The resource story is honest and operator-facing: **footprint = what we index × how deep**, a declared
depth budget — *not* "we drop old data to fit," because we structurally cannot drop safely.

### 5. Deletion is a rare, graph-aware, framework-gated exception (off the critical path)

Deletion is **only** for **mis-registration** (wrong path/repo/typo) and **pure churn**
(never-authoritative) — never for old versions. Any real deletion must be **referentially complete**:
enumerate the scope (`graph.query.prefix` lists a subgraph by ID prefix — enumeration is free), then
clean the query indexes (**#433**) *and* fix dangling referrer triples (**referential cascade**).
Neither is complete in the framework today, so this path stays gated (see Guardrail). Because we
**retain by default**, this gate no longer blocks the flagship — it fences off only the mistake-cleanup
path.

## Task #36, folded in

Refresh-on-bump is §1 verbatim: index the new version as a new scope, add supersession edges, move the
current marker. Additive; no deletion; no #433. The dependency-version lifecycle is *served by the
retention core*, not a separate deletion mechanism.

## Rejected alternatives

- **Fact-contribution ledger + graph projection + winner rules** (withdrawn draft 1) — over-engineered:
  it existed to preserve cross-branch unification on *shared* entity IDs, which versioned sources do not
  have; its winner rule returned `main`'s body for an agent querying its own branch; its core promise
  dead-ended on #433.
- **Deletion-on-bump / scoped retraction** (withdrawn draft 2) — destroys the version-diff payload
  *and* is a referential hazard.
- **NATS-policy retention (TTL / MaxBytes / discard) on the live graph** — reference-**blind**: evicts by
  age or bytes with no knowledge of which edges point where → orphaned edges → integrity loss. Recorded
  explicitly so it is not reached for again.
- **Branch-scoped identity as the primary model** (ADR-0007 Option A) — duplication + no unification;
  used only incidentally, where a source genuinely *is* a distinct scope (a version).

## Consequences

### Enables
- The **dependency-upgrade** query ("what changed between versions") — the money use case — and
  **audit/reproducibility** (retained history *is* the audit trail; ADR-0007 §5).
- An **additive, tier-0 critical path** — version scoping + supersession + current-marking — with no
  deletion and no framework blocker.
- #433 and the whole deletion machinery **leave the critical path**.

### Costs / risks
- Growth is real and bounded **only by ingestion discipline** (depth policy) — which must be set
  honestly per source.
- Correspondence beyond stable-named symbols (renames) needs **tier-1** models; deferred.
- "Current" marking + demote must be wired (task-#38 / beta.130 pattern) or everyday queries get noisy.
- **Related pre-existing concern (not this ADR):** re-indexing an entity on the append-only
  `MergeEntity` path accumulates duplicate `(subject,predicate)` triples; clean current-update-in-place
  for the working repo may need the RPC replace-by-predicate path. Tracked separately.

### Upstream asks
- **semstreams#433** (index cleanup on delete) — asks #15; now **off the critical path** (deletion
  exception only).
- **New candidate — referential-integrity-aware delete** (cascade, or refuse-if-referenced) — the other
  half of "delete is incomplete in the framework". File when the mistake-cleanup path is actually built.

## Open items

1. **Intentional version/ref scoping** — ✅ **DONE** (`entityid.ScopedSystemSlug` + optional `version`
   config field; each version gets a distinct `system` segment). Also fixed a latent raw-passthrough bug
   (code-entity IDs were unsanitized). See the `feat(entityid)` commit.
2. **Version triple + supersession edges + the tier-0 correspondence pass.** Emit a **version triple** on
   each entity (the raw registered version — the ID's `system` is folded/capped and *not* parseable back).
   Correspondence groups entities by their **version-independent triples** (`code.artifact.path`, name,
   `CodeType`, `CodePackage`) and reads the version from that triple; "changed" = `code.artifact.body`
   hash diff across the two scopes. Then emit supersession edges between corresponding entities. **No ID
   string-parsing** — that was an early-draft simplification the #1 implementation invalidated (version is
   not recoverable from the folded/capped `system` slug).
3. **"Current" marker predicate + demote weight** — per-source; tracking vs pinned.
4. **Changeset → symbol modeling** — extend git commit entities with touched-symbol edges from diff
   line-ranges.
5. **Per-source retention depth config** + honest resource guidance.
6. *(Deferred, exception path)* referential cascade-delete design + the upstream ask.

## Guardrail (preserved from ADR-0007)

**No eager deletion until it is referentially complete** — index cleanup (#433) *and* referrer cascade.
Until then, deregister / branch-delete **stops ingestion without deleting** — the safe default. Because
retention is now the default, this guards only the rare mistake-cleanup path, not the flagship.
