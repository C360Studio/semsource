# ADR-0007: SemSource Sidecar — Dynamic Repo Registration & Branch Lifecycle

> **Status:** Accepted — both consumers (SemTeams, SemSpec) signed off 2026-07-02; the remaining design items (below) are follow-on, not acceptance blockers | **Date:** 2026-07-02
> **Pairs with:** ADR-0006 (external-service source registration), ADR-0004 (deterministic fusion gateway), ADR-0003 (programmatic source-add API)
> **Depends on:** the ObjectStore-for-code move (landed — code bodies offload to the CONTENT bucket; the fusion lenses are stateless and hydrate by handle, so answers survive worktree teardown).
> **Consumer input:** SemTeams & SemSpec reviewed & endorsed 2026-07-02 — feedback incorporated below (tagged _SemTeams_ / _SemSpec_).

## Context

SemSource was **removed from semspec** because its headless mode caused **graph churn**: it
inferred what to index from ambient/mounted config and re-indexed continuously, thrashing the
graph. The replacement design runs SemSource as a **sidecar** to semspec (and semteams), exposing
the code/doc/AST graph and fused reasoning (ADR-0004) as a service the consumer *points at*, rather
than a process that infers its own targets.

The driving consumer shape:

- **semspec runs an issue→PR flow.** Agents work in a **sandboxed Docker container** on **feature
  branches**, generate code, submit it for review, and — if approved — merge to `main`.
- semspec must **dynamically point SemSource at whatever repo root it is working on** at runtime.
- **Firm constraint:** external consumers **never touch our NATS** — preferably ever. All external
  interaction is over **HTTP or MCP**. (MCP is not yet started; HTTP is the first target.)

Two facts about the current implementation shape the decision and the open problem:

1. **Remove does not retract.** The NATS/`sourcespawn` remove path (`handleRemoveRequest` →
   `sourcespawn.Remove`) deletes the component config and stops ingestion, but **leaves the
   entities in the graph**. ADR-0006 §4's "entities retracted per existing semantics" is
   aspirational — no retraction-on-remove exists today.
2. **Code identity is path-derived, not branch-aware.** For AST/code entities the `system` ID
   segment is the base name of the worktree root path (`pathToSystemSlug`), with an optional
   `project` override. Git entities are branch-scoped via `BranchScopedSlug` *only when* a
   `BranchSlug` is set. So whether two branches of the same repo produce the **same** code entity
   IDs depends entirely on whether they are read from the **same path** — implicit, inconsistent,
   and fragile.

## Decision (architecture — settled)

### 1. Sidecar topology: HTTP control plane, shared-mount data plane

SemSource runs alongside the semspec sandbox. Responsibilities split by plane:

| Plane | Transport | Why |
|---|---|---|
| **Control** (register / deregister / status) | **HTTP** (later MCP) | Respects the no-external-NATS rule; the consumer declares intent explicitly. |
| **Data** (read the repo bytes) | **Shared filesystem mount** | The sandbox is a Docker container; expose the repo root as a mount SemSource reads **in place**. No byte-shipping, no NATS exposure. |

The **operator declares a shared root**; the consumer (semspec/semteams) owns **per-run workspace
allocation under that root**. SemSource receives either a SemSource-visible path or — preferably — a
**`root-id` + relative path** it resolves against an **allowlisted mount**, mounts **read-only**, and
**never deletes caller-owned worktree disk** for in-place sidecar registrations. semspec already
places setup burden on the operator, so a declared shared mount is acceptable. This split is the
concrete fix for the headless churn eviction: targets are **declared over HTTP**, never inferred from
ambient state. _(SemTeams-confirmed; concrete `root-id` scheme still to pin — see Remaining design items.)_

### 2. HTTP registration façade (ADR-0006 §1)

`POST /sources`, `DELETE /sources/{id}`, `GET /sources/{id}` — a thin façade over the **same**
`sourcespawn` path and `SourceEntry` schema the NATS transport uses (no parallel logic). Auth:
**optional bearer token, permissive default** (ADR-0006 §6) — the auth seam ships now, enforcement
is opt-in via `SEMSOURCE_API_TOKEN`.

### 3. In-place indexing (resolves issue #1)

Accept **path-only** `git`/`repo` config so the mounted repo root is indexed **in place**, never
cloned. Today `config/source.go` rejects `git` without a `url`; the handler's `resolveRepoPath`
already reads a local path in place. This is the semspec#142 gate.

**Filesystem-root allowlisting / root-relative resolution ships in v1 of the sidecar path** — a
path-only `git`/`repo` over HTTP must **not** accept arbitrary host paths, even under permissive
auth. The path (or `root-id` + relative path) resolves against the allowlisted mount root(s) with a
path-traversal guard; anything outside is rejected. _(SemTeams-requested tightening — this is a v1
requirement, not a deferred seam.)_

### 4. Cadence: coherent checkpoints, not churn

