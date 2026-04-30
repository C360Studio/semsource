# ADR-0003: Programmatic Source-Add API

> **Status:** Proposed | **Date:** 2026-04-29

## Context

SemSource sources are added today only via the CLI (`semsource add`,
interactive wizard or flags), which writes `semsource.json` on disk. The
running process loads sources at startup; there is no live add path for
programmatic callers.

The SemTeams coordinator agent now has a "research" subagent that can
discover sources (repos, doc sites, URLs) worth ingesting. To close the
loop, that agent must be able to register a discovered source with a
SemSource instance without human intervention.

This ADR defines the contract for that programmatic add path.

### Constraints from the existing system

- **Headless mode is first-class.** SemSource commonly runs embedded in a
  semstreams host app, sharing NATS and a `semstreams_config` KV bucket
  managed by `semconfig.Manager`. SemTeams may run the same way, on the
  same NATS.
- **Hot-spawn already works.** Branch-watcher discovery
  (`startBranchWatchers` in `cmd/semsource/run.go:1099`) writes component
  configs into ConfigManager's KV at runtime; ServiceManager reactively
  spawns the components. Adding a source is the same shape of write.
- **The wire format already exists.** `config.SourceEntry`
  (`config/source.go:24-108`) is a fully-typed, fully-validated request
  shape covering all 9 source types; its `Validate()` enforces
  type-specific required fields. No new schema work is needed.
- **Per-type translation already exists.** Five helpers in `run.go`
  (`{ast,git,doc,cfgfile,url}SourceComponentConfig`) turn a `SourceEntry`
  into `(instanceName, configMap, error)` ready for KV write. They are
  private to `cmd/semsource/` today, but only because nothing else has
  needed them.
- **Read-side already exists on `source-manifest`.** That component owns
  `graph.query.sources`, `graph.query.summary`,
  `graph.query.predicates`, `graph.query.status` and the
  `/source-manifest/*` HTTP handlers.

## Decision

**Expose a programmatic source-add contract on NATS, owned by the
`source-manifest` component, backed by a reusable `sourcespawn` package
extracted from `cmd/semsource/run.go`.**

### Wire shape

Two new NATS request/reply subjects, symmetric with the existing
`graph.query.*` namespace:

| Subject | Purpose |
|---|---|
| `graph.ingest.add.{namespace}` | Register a new source on the SemSource instance(s) serving `{namespace}` |
| `graph.ingest.remove.{namespace}` | Deregister a source by `instance_name` |

The `{namespace}` suffix is the SemSource `Namespace` config field
(`config/config.go:72`). Targeting by namespace gives callers a
deterministic, federation-aware addressing model without needing a
service registry. SemSource subscribes only to its own namespace's
subjects.

### Request shape

```json
{
  "source": { /* config.SourceEntry */ },
  "provenance": {
    "actor": "semteams.research-agent",
    "on_behalf_of": "user-id",
    "trace_id": "..."
  }
}
```

`source` is the existing `config.SourceEntry`. `provenance` is an opaque
metadata block: in v1, SemSource only logs the `actor` field at INFO when
processing the request — `on_behalf_of` and `trace_id` are dropped on
the floor and **nothing is persisted**. Durable recording (onto source
metadata) and forward-stamping (onto entity-event provenance) are
deferred to a follow-on PR.

Callers should still send a complete `provenance` block — its shape is
already part of the wire contract and future SemSource versions will
start persisting it without breaking changes. ADR-030's lifted identity
(or any other identity shape SemTeams chooses) drops in unchanged
because SemSource doesn't interpret the field's contents.

### Reply shape

```json
{
  "instance_name": "git-source-acme-foo-main",
  "created": true,
  "status_subject": "graph.ingest.status",
  "ready_when": "source_status.phase in ['watching', 'idle']",
  "error": null
}
```

