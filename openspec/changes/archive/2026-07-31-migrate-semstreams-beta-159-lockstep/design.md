## Context

SemStreams `v1.0.0-beta.159` is a sister-lockstep breaking wave: 92 commits, seven breaking merges,
and a tag annotation that names SemSource among the repos expected to conform at the tag. Framework
adoption is tracked in semstreams gh#753, whose owner ruling (2026-07-30) is explicit — SemStreams'
obligation is to note the break and publish guidance; conforming is the sister repo's job, and
problems found while adopting become new issues in the semstreams queue rather than blocking a
framework spec.

The starting position matters for how this change is planned. A compile probe of SemSource at
beta.159 in a throwaway worktree is clean at every build tag: `go build ./...`, `go vet ./...`,
`go vet -tags=integration ./...`, and `go vet -tags=e2e ./...` all pass with no edits. Seven breaking
merges produced zero Go API breakage here. The entire migration is configuration, runtime behavior,
and NATS state.

## Goals / Non-Goals

**Goals**

- Pin beta.159 and make SemSource's composition boot against it.
- Bring every declared KV port subject into conformance with the framework bucket catalog.
- Align the Compose NATS server with the version SemStreams tests against.
- Perform one destructive local graph-state cutover and prove query and replay parity afterwards.
- Produce the evidence envelope the sister-repo cutover checklist requires.

**Non-Goals**

- Adopting caught-up readiness folding (additive, ADR-088) — separable follow-up.
- Any GraphQL read-path edit — see D5.
- Compatibility shims, aliases, dual reads, or in-place state converters.
- Wiping or migrating state beyond the local Compose deployment.
- Editing SemStreams.

## Decisions

### D1. Plan this as a config/state migration, and refuse a green build as conformance evidence

The compile probe passing at all four build-tag combinations is the single most load-bearing fact in
this change, and also the most dangerous one. Every breaking contract in this wave fails at
*component validation*, *component start*, or *not at all until behavior diverges* — never at compile
time. The gateway note makes the failure mode explicit for its own change: envelope and unwrapped
payload both decode cleanly into a permissive target, so a wrong client reads zero values silently
rather than erroring.

Consequence: acceptance for this change is runtime acceptance. `task check` passing proves nothing
about conformance and is not admissible as migration evidence on its own.

### D2. Derive the deletion set from the catalog; keep the literal list as a tripwire

beta.159 centralizes every framework-guaranteed bucket in one descriptor catalog (22 entries). The
cutover's deletion set is therefore derived from that catalog at the pinned target under each
bucket's resolved name — not from the beta.148-era literal list, which is now wrong in both
directions (`EMBEDDINGS_CACHE` is gone; `COMMUNITY_SUMMARIES`, `GRAPH_STATUS`, `STORAGE_REPORT`,
`ENTITY_SUFFIX_INDEX`, and `GRAPH_INGEST_APPLIED_SEQ` are new).

The literal list in the rehearsal test is kept, but its job changes: it is a parity tripwire that
must *fail* when the catalog moves, exactly as its existing comment states. Updating the literal is a
deliberate reviewed act; deriving the deletion set is automatic. Conflating the two is how a
deletion boundary widens silently against a shared account.

### D3. Take the whole wave at one tag, in one wipe window

The cutover checklist is explicit that the index work does not ship on a later routine bump — it
consumes the same pre-v1 wipe window as the identity changes, and a missed window requires a separate
migration proposal that "must not create a second undeclared wipe." So the dependency pin, the
composition fixes, and the NATS server pin all land together and are proven by one stop-wipe-reseed
cycle. Splitting them buys nothing and costs a second destructive window.

### D4. Ride the NATS server pin on this window

SemSource pins `nats:2-alpine`, a floating major tag; SemStreams runs `nats:2.12-alpine` in its e2e,
tiered, and testcontainers harnesses. Two independent reasons to fix it here: the governed graph
should not run on a server line the framework has not exercised, and `compose-deployment` already
requires exact-version pins, which a floating major tag does not satisfy. Because the JetStream data
volume is being destroyed anyway, the server change costs nothing extra in this window and would cost
a full second one later.