Default to indexing **coherent states**, never keystroke churn (the eviction cause):

- **Watch stable branches** (e.g. `main`) and re-index on **merge** — a merge is a commit on the
  watched ref; the git source's hook/poll catches it. (Seam to nail: the git ref-change must
  re-drive AST/doc re-indexing; today AST/doc refresh via fsnotify when files on disk change, which
  holds when SemSource reads the live working tree the merge lands in.)
- **Do not fsnotify-watch churning agent worktrees by default.** If an agent's in-flight worktree is
  registered, it is an **explicit one-shot snapshot at a phase boundary** (e.g. review, or an
  on-demand `code_context`), never a live watch over a churning workspace. _(SemTeams-confirmed.)_
- Rationale: SemSource's value is a **stable graph substrate + reasoning** (dependency repos,
  reference docs, base/PR branches, review snapshots), not tracking an agent's half-written code —
  which is redundant with the agent's own context and better served by the compiler/tests/LSP.

### 5. Registration handle — stable & auditable _(SemSpec-required, v1)_

SemSpec runs last **hours** and must **prove which graph view a plan/execution used** (carried into
OpenSpec artifacts and paid-run evidence). So the handle returned by `POST /sources` and resolvable
via `GET /sources/{id}` must carry stable, auditable metadata, not just an instance name:

- `root-id` + relative path (the allowlisted-mount coordinates, §1)
- `mode` — `snapshot` vs `watch`
- `ref`/`commit` or **content epoch/generation** (what was actually indexed)
- **readiness + indexed timestamps**

A **source-set/group** handle may be a **client-side list in v1** (SemSpec composes handles);
first-class grouping is post-v1.

### 6. Read/query stays external too — pairs with ADR-0004 _(SemSpec-required)_

This ADR is the **write/control** side. The **read** side must be equally external: fused
`code_context`/`doc_context` **and source readiness** over **HTTP/MCP**, never requiring
`graph.query.*` NATS from an external consumer. **ADR-0004** owns the fused query path (already HTTP
via the code-context/doc-context gateways). Concretely: **readiness must be HTTP-pollable** via
`GET /sources/{id}` — the existing `AddReply.StatusSubject` is a NATS subject usable **in-mesh only**;
external consumers poll the HTTP status endpoint. Treat ADR-0004 as a required companion so we never
accidentally couple SemSpec to NATS on the read path.

## The hard problem (resolved: B + C) — branch lifecycle & deletion

**Lifecycle (proposed, simple):** keep a branch's graph data as long as the branch exists. The hard
part is **deletion** — specifically a feature branch deleted **after it merges to `main`**.

The hazard, concretely:

1. Agent works on `feature/x`; SemSource indexes it. New symbols get entity IDs.
2. `feature/x` merges to `main`; SemSource re-indexes `main` → the **same symbols** now attributed
   to `main`.
3. `feature/x` is deleted. If we naively "retract the feature branch's entities," and code IDs are
   **branch-independent**, we retract entity IDs that **`main` still needs** — deleting live data.
   *Deleting without knowing exactly what we are doing is a shit show.*