On synchronous failure: `instance_name` and `created` may be empty;
`error` is populated with a typed code (see "Failure shapes" below).

### Authorization

The NATS connection's principal authorizes the call. SemSource trusts
that any caller able to publish on `graph.ingest.add.{namespace}` is
authorized to add to that namespace. There is no header-level identity,
no allowlist inside SemSource, and no per-call auth token. Subject
permissions are the boundary.

This matches every other write in the SemStream ecosystem (KV writes,
status emissions) and avoids a parallel auth system.

### What "indexed" means in the reply

SemSource reports authoritative readiness up through **graph-queryable**:

```
add returns      → KV write committed; ServiceManager notified
seed_started     → component spawned; ingestion begun
seed_complete    → all entities emitted to GRAPH stream
graph_queryable  → graph.query.* returns these entities  ◄── SemSource scope ends
─── downstream of SemSource ───
indexed (search) → semsearch has consumed
indexed (embed)  → semembed has produced vectors
```

The reply's `ready_when` condition references existing per-source phase
reporting in `graph.ingest.status`
(`processor/source-manifest/payload_status.go`). When `SourceStatus.Phase`
flips to `watching` or `idle`, the source's initial seed is in the graph
store. The aggregate `phase: "ready"` means all sources have settled.

Full-text and embedding readiness are **out of scope** for this reply.
Callers who need those signals must subscribe to the relevant downstream
component's status, owned by semsearch / semembed teams.

### Failure shapes