### D5. Explicitly change nothing on the GraphQL read path

The wave's only GraphQL break is `graphSummary` losing its `QueryResponse` envelope. SemSource does
not read it: there is no `graphSummary` or `data.data` occurrence in Go, `ui/`, or any GraphQL
document, and SemSource's only consumer of `graph.query.summary` is source-manifest over NATS-direct
request/reply, which the adopter note states is unaffected because the gateway is not in that path.

This is recorded as a decision rather than an omission because the adopter note warns that the
instinct during this migration is to add or strip a `.data` hop on fields that never carried the
envelope — doing so introduces a bug rather than adopting a change. The correct action here is
provably no action.

### D6. Verify the three silent behavioral contracts rather than assume them

Three changes in the wave alter behavior without touching anything SemSource declares, so each needs
a runtime check rather than a code edit:

- **Add-lane six-field tuple deduplication.** SemSource is the primary add-lane producer. The
  observable risk is entity/triple counts, which SemSource asserts on its status surfaces.
- **`index_not_ready` naming an owner.** Readers no longer create buckets on first use, so a
  deployment ordering that previously self-healed by conjuring an empty bucket now surfaces a
  classified error. Branch on the class, not the code string.
- **Clustering ownership split.** `COMMUNITY_INDEX` stays a valid catalog bucket and remains
  SemSource's declared clustering output, but it becomes trigger-only, with summaries in the
  content-addressed `COMMUNITY_SUMMARIES`. Note the verification dependency: **no shipped config
  enables clustering** — `tier2-semantic-instruct.json` sets `enable_clustering: false`, and the
  only config that turns it on is the Compose tier-2 dev config, which must be tracked before this
  contract can be exercised at all.

### D7. Declare the ingest transport's discard policy as `new`, not `old`

Found during adoption rather than during the audit: beta.159 requires every ordinary stream to
declare `max_age`, `max_bytes`, **and** `discard`. The pre-migration audit saw `GRAPH` already
declaring the two bounds and concluded no change was needed; the third field was missing, and the
unit suite caught it (`internal/sourcespawn`, config validation refusing the component write).

The policy is a real choice, and beta.159's own note is that it was hardcoded to `old` before, so
nobody had ever chosen it. `GRAPH` carries `graph.ingest.*` — the transport, not the graph, which
lives in the `ENTITY_STATES` KV, so ADR-0008's retention-first rule does not apply here and bounding
is correct.

`old` evicts the oldest messages at the ceiling and the publish still succeeds: entities would be
published, silently dropped before graph-ingest consumed them, and every producer-side surface would
report healthy. That is the exact failure class `entity-publish-integrity` exists to prevent.

`new` refuses the publish instead. `internal/entitypub` treats a non-circuit-breaker publish error as
terminal and returns it, so the pressure becomes a counted, surfaced failure rather than a silent
gap. `MaxAge` still expires messages, so a producer stalled at the ceiling recovers without operator
action. The knob is exposed as a `streams.GRAPH.discard` override so a deployment that would rather
shed load than stall can say so explicitly.

### D9. Readiness reads move to GRAPH_STATUS — and this was not additive

The wave's readiness note is headed "additive, NOT breaking," and this change's first draft took it
at its word and listed GRAPH_STATUS adoption as a non-goal. That was wrong, and the way it was wrong
is the point of D1: the codebase compiled, every unit test passed, and the defect only appeared when
a real stack answered a real status request.

The note is accurate about what it describes — *folding* readiness into gating decisions is additive.
But ADR-083 separately **deleted** `graph.index.query.status`, and SemSource read it in two places.
A deleted request/reply subject does not fail loudly at a client: it returns no-responder, which the
composer correctly reports as `unknown`. So the surface degraded to a permanent honest-looking
"we don't know" while the substrate published `ready: true` one KV key away.

The blast radius was larger than one field. `/source-manifest/status` and the MCP `source_status`
tool both under-reported, and `scripts/core-profile-smoke.sh` blocks on `index.ready` — so the very
gate this change lists in task 6.1 could never have gone green. A migration that shipped with the
original non-goal would have been "complete" with a broken shipped surface.