The root cause is unresolved **identity scoping across branches** (see Context fact #2). The design
axis:

- **Option A — Branch-scoped identity.** Fold the branch/ref into the entity ID (or read each branch
  from a distinct path). *Pro:* per-branch retraction is trivially safe (distinct IDs); deleting a
  branch can't touch another's data. *Con:* **full duplication** — the same unchanged symbol exists
  N times across N branches, N body blobs; no cross-branch unification ("is this the same function
  as on main?"); graph size grows with branch count; merge-to-main creates a fresh copy and orphans
  the feature copy. Contradicts the spec's intrinsic, branch-free ID.
- **Option B — Branch-independent identity + provenance/refcount.** One entity per intrinsic ID;
  track which sources/branches currently produce it. Branch deletion **removes that source's
  contribution**; retract **only when no active source still produces it**. *Pro:* correct dedup +
  safe deletion; a symbol on both `feature/x` and `main` survives the feature's deletion because
  `main` still references it; a symbol unique to the deleted branch is retracted. Aligns with the
  intrinsic ID spec and the existing federation merge direction (`public.*` merges, `{org}.*`
  sovereign). *Con:* requires provenance/refcounting and defined merge semantics.
  **_SemTeams sharpening (load-bearing):_** provenance must be at the **fact/contribution layer —
  keyed by (source × ref/commit) — not the entity row alone.** The same intrinsic symbol ID carries
  **divergent per-branch facts**: different body blob (`code:<sha>` — feature and main hash to
  different keys), line ranges, and relationships (calls/imports). So you cannot merely refcount
  "does the entity exist"; you must track *which source contributes which facts*, so branch deletion
  removes exactly that branch's facts (its body-key, its line range, its edges) while `main`'s facts
  for the same ID survive — and the entity vanishes only when no source contributes any fact.
  Triples already carry `Source`/`Timestamp`/`Confidence`; extend that provenance with the concrete
  source-contribution identity (source instance + ref/commit).
- **Option C — No eager delete; staleness GC.** Never hard-retract on branch deletion. Stamp
  `last_seen`; a GC pass retracts entities unreferenced by any active source after a TTL. *Pro:*
  the catastrophe is impossible — shared entities are never eagerly deleted; convergence is
  eventual. *Con:* stale data lingers; needs a GC policy and TTL tuning.

**Prerequisite regardless of A/B/C:** **retraction-on-remove must actually be built** — it does not
exist today (Context fact #1). Branch deletion / source removal currently leaks entities.

### Decision — branch lifecycle (B + C)

Adopt **B (fact-layer provenance/refcount) with C (staleness GC) as the safety net** — endorsed by
**both SemTeams and SemSpec**. Provenance is keyed at the **(source × ref/commit × generation)**
contribution level (SemSpec: contribution identity is **not** path-only) so divergent per-branch
facts reconcile correctly; branch deletion removes that source's fact contributions and only retracts
a fact/entity once no active source still produces it, with a GC backstop for anything missed — so
deletion is never a hard, irreversible retraction of shared data. **Explicitly reject** the naive
"delete branch → retract its entities." This matches SemSource's intrinsic-ID spec and federation
model, and makes the merge-then-delete flow safe by construction.

**Sequencing guardrail (SemSpec, hard):** **no eager retraction ships until fact-layer provenance +
staleness GC exist.** Until then, source removal / branch deletion **stops ingestion without eager
retraction** — today's non-retraction (Context fact #1) is therefore the *safe default*, not a bug to
rush-fix; adding naive retraction-on-remove first would reintroduce the exact data-loss hazard this
ADR exists to prevent.

## No-regret implementation (unblocked — ADR accepted)

These are prerequisites for **every** version of the sidecar design and do not depend on resolving
the branch-identity question:

1. **HTTP façade** — `POST/DELETE/GET /sources`, optional bearer token, over the existing
   `sourcespawn` path.
2. **Issue #1** — accept path-only `git`/`repo`, enabling in-place indexing of the mounted repo root.

## Remaining design items (follow-on)

_Both SemTeams and SemSpec signed off 2026-07-02. These are concrete design items, **not** acceptance
blockers — the decisions above are settled; these are how-to detail._

1. **`root-id` / allowlist scheme** — how operator roots are declared and allowlisted, and how a
   `root-id` + relative path resolves against them with a traversal guard. _(Both endorsed the model;
   scheme TBD.)_
2. **Fact-provenance data model** — the concrete (source × ref/commit × generation) contribution
   store and divergent-fact reconciliation: whether it rides the existing triple `Source` provenance
   or a new contribution index; the staleness-GC policy/TTL. **Gates any retraction work** (sequencing
   guardrail above). **→ Superseded by [ADR-0008](0008-versioned-source-retention-supersession.md)**
   (Proposed), which **replaces this whole A/B/C debate** with *retain + relate, don't delete*: the
   graph is temporal — versioned sources are retained as distinct subgraphs and related by
   **supersession edges**, "current" is a per-source ranking marker, and **deletion** is demoted to a
   rare, graph-aware, framework-gated exception (mistakes/churn only), off the critical path. The
   fact-layer ledger and every deletion-first model were rejected — a graph cannot safely delete by
   policy (reference-blind eviction orphans edges), and for dependency upgrades the version
   *relationship* is the payload, not something to throw away.
3. **Registration-handle payload** — finalize the `GET /sources/{id}` / `AddReply` fields from §5
   (root-id, relative path, mode, ref/commit-or-generation, readiness/indexed timestamps) and the
   HTTP readiness endpoint (§6).
4. **Source-set/group handle** — client-side list in v1; first-class grouping is post-v1 (both agree).

## Consequences

### Enables
- semspec (and any agent) dynamically points SemSource at a repo root over HTTP and queries fused
  `code_context`/`doc_context` over it — the sidecar use case, without NATS exposure.
- A clean control/data split (HTTP + mount) that structurally prevents the headless churn that caused
  the original eviction.

### Costs / risks
- **Retraction semantics are a hard prerequisite** for safe deletion and must be built before any
  branch-deletion handling ships.
- The branch → identity decision is load-bearing and touches the entity-ID foundation; getting it
  wrong risks either graph bloat (A) or data-loss on deletion (naive B).
- HTTP is new external attack surface even when "trusted" — the auth seam must ship from day one.

### Out of scope (v1)
- SemSource-materialized ephemeral worktrees with TTL/GC (ADR-0006 §2) — semspec brings its own
  worktree, so SemSource indexes in place and **never deletes the caller's worktree disk**.
- MCP surface (tracked as the follow-on after HTTP).
- Enforced quotas (seams only, per ADR-0006 §6) — **except filesystem-root allowlisting, promoted to
  a v1 requirement** for the sidecar path (§3, SemTeams).
