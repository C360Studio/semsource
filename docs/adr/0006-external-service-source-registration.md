# ADR-0006: External-Service Source Registration

> **Status:** Proposed | **Date:** 2026-06-29
> **Supersedes premises of:** ADR-0003 (programmatic source-add API)
> **Pairs with:** ADR-0004 (fusion query side), [semstreams#376](https://github.com/C360Studio/semstreams/issues/376)

## Context

ADR-0003 defined a programmatic source-add contract (`graph.ingest.add/remove.
{namespace}`, request/reply, carrying a typed `config.SourceEntry`). It was written
on an explicit premise: *"Headless mode is first-class — SemSource commonly runs
embedded in a semstreams host app, sharing NATS and a config KV bucket."* On that
premise it deferred an HTTP write API, an MCP surface, worktree/ephemeral targets,
and any in-process trust boundary — all justified by "the host owns that."

That premise is being retired. After SemSpec's refactor, **SemSource is an optional
external service**, never embedded headless. The motivating consumer is now an agent
— **Claude Code pointing SemSource at targets over MCP or HTTP** — that wants to say
"index this repo / folder / URL / worktree, then answer `code_context` over it," with
no shared host config to inherit watch intent from.

This flips four of ADR-0003's deferrals into requirements:

1. **There is no ambient config to read.** In the headless model, *what to watch*
   could come from the host's shared KV or a startup `semsource.json`. As an external
   service the **only** source of truth for what to watch is the runtime request.
2. **HTTP and MCP are primary surfaces**, not "build if someone asks." Claude Code is
   the someone.
3. **Ephemeral targets are in scope.** An agent wants transient context on a specific
   worktree/branch: register → ingest → query → tear down.
4. **The trust boundary must be designed**, not assumed to be NATS subject perms. The
   call now arrives over HTTP/MCP from an agent; we want it usable by trusted callers
   today but structured so untrusted/multi-tenant hardening lands without a rewrite.

This ADR re-bases ADR-0003's wire contract for that world. It does **not** rewrite the
contract — `AddRequest`/`AddReply`/`RemoveRequest` and `SourceEntry` stay — it extends
surfaces, target taxonomy, watch semantics, lifecycle, and the trust model.

## Decision

**Make source registration the external-service control plane: one registration
contract, three transports (NATS, HTTP, MCP), with caller-declared watch intent,
explicit target lifecycle, durable bytes, and a trusted-now / untrusted-ready trust
model.**

### 1. One contract, three transports

The ADR-0003 NATS contract stays the source of truth. HTTP and MCP are thin façades
over the **same** `sourcespawn` calls and the same `SourceEntry` schema — no parallel
logic.

| Transport | Surface | Caller |
|---|---|---|
| NATS (exists) | `graph.ingest.add/remove.{namespace}` | in-mesh sem* products |
| HTTP | `POST /sources`, `DELETE /sources/{id}`, `GET /sources/{id}` | external services, scripts |
| MCP | tools `add_source`, `remove_source`, `source_status` (+ `code_context` from ADR-0004) | Claude Code and other agents |

The MCP surface is the convergence point: the agent gets *both* halves of SemSource —
"point me at a target" (this ADR) and "answer over it" (ADR-0004) — as native tools.

### 2. Target taxonomy

The existing `SourceEntry` types stand (`git`, `repo`, `ast`, `docs`, `config`,
`url`, media). Two are promoted to first-class, ergonomic targets because the agent
use case demands them:

- **Folder** — a non-git local path. Sugar over `ast`+`docs`+`config` on that path
  (the way `repo` is sugar over git+ast+docs+config), so "index this directory" is one
  request, not three.
- **Ephemeral git worktree** — a transient checkout of a branch/ref. We already
  materialize worktrees for branch ingestion (`config/expand.go`); this exposes them as
  a registerable target with an explicit, caller-driven lifecycle (below).

### 3. Watch-config contract — answering "how do we know what to watch"

**The caller declares watch intent; SemSource never infers it from ambient state**
(there is none). The watch spec is part of the typed request — `SourceEntry` already
carries type-specific watch fields — with **type-specific defaults applied
server-side** when the caller omits them:

| Target | Default watch | Override |
|---|---|---|
| repo / git | watch the named branch (hook or poll) | branch(es), poll interval, paths |
| folder | recursive fsnotify | globs, depth |
| url | poll (conservative default interval) | interval, etag/conditional |
| ephemeral worktree | **one-shot snapshot, no watch** | opt-in watch for live updates |

This makes "what we watch" deterministic and auditable: it is exactly what the
`AddRequest` said, defaulted per type. It also resolves the watch-config sharp edge
flagged in the ADR-040 curator work — watch behavior is contract, not inference.

### 4. Lifecycle and identity

Registration returns a **stable handle** (`instance_name` today; treat it as the
target id) the caller uses to poll status and to remove. Add the lifecycle states the
ephemeral case needs:

- `add` → handle + `status_subject` + `ready_when` (as ADR-0003).
- `status` → per-target phase (`seeding`/`watching`/`idle`/`error` + `last_error`).
- `remove` → deregister; **for ephemeral worktrees, tear down the checkout and GC its
  disk**; entities are retracted per existing semantics.
- **TTL / GC for ephemeral targets** — a worktree the caller forgets to remove must
  not leak disk. Default TTL with refresh-on-use; documented, caller-overridable.

### 5. Durable bytes (couples to the ObjectStore move)

An ephemeral worktree is torn down on `remove`, but the fused `code_context` answer
hydrates verbatim source. If bodies live only on that worktree's disk, the answer
breaks the moment the target is gone — and a remote/scaled gateway can't read them at
all. **Ingested content must be offloaded to ObjectStore** (the pattern media/docs
handlers already use via `message.StorageReference`), so bytes outlive the target and
the query is location-independent. This is the concrete reason the ObjectStore move
(currently code-on-disk) is a dependency of ephemeral-target support, not a separate
nicety.

### 6. Trust model — trusted now, untrusted-ready

v1 keeps ADR-0003's stance for the NATS path (subject permissions are the boundary)
and treats HTTP/MCP callers as **trusted** (sem* products, Claude Code in a trusted
mesh). But because the call now points a service at arbitrary external resources, we
**design the seams** for untrusted/multi-tenant use so hardening is additive:

- **Auth seam** on HTTP/MCP (token/principal), no-op-pluggable in v1.
- **Resource allowlists** — permitted filesystem roots (path-traversal guard), URL
  host allowlist / SSRF guard, git remote allowlist.
- **Resource limits** — clone size/time caps, max concurrent targets, per-caller
  quotas (the deferred `SEED_TOO_LARGE`/`SEED_TIMEOUT` codes from ADR-0003 land here).
- **Per-caller isolation** — provenance (already in the contract) becomes the tenancy
  key when isolation is turned on.

None of these are enforced in v1; all are present as interfaces/config with permissive
defaults, so "design for untrusted" is real, not aspirational.

## Consequences

### Enables
- Claude Code (and any agent) registers targets and queries them over MCP/HTTP as
  native tools — the original "point us at repos, get fused results back" use case.
- Ephemeral, agent-scoped context: spin up a worktree, get `code_context`, tear down.
- A single control plane regardless of transport; CLI `semsource add` can ride the
  same `sourcespawn` path.

### Costs / risks
- **Headless retirement is a precondition** (its own removal task: drop dual-mode
  `run.go`, foreign-component pruning, the RPC-reply guard). Recorded here as the
  enabling premise; tracked separately for implementation.
- **ObjectStore dependency** for ephemeral targets — sequence the ObjectStore move
  before/with ephemeral-worktree support.
- HTTP/MCP surfaces are new external attack surface even when "trusted" — the trust
  seams above must ship as interfaces from day one, not be retrofitted.

### Out of scope (v1)
- Enforced sandboxing/quotas (seams only).
- Multi-instance target ownership arbitration beyond ADR-0003's namespace queue-group.
- Provenance persistence (still deferred per ADR-0003).

## Alternatives considered

- **Keep NATS-only, let consumers wrap it (ADR-0003's stance).** Rejected for the agent
  use case: Claude Code is out-of-mesh; forcing every consumer to build a NATS façade
  is the exposure mistake ADR-0004 just fixed on the query side.
- **Infer watch targets from a shared/mounted config.** Rejected: that *is* the headless
  model we're retiring; an external service has no ambient config to read.
- **Keep code bytes on disk, skip ObjectStore.** Works only for persistent, co-located
  targets; breaks ephemeral worktrees and any remote/scaled gateway. Rejected as the
  default; disk remains an opt-in "live worktree / read-your-writes" mode.

## Open questions

1. **MCP hosting** — does SemSource embed an MCP server, or ship an MCP façade process
   over the HTTP/NATS contract? (Leaning façade, to keep the core transport-agnostic.)
2. **Worktree TTL defaults** — what default lifetime + refresh policy balances agent
   ergonomics against disk pressure?
3. **Handle stability across restarts** — `instance_name` is deterministic; confirm it
   survives a service restart so a caller's handle stays valid.
4. **Auth shape for HTTP/MCP** — align with whatever identity sem* / Claude Code present.

## References

- ADR-0003 — programmatic source-add API (the contract this extends)
- ADR-0004 — deterministic fusion gateway (the query half of the external-service API)
- ADR-040 (curator) — runtime add over NATS; the watch-config sharp edge resolved here
- `config/source.go` — `SourceEntry` schema; `config/expand.go` — worktree materialization
- `internal/sourcespawn/` — the shared spawn path all three transports call
- semstreams#376 — lifting fusion upstream; shares the location-independent-bytes question