The fix is a small shared reader (`internal/graphstatus`) rather than per-caller KV code, because
both call sites need the same three things: binding must-exist through the catalog seam (readers
never create), retrying the bind rather than caching a startup failure, and treating an absent key
as *unknown* rather than as *not ready*. The producer key list stays SemSource's own, per ADR-088 —
there is no framework-declared "all producers" set, and declaring a key you do not depend on makes
you defer on someone else's outage.

### D10. Fail-closed boot converts latent source misconfiguration into a boot failure

beta.159's component-start barrier (#719) is listed in the wave as "component-start barrier,
fail-closed boot," and reads like an internal robustness change. Its actual adopter impact is that
**any component that never finishes `Start` now takes the whole service down**, where beta.158 would
bring the manager up around it.

SemSource had exactly such a component, in the default install. `doc-source` and `cfgfile-source`
legitimately name files (`README.md`, `go.mod`), but both gated their initial ingest on
`workspace.IsRepoReady`, which is documented as a *directory* check and returns
`path is not a directory` for a file. Callers retry persistently (30 attempts), so those two sources
never ingested — and the only evidence was a debug-level line.

Under beta.158 this was invisible: the service came up, `/source-manifest/sources` answered, and the
quickstart e2e passed in 7.75s with the same six errors in its logs. Under beta.159 `StartAll` never
returns, so nothing serves and the e2e fails at its first HTTP call. The bug is SemSource's and
predates the migration; the migration is what made it fatal instead of silent.

The fix is a new `workspace.IsPathReady` that delegates directories to `IsRepoReady` and accepts an
existing regular file, rather than relaxing `IsRepoReady` itself — ast-source and git-source depend
on its directory contract, and for them a non-directory genuinely is the error.

The general lesson is D1's again, sharpened: this is the third defect in this migration that no
compiler and no unit test could have found, and the only reason it was found at all is that the plan
insisted on runtime acceptance.

### D8. Scope the destructive cutover to the local Compose deployment

The wipe covers the local Compose JetStream state (the `nats-data` volume) only. The checklist's
prohibition on wildcard deletion and copied default lists against shared accounts is the reason to
state the boundary in the change rather than leave it to the operator's memory. Any shared or staging
account is a separate, operator-run execution of the same documented procedure.

## Risks / Trade-offs

- **Silent zero-value reads.** The wave's decode-compatible breaks fail quietly. Mitigated by D1's
  runtime acceptance and by the grep evidence behind D5.
- **Destructive history reconcile.** `ENTITY_STATES` History reconciles 3 → 1 on first boot,
  discarding revisions beyond depth 1. No shipped consumer reads that depth; the risk is out-of-tree
  tooling, which must capture what it needs first. Accepted — the graph is rebuilt from canonical
  source inputs regardless.
- **Count drift from add-lane dedup.** If deduplication changes distinct-entity cardinality,
  `ingestion-readiness`'s count invariants and known-answer queries may move. Treated as a finding to
  investigate, not a number to quietly re-baseline.
- **NATS 2.12 on an existing volume.** Not a risk in this change because the volume is destroyed as
  part of the cutover; it would be one for any deployment that pins the server without the wipe.
- **CLI-only OpenSpec tooling.** The `openspec` CLI is a local authoring dependency installed on the
  developer machine; it is not in CI and this change does not add it there.

## Rollout Plan

1. Pin beta.159, tidy, and confirm the build stays clean — establishing the baseline, not conformance.
2. Fix the composition: remove the graph-embedding `ports.outputs` block; confirm all remaining KV
   port subjects resolve against the catalog.
3. Update the rehearsal inventory to the beta.159 catalog and let the parity assertion prove it.
4. Pin `nats:2.12-alpine` in Compose.
5. Stop writers, capture the account inventory, execute the catalog-derived deletion, and reseed.
6. Prove readiness, a canonical known-answer query, and replay parity after one restart with no
   intervening write.
7. Run `task check`, `task test:race`, `task test:e2e`, and `task core:smoke`; record the evidence
   envelope; report adoption on semstreams gh#753.

Rollback is redeploy-at-beta.158 plus another reseed from canonical sources. There is no state
rollback, by design.