Synchronous failures (returned in the reply's `error` field):

| Code | Retryable | Caller response |
|---|---|---|
| `VALIDATION_FAILED` | No | Bubble field-level error to user |
| `INSTANCE_EXISTS` | Idempotent | If submitted config matches existing, treat as success; otherwise prompt user before replace |
| `KV_WRITE_FAILED` | Yes | Backoff + retry |
| `UNSUPPORTED_TYPE` | No | Type not implemented in this build (e.g., image/video deferred) |
| `INTERNAL_ERROR` | Yes | Unexpected SemSource-side failure not attributable to caller input. Backoff + retry; if persistent, surface to operator. |

Asynchronous failures (emitted on `graph.ingest.status` via a new
`last_error` field on `SourceStatus`):

| Code | Meaning | Caller response |
|---|---|---|
| `SOURCE_UNREACHABLE` | Git 404, URL DNS fail, path missing | Single retry then surface to user |
| `SOURCE_AUTH_FAILED` | Private repo, 401/403 | Prompt user for credentials; unrecoverable without input |
| `WATCH_FAILED` | fsnotify or polling errors after seed | Already counted in `error_count`; cosmetic surfacing |

Codes deferred to later ADRs: `SEED_TIMEOUT` (needs a degraded-per-source
notion), `SEED_TOO_LARGE` (needs a configurable size cap that does not
exist today). Add when first caller hits them.

### Implementation outline

1. **Extract** the five `*SourceComponentConfig` helpers from
   `cmd/semsource/run.go` into a new `internal/sourcespawn/` package.
   Existing call sites (startup loader, branch watcher) update to
   import them.
2. **Add** `sourcespawn.Add(ctx, src, configMgr, org) → (instanceName,
   error)` and `sourcespawn.Remove(ctx, instanceName, configMgr) →
   error`. Pure dispatch + wrap + KV call.
3. **Extend** `source-manifest` with subscriptions on
   `graph.ingest.add.{namespace}` and `graph.ingest.remove.{namespace}`.
   Validate → call `sourcespawn` → reply.
4. **Extend** `SourceStatus` with `last_error: { code, message,
   timestamp }`. Source components populate it on error transitions.
5. **Offer** an in-process Go API on `source-manifest` for co-located
   host apps (zero-overhead, type-safe). Same underlying
   `sourcespawn` call.

## Consequences

### What this enables

- SemTeams research agents register discovered sources directly,
  closing the coordinator → research → ingestion loop.
- Any future MCP-capable client can wrap the NATS contract as a thin
  façade — no semsource change needed.
- The existing CLI (`semsource add`) can migrate onto the same
  `sourcespawn` package, eliminating duplicated source-spawn logic.

### What is out of scope for v1

- **HTTP write API.** Headless deployment has no SemSource HTTP
  surface bound externally; build only if a non-NATS, non-MCP caller
  actually needs it.
- **MCP server.** Defer until an out-of-mesh consumer requests it. The
  NATS contract being firm makes the wrapper a small follow-on.
- **Image, video, audio types.** They are valid in
  `validSourceTypes` but lack `*SourceComponentConfig` helpers in the
  branch-watcher path; verify and add separately.
- **Multi-branch `repo` type.** Multi-branch expansion runs through
  the `BranchWatcher` goroutine, which is currently startup-only and
  not KV-reactive. v1 accepts single-branch `git` and `repo` adds;
  multi-branch adds return `UNSUPPORTED_TYPE` until the watcher itself
  becomes a KV-spawned component.
- **`SEED_TIMEOUT` / `SEED_TOO_LARGE` failure codes.** Defer until
  needed.
- **Provenance persistence.** v1 logs `actor` only; no field is written
  to component KV configs or stamped onto entity events. Wire contract
  is stable so this can land transparently later.
- ~~**Manifest refresh on Add/Remove.**~~ Implemented: a successful Add
  appends the source to the in-memory manifest, re-marshals
  `responseData`, and republishes on `graph.ingest.manifest`. A
  successful Remove inverts the operation. `graph.query.sources` and
  the HTTP `/source-manifest/sources` handler always reflect the
  current set under a read-write mutex.

### Trust model and risk

The shared `semstreams_config` KV bucket is not segmented per app.
SemTeams could, in principle, write a source-component KV entry
directly and bypass `graph.ingest.add` entirely. Two reasons that's
fine in practice:

1. NATS subject permissions, not bucket ACLs, are the operational
   trust boundary. If SemTeams's creds permit `graph.ingest.add` they
   permit registering sources; we don't gain security by also blocking
   raw KV.
2. Going through `graph.ingest.add` gets validation, ownership
   tagging, and provenance stamping. Direct KV writes get none of
   that. Tooling discipline (not a security control) keeps callers on
   the API.

If a stricter boundary becomes necessary, NATS permissions can deny
direct KV writes from SemTeams's principal while permitting
`graph.ingest.add.{namespace}`. That is a deployment-time decision,
not an API-design decision.

### Idempotency

Component instance names are deterministic (e.g.,
`git-source-{repo-slug}-{branch-slug}`). A duplicate add lands on the
same KV key; the reply's `created: false` distinguishes refresh from
new. Callers can retry safely.

### Open questions for SemTeams

1. Confirm namespace-targeting model (`graph.ingest.add.{namespace}`)
   covers SemTeams's deployment topology. If multiple physical
   SemSource instances share a namespace, which one owns a given
   add? (NATS will deliver to one queue-group member; that's the
   simplest answer, but worth confirming this matches federation
   expectations.)
2. Confirm `provenance` block contents — what shape will ADR-030's
   lifted identity arrive in? SemSource records as-is, but SemTeams
   should pin a schema so downstream consumers can rely on its keys.
3. Confirm acceptable async-status latency. Existing status component
   publishes on heartbeat cadence (default 30s). For research-agent
   UX, this may be too slow; we may need a one-shot status emission
   on phase transition.

## References

- `config/source.go` — `SourceEntry` schema and validation
- `cmd/semsource/run.go:1099-1243` — branch-watcher hot-spawn pattern
  (template for this ADR's implementation)
- `processor/source-manifest/payload_status.go` — existing status
  payload, to be extended with `last_error`
- ADR-030 (SemTeams) — identity-lifting middleware whose payload lands
  in this API's `provenance` block
