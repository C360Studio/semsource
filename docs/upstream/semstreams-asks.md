# Upstream Asks: semstreams

Framework-capability gaps we hit while building semsource. The dual of the
"don't rebuild what the framework provides" rule: **don't silently absorb
framework-shaped work as bespoke either.**

**Triage each entry:**
- **framework-shaped** → file an issue against semstreams; stopgap locally if blocking.
- **product-shaped** → decide *first* whether it's our job; if so, extend locally
  (e.g. a `semsource/`-prefixed extension, per semstreams ADR-042's extension scheme).

Status: `candidate` (not yet filed) · `filed #NNN` · `local-stopgap` · `wontfix`

---

## Major proposals

### 0. Lift deterministic graph fusion into the framework — framework-shaped — filed [semstreams#376](https://github.com/C360Studio/semstreams/issues/376)
The deterministic fusion gateway proven in semsource (ADR-0004) is cross-domain
(code + docs lenses over one engine) and is the deterministic sibling of
`research_graph` (ADR-045) — so it's framework-shaped, not product-shaped. Filed as
a research issue to scope an upstream ADR. The gating design decision is **verbatim
body hydration / content addressing** (where bytes come from in a headless/remote
deployment) — this is exactly what keeps semsource's gateway standalone-only today.
**Sub-dependencies of this proposal:** asks #1 (subclass helper), #2 (predicate
salience), #5 (name→ranked-IDs index) below.

---

## BFO/CCO alignment (ADR-0005)

### 1. BFO/CCO `SubClassOf` / hierarchy helper — framework-shaped — candidate
`vocabulary/bfo` and `vocabulary/cco` ship IRI constants only, no subclass graph.
Ontology-distance ranking needs the (fixed, standard) BFO/CCO subclass tree. A
`SubClassOf` map or `Parents(iri)`/`IsA(child, parent)` helper in those packages
would serve every consumer instead of each re-encoding it.
**Stopgap:** static subclass subtree in `source/ontology/` (only the classes we use).

### 2. Predicate role / salience in the vocabulary registry — framework-shaped — candidate
Predicate roles (identity/relationship/metric/…) are pattern-matched in semsource
`processor/source-manifest/status.go`, not stored. A `WithRole`/`WithWeight` option
on `vocabulary.Register` (carried in `PredicateMetadata`) would make salience a
first-class, framework-level ranking signal rather than a per-consumer heuristic.
**Stopgap:** salience table in `source/ontology/` / the fusion ranker.

### 3. CCO software relations (`calls`, `imports`) — product-shaped — decide ownership
CCO's software surface is shallow (`SoftwareCode`/`Algorithm`/`SoftwareAgent`; no
call/import relations). These are software-engineering-domain relations.
**Decision needed:** likely *ours* to extend (`semsource/`-prefixed extension IRIs)
since they're our domain — but if a general software ontology is wanted framework-side,
propose upstream. Do not force a bad mapping onto an existing CCO relation.

### 4. `vocabulary/export` renders absolute-IRI objects as literals — framework-shaped — candidate
`export/object.go` `classifyString` treats a string object as an IRI resource only
when `message.IsValidEntityID(s)` (a 6-part dotted ID). A full IRI object like
`http://…/CommonCoreOntologies/Algorithm` is classified as a string literal, so the
deferred RDF export of our `entity.ontology.class` (rdf:type) triple would emit
`<e> rdf:type "http://…/Algorithm"` — **invalid RDF** (rdf:type's object must be an
IRI node). `export` should recognize absolute-IRI string objects (`http(s)://`,
`urn:`) as resources. Until fixed, the "export is free later" claim in ADR-0005 does
not hold for class/relation IRIs.
**Surfaced by:** ADR-0005 A0 review. Not exercised today (export deferred).

### 5. No name→ranked-IDs index for symbol resolution — framework-shaped — candidate
`graph.query.*` has no subject that maps a bare symbol NAME (e.g. "OnEvent") to a
ranked list of entity IDs. `entityByAlias` returns a single canonical ID (only if
the name is a registered alias); `semantic` is embedding-based. So the fusion code
lens's deterministic symbol lookup (natsgraph `resolveSymbol`) falls back to
semantic search for un-aliased symbols — works, but the result is embedding-ranked,
not an exact deterministic name match, and the provenance label is then optimistic.
A name/title index subject (`graph.query.byName` or similar) would make code symbol
lookup truly deterministic.
**Surfaced by:** Fusion D review (ADR-0004).

---

## Fusion (pkg/fusion, ADR-062)

### 9. Paths / Impact facets deferred from the framework engine — framework-shaped — RESOLVED in beta.123 ([semstreams#409](https://github.com/C360Studio/semstreams/issues/409), PR #413)
**Status:** Shipped. The engine now computes `Response.Paths`/`Response.Impact` when the request
Wants them (`WantPaths`/`WantImpact`). semsource adopted beta.123 and retired its local
`source/fusion/impact` extension + `contextResponse` wrapper — the code-context "impact" verb reads
`Response.Impact` directly. (Original ask below.)

`pkg/fusion` (beta.122) lifts the engine, Lens SPI, and hydration contract, but
`WantPaths`/`WantImpact` are reserved constants the engine ignores — the
transitive relation-path and reverse-closure facets are a deferred follow-on.
semsource's `code_context` "impact" verb needs the reverse closure, so on the
convergence we kept a thin local extension (`source/fusion/impact`) that walks
`fusion.RetrievalClient.Neighbors` and attaches an impact summary to the response
(`contextResponse`). It's the deterministic sibling of the relations facet the
engine already owns; lifting Paths/Impact into the engine (a `Want`→facet the
engine computes, carried on `Response`) would retire this extension the way the
Lens SPI retired our local engine.
**Surfaced by:** ADR-062 increment-6 convergence (source/fusion → pkg/fusion).

### 10. `graph.query.byName` readiness depends on products registering label predicates — framework-shaped — RESOLVED in beta.123 ([semstreams#410](https://github.com/C360Studio/semstreams/issues/410), PR #412)
**Status:** Shipped. `vocabulary.Register` now amends rather than replaces, so a role-less re-Register
retains a previously-declared `AliasTypeLabel`. semsource's `WithAlias(AliasTypeLabel, 1)` on
`dc.terms.title` is now belt-and-suspenders (still correct, no longer load-bearing). (Original ask below.)

graph-index's `graph.index.query.status` readiness (and `graph.query.byName`
itself) is driven by the NAME_INDEX, populated only for predicates
`vocabulary.DiscoverLabelPredicates()` returns — i.e. those registered
`WithAlias(AliasTypeLabel, …)`. Because `vocabulary.Register` OVERWRITES rather
than merges, a product that re-registers `dc.terms.title` (for a description/IRI)
without re-declaring the label alias silently drops it from the label set —
breaking both byName symbol resolution and fusion readiness, with no error.
semsource hit exactly this (`source/ast/vocabulary.go` re-registered DcTitle
without the alias; fixed by re-adding `WithAlias(AliasTypeLabel, 1)`). The
framework could make label roles sticky across re-registration, or expose a
merge-mode Register, or at minimum document that label aliases must be
re-declared on any re-Register of a name predicate.
**Surfaced by:** ADR-062 convergence — fusionnats readiness stuck `building` in the
live-graph integration test until DcTitle's label alias was restored.

### 11. graph-embedding fetches offloaded content by one fixed bucket, not `StorageReference.StorageInstance` — framework-shaped — filed [semstreams#414](https://github.com/C360Studio/semstreams/issues/414)
graph-embedding's ContentStorable fetch builds ONE `objectstore.Store` from a single `store-read`
input-port bucket, with no `StorageInstance` resolution — unlike the fusion hydration helper (#399),
which resolves `StorageReference.StorageInstance` → a registered `storage.Store`. So embedding can't
fetch content whose instance differs from its one configured bucket, and when it can't fetch it
silently degrades to inline text extraction — which is ABSENT for offloaded entities (offloading omits
the inline triple). Net: offloaded body text is silently dropped from embeddings/BM25/search. semsource
hit this — its graph-embedding wires no `store-read` port, so doc/media/code bodies offloaded by the
producers aren't embedded. Ask: reuse the #399 `StoreResolver` in graph-embedding; at minimum make the
"StorageRef set but unfetchable → inline absent → body dropped" case loud, not a silent Debug.
**Blocks:** the doc-store unification (semsource task #19) — a true unify needs instance-aware embedding.
**Surfaced by:** ADR-062 convergence — investigating why doc bodies were double-stored (CONTENT vs MESSAGES).

## Transport / subject taxonomy

### 6. RPC reply subjects share the `graph.ingest.*` prefix with the persisted data plane — framework-shaped — candidate
graph-ingest serves core request/reply on `graph.ingest.query.{entity,batch,prefix,
suffix}` and the curator workflow on `graph.ingest.add/remove.*`, while the persisted
entity stream binds `graph.ingest.entity`. Because the RPC plane and the persisted
plane sit under the same `graph.ingest.*` prefix, the natural-looking stream binding
`graph.ingest.>` silently breaks request/reply: JetStream PubAcks land on the
request's reply inbox and win the race against the real handler reply, so reads
return zero results and curator spawns no component — with no error. Separating the
RPC plane from the persisted plane (e.g. `graph.rpc.ingest.*` vs `graph.ingest.*`, or
documenting a reserved sub-tree streams must never bind) would make a wildcard stream
binding safe by construction. Low urgency for semsource specifically: with headless mode
removed (ADR-0006) semsource owns its own `GRAPH` stream and binds only the five data-plane
subjects, so it is no longer exposed to a host-owned wildcard stream — this is now a purely
framework-shaped concern for other consumers. (The semsource-side boot guard that probed for
this was removed alongside headless mode; it only made sense when a host owned the stream.)
**Surfaced by:** Fusion full-pipeline integration test (ADR-0004) — a `graph.ingest.>`
test stream zeroed the fused response.

---

## Service auth

### 7. Service-level auth (API token / session) as a framework primitive — framework-shaped — candidate
As semsource becomes an external service (ADR-0006) it needs auth on its HTTP/MCP
surfaces — API token first, session-based for interactive callers later. Every sem*
service exposed externally faces the same need. Rolling auth per service means N
incompatible token schemes, no shared principal/tenancy model, and per-service drift.
A framework primitive — pluggable auth middleware + a principal/identity type carried
through `component.Dependencies`, reusable across HTTP/NATS/MCP surfaces — would give
the mesh one auth story. semsource will ship a local pluggable seam (permissive
default) per ADR-0006; if the shape generalizes, propose lifting it upstream so it
isn't re-rolled.
**Surfaced by:** ADR-0006 trust model (trusted-now / untrusted-ready).

---

## Runtime config / ComponentManager

### 8. Runtime config writes skipped by engine-owned-revision watcher — framework-shaped — RESOLVED in beta.145 ([semstreams#388](https://github.com/C360Studio/semstreams/issues/388)) — ADOPTED
`ConfigManager.handleUpdate` skipped events whose revision `<= engineHighWaterRev`
and returned **before notifying subscribers**, contradicting its own doc comment
("the skip only affects the in-memory cache update, not subscriber notification").
`PutComponentToKV`/`DeleteComponentFromKV` bumped the high-water without notifying,
so runtime add/remove writes landed in KV but did not reconcile the running
`ComponentManager`.

**Resolution evidence:** SemSource adopted `github.com/c360studio/semstreams
v1.0.0-beta.145`, switched runtime add back to targeted `PutComponentToKV`, and
keeps runtime remove on targeted `DeleteComponentFromKV`. Both paths now rely on
beta.145's engine-owned notification contract instead of SemSource's old full
`PushToKV` workaround.

**Surfaced by:** wiring e2e into CI (curator runtime add/remove, ADR-040 / ADR-0006).

### 8a. Runtime config PushToKV restarts unchanged components — framework-shaped — filed [semstreams#520](https://github.com/C360Studio/semstreams/issues/520)
beta.145 fixed the #388 notification drop, but SemSource stress testing found a
follow-on lifecycle hazard: full `PushToKV` rewrites every component key, and
`ComponentManager.handleComponentConfigUpdate` restarts any existing enabled
component that receives an individual `components.<name>` notification, even if
the effective config is unchanged. In a verbose `TestE2E_RuntimeSourceAdd` run,
one source add restarted unchanged components such as `supersession`,
`websocket-output`, `code-context`, `doc-source-docs`, `graph-ingest`, and
`graph-query`; a separate run saw `/source-manifest/status` return `503
component not started` for a minute after a successful add.

This is especially risky for components with one-shot HTTP or externally-wired
subscription lifecycle: ServiceManager registers component HTTP handlers once
during `completeHTTPSetup`, so a component restart can leave the mux serving a
stopped old instance. SemSource now avoids the trigger by using targeted
`PutComponentToKV` for runtime add, but the framework should still skip restarts
when an individual config update is semantically identical to the running
component config.

**Surfaced by:** SemSource beta.145 pin validation (`TestE2E_RuntimeSourceAdd
-count=5`, July 9, 2026).

### 8b. Heartbeat Stop is not idempotent after context cancellation — framework-shaped — filed [semstreams#520](https://github.com/C360Studio/semstreams/issues/520)
`HeartbeatService.Stop` still returns `heartbeat service not running (status:
stopped)` when the lifecycle context has already stopped the heartbeat before
`Manager.StopAll` reaches it. `StopAll` aggregates that as a hard shutdown error,
which makes SemSource print `semsource: stop services: stop errors: [failed to
stop service heartbeat: heartbeat service not running (status: stopped)]` on an
otherwise orderly interrupt path.

`BaseService.Stop` already treats stopped/stopping as idempotent. Heartbeat
should follow that contract, or `StopAll` should treat already-stopped service
states as clean during coordinated shutdown.

**Surfaced by:** SemSource beta.145 e2e shutdown logs during
`TestE2E_RuntimeSourceAdd`.

### 8c. WebSocket output metric registration is not restart-safe — framework-shaped — RESOLVED in beta.144 ([semstreams#490](https://github.com/C360Studio/semstreams/issues/490)) — ADOPTED
`output/websocket.newMetrics` creates fresh Prometheus collectors and unconditionally
calls `PrometheusRegistry().MustRegister(...)`. When a runtime config update restarts
`websocket-output`, the same collector names are registered again in the same registry,
panicking with `duplicate metrics collector registration attempted`.

**Resolution evidence:** SemSource adopted `github.com/c360studio/semstreams
v1.0.0-beta.144` after [semstreams#490](https://github.com/C360Studio/semstreams/issues/490)
closed. The previously blocked gate, `go test -tags=e2e -timeout 300s ./test/e2e/`,
passed on 2026-07-08.

**Framework fix shape:** websocket metric registration is now restart-safe instead
of panicking on duplicate collector registration during restartable component
construction.

**Surfaced by:** validating the SemTeams UI profile OpenSpec slice; all UI-profile
checks passed, but the existing full black-box e2e gate exposed this runtime restart
panic.

---

## Gateways

### 9. graph-gateway MCP endpoint is a stub — framework-shaped
`gateway/graph-gateway` advertises an `mcp_path` (default `/mcp`, in the schema + OpenAPI
doc) and mounts `handleMCP`, but the handler is a **placeholder** — it returns
`{"message":"MCP endpoint"}` with the comment *"In real implementation, this would handle
MCP protocol."* So there is no working MCP surface in the framework, and a service that
configures the graph-gateway exposes a dead `/mcp`. A real, ideally **pluggable** MCP
gateway (tools contributed by components, not hard-wired to graph queries) would let every
sem\* service expose its own tools over one framework MCP implementation.
**Stopgap (semsource):** shipped a **product-shaped** `mcp-gateway` component using the
official `github.com/modelcontextprotocol/go-sdk` (Streamable HTTP) exposing semsource's
source-registration tools, translating tool calls → NATS. If the shape generalizes,
propose lifting the MCP-server machinery upstream so it isn't re-rolled per service.
**Surfaced by:** adding MCP to semsource (ADR-0007 §1; first MCP across sem\*).

### 9b. GraphQL capabilities route points at an unregistered graph-query subject — framework-shaped — candidate
SemStreams beta.144 `gateway/graph-gateway` still routes GraphQL `capabilities`
queries to `graph.query.capabilities`, but `processor/graph-query`'s handler table
does not register a responder for that subject. The SemStreams docs describe
`graph.query.capabilities` as an aggregation surface, so the route/responder
contract is inconsistent.

**Stopgap (semsource):** do not advertise `graph.query.capabilities` in SemSource
consumer docs until SemStreams aligns the GraphQL route and graph-query responder.
**Surfaced by:** re-auditing SemSource docs after the beta.144 adoption for
[semstreams#490](https://github.com/C360Studio/semstreams/issues/490).

---

## Graph indexing at scale (found dogfooding — first real, non-synthetic corpus)

Indexed **beta.124 itself** (21k+ code entities from 957 Go files + 289 docs) in a live
semsource and queried it over MCP. This is the first time the graph/query stack ran on a
real, high-cardinality corpus rather than fixtures. Three findings, one with a CPU profile.

### 10. graph-index `UpdatePredicateIndex` is O(N²) at scale — framework-shaped — **CPU-profiled** — RESOLVED in beta.127 ([semstreams#430](https://github.com/C360Studio/semstreams/issues/430), PR #434) — ADOPTED

**Status:** Shipped. beta.127 replaced the monolithic per-predicate JSON list with composite-key
sharding (`predicateIndexKey(predicate, entityID)` — one unconditional Put per (entityID,
predicate); see gh#474 notes in `processor/graph-index/component.go:2011`). Re-benchmarked on the
same corpus: byName 15/15 within seconds of ready (was 9-10/15 over 6 min), `UpdatePredicateIndex`
63% → 2.15% of ingest CPU, BM25 search functional during ingest. Verified present in beta.153.
The original report is retained below as the profile record.
The dominant cost of indexing a real codebase. A 30s CPU profile during ingest
(`--pprof-port`, see #12) showed **63% of CPU in
`graph-index.(*Component).UpdatePredicateIndex → natsclient.KVStore.UpdateWithRetry`**
(`processor/graph-index/component.go:1305`), with **~37% flat in `encoding/json`**
(unquoteBytes / checkValid / rescanLiteral / stateInString) and ~29% in network syscalls.
Mechanism: the PREDICATE_INDEX stores **one monolithic JSON list per predicate**, updated
per-entity via a **CAS read-modify-write** (`UpdateWithRetry`). For a high-cardinality
predicate (e.g. `code.artifact.type`, carried by ~all 21k code entities) the value grows to
~21k entries, so each of N updates re-reads + re-parses + re-writes an O(N) blob → **O(N²)**,
and under concurrency the hot keys thrash on CAS conflicts (retry storms re-parsing the blob).
Evidence it is NOT worker-bound: raising graph-index `workers` 1→8 moved byName coverage only
9/15 → 10/15 — more workers just add CAS contention on the hot keys. Consequences: indexing
22k entities takes **many minutes** (not seconds), the NAME_INDEX lags, and graph-embedding is
starved (EMBEDDINGS_CACHE had ~1 entry — semantic search effectively non-functional during
ingest). **Fix direction (semstreams):** stop storing a monolithic JSON list per predicate —
e.g. append-only per-entity sub-keys (`PREDICATE_INDEX.<predicate>.<entityID>`), a set/CRDT,
sharded keys, or batched/coalesced index writes. **Surfaced by:** indexing beta.124 in semsource
(dogfood). Profile artifact retained in the session.

### 11. `phase: ready` fires long before the query indexes are populated — framework-shaped — filed [semstreams#431](https://github.com/C360Studio/semstreams/issues/431)
Consumers gate on `graph.query.status` → `phase: ready` before querying (ADR-0003). But at 22k
scale `phase` flips ready at ~30s while byName/NAME_INDEX + embeddings keep populating for
**minutes** afterward (byName hit-rate climbed 17% → 50% → plateauing over 6+ min). So a
consumer that correctly waits for `ready` still gets unreliable byName/search results. Readiness
should reflect **query-index completeness**, or the contract should distinguish "ingest ready"
from "indexes ready" (and expose index-build progress). Related to #10 (the indexes are slow
because of it). **Surfaced by:** the same dogfood; byName coverage measured over time.

### 12. `service.MaybeStartPProf` validated under real load — works ✓ (note)
Not a gap — a positive. semsource added a `--pprof-port` flag (blank-import `net/http/pprof` +
`service.MaybeStartPProf`); the pprof HTTP server came up and produced the profile that found
#10. This is believed to be the first hard use of that helper "as a service" — it works.

## Semantic tiers (ADR-0002 / model registry)

### 13. `graph-embedding` has no asymmetric query/document embedding — framework-shaped — RESOLVED in beta.129 ([semstreams#438](https://github.com/C360Studio/semstreams/issues/438), PR #440)
**Status:** Shipped. beta.129 added `EndpointConfig.query_prefix` + a `GenerateQuery` interface method
(HTTP prepends the prefix on the query side only; BM25 `GenerateQuery == Generate`). semsource set
`query_prefix` on the semembed endpoint in `configs/tiers/tier1-semantic.json` / `tier2-semantic-instruct.json`.
Re-ran the A/B (dogfood, 21k entities): with the prefix, tier-1 `code_search` beats tier-0 BM25 on
paraphrase queries BM25 misses (e.g. *"prevent processing the same message twice"* → the msgid dedup
test; *"compute a sha256 hash"* → the content-addressed cache) and stays relevant at full population
(no-prefix degraded to `const` noise). (Original ask below.)

The HTTP embedder (`graph/embedding/http_embedder.go`) exposes a single
`Generate(ctx, texts)` and both ingest (documents) and `graph.query.semantic`
(the query) go through it identically — there is no query-side instruction
prefix / `EmbedQuery` path. The dominant open-weight retrieval embedders
(Snowflake **arctic-embed**, **BGE**, **E5**) are trained *asymmetrically*: the
query must be prefixed (arctic/E5: `"Represent this sentence for searching
relevant passages: "`; BGE has its own) while documents are embedded raw.
Omitting the query prefix is a well-known quality cliff, not a rounding error.

**Measured** (semsource dogfood, tier-1 wired to semembed `arctic-embed-s`,
21,648 entities, embeddings complete: `embeddings_generated_total=8096`,
`content_resolved_total=7320`, `errors_total=0`): direct cosine of the query
*"retry with backoff"* against a relevant `retry.Do` body vs a noise `const`:

| query embedding | cos(relevant) | cos(noise) | margin |
|-----------------|---------------|------------|--------|
| **no prefix** (current) | 0.8162 | 0.7264 | **+0.090** |
| **with arctic prefix** | 0.6527 | 0.4947 | **+0.158** |

The prefix **~doubles the relevant-vs-noise margin (+0.090 → +0.158, +76%)**.
Live `code_search` at tier 1 (no prefix) confirmed the symptom: short generic
entities (`const`/`var`) crowd out the correct hit, and results *degrade* as
more entities embed — so as-wired tier-1 semantic search underperforms tier-0
BM25 on these queries. **Fix direction (semstreams):** let the embedder (or
`graph-query`'s semantic path) apply a configurable per-model query instruction
— e.g. `EndpointConfig.query_prefix`, or an `EmbedQuery` distinct from `Embed`
that graph-query calls. Until then, tier-1 quality is capped regardless of
model size. **Surfaced by:** wiring the http embedder (semsource task #35).
Product-side note: even with the prefix, a 33M general model discriminates code
weakly — a code-specialized or larger embedder is the complementary lever.

## Fusion ranking (RankSignals, ADR-062 increment 5)

### 14. `fusion.RankSignals` is additive-only — no way to down-rank noise — framework-shaped — RESOLVED in beta.130 ([semstreams#441](https://github.com/C360Studio/semstreams/issues/441))
**RESOLVED (beta.130):** `PredicateSalience` is now **signed** — a negative weight demotes
("reorders behind … a bounded secondary reordering, never an exclusion"); the engine folds an
entity's strongest boost and strongest demotion together (`entitySalience`). This landed fix
direction (1). Unblocks the demote-complement of task #38 (down-rank tests/generated code).
(An earlier ADR-0008 draft also proposed a soft-staleness demotion tier on top of this; that draft
was withdrawn as over-engineered — see ADR-0008 — so this ask now stands solely on the task-#38
demote-complement.) Original report below.
`PredicateSalience` returns a weight ≥ 0, `entitySalience` takes the **max** over
an entity's predicates, and the engine **adds** `salienceScale × maxWeight` — so a
consumer can boost high-value entities but can never **demote** low-value ones. For
code retrieval the entities I most want *out* of the top hits are structurally
identifiable but unboostable-away: **tests** (`*_test.go`, mocks — they carry the
same boosted predicates as the real impl, e.g. a test with a doc-comment gets the
same salience as `retry.go`) and **generated code**. A negative `WithWeight` is
inert (max over predicates never picks it; the model has no "subtract"). **Fix
directions:** (1) allow negative salience + sum-with-floor (or a `PredicateDemotion`
that subtracts); (2) value-conditioned `PredicateSalience(predicate, value)` — also
solves boost-public-over-private (visibility is a *value* on a universal predicate,
so per-predicate-name weight can't reach it); (3) a distinct `Penalty` signal the
engine subtracts, preserving the "strictly additive" invariant on salience.
**Product half is ours** (semsource task #38: emit a presence predicate
`code.artifact.exported` + weight it — the boost side needs no framework change; the
demote side is the framework gap here). **Surfaced by:** wiring semsource's fusion
ranking (`.WithSignals(fusionvocab.New())` + predicate `WithWeight`, PR #36).

## Graph mutation / retraction lifecycle (ADR-0008)

### 15. Deletion is incomplete in the framework — two halves — framework-shaped
**Both halves are OFF semsource's critical path** as of ADR-0008: the graph is now
**retention-first** (versioned sources retained + related by supersession; "current" is a
ranking marker), so we do **not** delete by default. Deletion survives only as a rare,
graph-aware exception for **mistakes/churn** — and that exception needs *both* halves below
before it is safe. Logged so they are ready if/when the mistake-cleanup path is built.

**(a) Index cleanup on delete — filed [semstreams#433](https://github.com/C360Studio/semstreams/issues/433) (not by us, OPEN).**
`Component.DeleteEntity` removes the entity row but leaves `PREDICATE_INDEX` / `NAME_INDEX` /
`ALIAS_INDEX` / `CONTEXT_INDEX` populated — a deleted entity still answers
`byName`/predicate/prefix queries.

**(b) Referential integrity on delete — NEW candidate (not yet filed).** Deleting `A` cleans
`A`'s own adjacency buckets but does **not** rewrite the *referrer*: a triple `B —calls→ A`
lives on `B`, so `A`'s deletion leaves a **dangling edge** on `B`. Reference-blind eviction
(NATS TTL/MaxBytes) has the same failure mode, which is why ADR-0008 **rejects** NATS-policy
retention on the live graph outright. A complete delete is a **cascade** (find referrers via
the incoming index, fix/remove the dangling assertions) or a **refuse-if-referenced**. Ask:
make `DeleteEntity` referentially complete (cascade or refuse) — pairs with #433 as the two
halves of "delete is a first-class, integrity-preserving operation." File when the exception
path is actually built; until then the ADR guardrail (no eager delete) makes it moot.

*(Write-path context, not an ask: semsource emits via publish → `graph-ingest` `MergeEntity`
**append** — `component.go:1794` — so it never removes anything today; enumeration for a future
scoped delete is free via `graph.query.prefix`.)*

## Fusion retrieval scope (ADR-0004)

### 16. Domain-scoped NL retrieval for a fusion lens — framework-shaped — RESOLVED in beta.141 ([semstreams#463](https://github.com/C360Studio/semstreams/issues/463), ADR-071) — ADOPTED
**RESOLVED (beta.141):** `fusion.Request.Scope []string` — OR-matched dot-delimited
entity-ID prefixes (fix direction (1), NL-only), applied at the candidate source in
`graph-embedding.findSimilarEntities` via `graph.MatchesAnyIDPrefix` (empty = no
filter = prior behavior). **ADOPTED (semsource):** the fusion gateway
(`processor/code-context/component.go` `serve`) now defaults `req.Scope` per lens
when the caller sends none — `docs` → `{org}.semsource.web`, `code` → the
code-language domain prefixes (`golang`/`python`/`typescript`/`javascript`/`java`/
`svelte`). The `{org}` segment is the deployment's single global org, sourced free
from `deps.Platform.Org` (= the required top-level `namespace`, forced onto every
source), so no new config surface. A caller-provided `Scope` is respected verbatim;
no org (standalone/tests) → no default = prior behavior. OpenSpec change
`domain-scoped-fusion-retrieval` (capability `fusion-gateway`). The filter
application is framework-tested + live-validated (the httpx mixed code+doc dogfood
below); semsource's seam — correct per-lens scope selection reaching the wire — is
covered by unit + a real-NATS integration test.
The `pkg/fusion` engine resolves NL seeds over the **whole shared embedding index**
with no per-lens/per-request domain filter: `Engine.fuse` calls
`graph.Resolve(query, mode, limit)` where `mode ∈ {symbol, prefix, nl}`, and the
`Lens` interface exposes no scope hook (its methods are Name / ResolveMode / Edges /
Label / Kind / Location / Hydrate). `fusion.Request` carries only `Query` + `Want`.
Consequence for semsource: `code-context` and `doc-context` are two lens instances
over **one** embedding index, so in a **code-heavy corpus a small doc set is diluted**
— an NL `doc_context` query can rank code entities above the relevant document
(measured live: httpx, 1304 Python code entities vs 30 docs → `doc_context "what
exceptions can be raised"` returned Python test funcs above `docs/exceptions.md`; the
same query with docs not drowned returns 100% documents, top-1 correct). Hydration is
**not** the problem (the doc body producer works — verbatim passages hydrate; ADR-062).
**Fix directions:** (1) an optional `Scope`/entity-type-prefix filter on `fusion.Request`,
plumbed to the embedding search (`graph.embedding.query.search`) so a lens instance can
constrain seeds to its domain (e.g. `*.web.*.doc.*`); (2) a `Lens.SeedFilter(entity) bool`
hook the engine applies post-retrieval over an over-fetched candidate set; (3) a
per-lens embedding namespace. **Product half is ours** (choosing/wiring the scope per
lens once the hook exists). **Not MVP-blocking:** `code_search` retrieves docs well and
`doc_context` is accurate when docs aren't drowned; this bites only mixed code+doc
corpora. **Surfaced by:** Tier A #3 / B #4 live validation (Python repo + docs).

### 17. Per-facet edge selection for the fusion engine (impact ≠ relations) — framework-shaped — RESOLVED in beta.140 ([semstreams#475](https://github.com/C360Studio/semstreams/issues/475), PR #481) — ADOPTED
**RESOLVED (beta.140):** `EdgeSpec` gained `Facets []Facet` (`nil` = all facets; fix
direction (1) — the per-spec facet mask). **ADOPTED (semsource):** the code lens now
tags `CodeContains` with `Facets: {FacetRelations, FacetPaths}`, so containment still
populates the container/contents relations but is excluded from the impact walk —
`code_impact` is now a pure reverse-dependency closure (the httpx `BaseClient` impact=5
= 2 subclasses + 3 containers case). Regression: `TestCodeLens_ImpactExcludesContainment`.

Original ask below (for history):
`fusion.Lens.Edges()` returns ONE `[]EdgeSpec` list the engine uses for THREE facets:
`computeImpact` (incoming BFS), `computePaths` (outgoing DFS), and the `relations` map
on each node. A lens cannot say "walk this edge for relations but not for impact."
Concretely: semsource's code lens must include `CodeContains` (file→symbol) so
`code_context` shows a symbol's container/contents — but that same edge then pollutes
`code_impact`, whose incoming-contains walk pulls every dependent's file→folder→repo
into the blast radius. So the impact *count* mixes structural containment ancestry with
real reverse-dependents (measured: httpx `BaseClient` impact = 5 = 2 subclasses + 3
containers). **Fix directions:** (1) an optional per-`EdgeSpec` facet mask (e.g.
`Facets: impact|paths|relations`) so a spec opts out of the impact/paths walk; (2)
separate `ImpactEdges()`/`RelationEdges()` lens methods; (3) engine treats containment
predicates specially. **Not MVP-blocking:** impact is directionally useful and bounded
(`maxImpactNodes`); the count is just noisier than a pure dependency closure.
**Surfaced by:** task #43 adversarial review (adding type-dependency edges made the
containment compounding visible).

### 18. Governed graph projection facet for fusion v2 consumers — framework-shaped — RESOLVED in beta.153 ([semstreams#533](https://github.com/C360Studio/semstreams/issues/533), [PR #577](https://github.com/C360Studio/semstreams/pull/577)) — ADOPTED

**RESOLVED (beta.153):** SemStreams added the requested additive `want: ["graph"]` fusion facet in
PR #577 (`f54c06bd`) and released it in
[`v1.0.0-beta.153`](https://github.com/C360Studio/semstreams/releases/tag/v1.0.0-beta.153). Non-requesting
fusion v1 callers retain their existing response shape. The facet supplies typed property facts,
explicit directed source/predicate/target edges, opaque handles, optional verbatim evidence,
independent fact/edge/projection truncation, and a pre/post-fetch `ViewRevision` with a `Coherent`
flag.

**ADOPTED (SemSource):** SemSource pins beta.153 and requests the facet through the existing
`POST /code-context/context` endpoint with `want: ["graph"]`. The compatibility test
`TestHTTPGraphProjectionCompatibility` exercises the real fusion engine through that HTTP route;
owned browser contract/model/component tests cover strict facts/edges/evidence parsing, explicit
unresolved endpoints, opaque handles, deterministic rendering, and non-deletion when the projection
is truncated, incoherent, or has no meaningful nonzero revision. No new projection endpoint or
GraphQL dependency was added.

This RESOLVED/ADOPTED status covers both dependency/backend adoption and the SemSource workbench:
tasks 5.2, 5.3, and 7.2 prove the WebGL renderer, valid no-graph behavior, classified errors,
stale-revision protection, and ready-graph browser/accessibility/live-route path.

Two upstream limits remain explicit rather than repaired locally: incoming edges for lens-undeclared
predicates are not discoverable, and stored confidence `0.0` is currently indistinguishable from
unset and therefore projects as absent. Product lenses must declare incoming predicates they need;
SemSource does not infer either missing relationship vocabulary or evidence.

Original ask below (for history):

The lens-driven fusion v1 response is sufficient for bounded search, body, role-based relations,
paths, and impact views, but it intentionally drops information required by a canonical graph
drill-down. `Node.Relations` maps lens roles to human `Ref` values without target handles, original
predicates, direction, stable edge identity, or fact evidence; underlying triple properties and
their source/timestamp/confidence metadata are also absent. The per-role relationship cap is silent,
and the index catch-up watermark is not a coherent response/view revision.

**Ask:** add an optional governed projection facet, or an explicitly versioned fusion response, that
returns typed property facts; explicit directed edges with source/target handles and original
predicates; per-fact/per-edge evidence without fabricated defaults; preservation of parallel and
opposite-direction facts; relationship truncation metadata; and documented response consistency or
view-revision semantics. Preserve fusion v1 compatibility and keep product lenses responsible for
domain predicates and human roles. Existing NATS/HTTP/MCP transports can carry the response;
GraphQL exposure is not a prerequisite.

**SemSource boundary:** continue using fusion v1 through `/code-context/*` and `/doc-context/*` for
workbench search/list/detail/impact views. Adopt the optional graph facet through the existing context
request. Do not build a parallel SemSource graph projection, adapt role maps into invented
directed/evidenced edges, or make GraphQL a prerequisite.

**Related:** semstreams#376 (fusion framework primitive), semstreams#367 (graph-visible provenance
conventions), OpenSpec `add-opt-in-source-workbench` D9/tasks 3.1–3.2.

### 19. Impact facet should name direct dependents (bounded, truncation-labeled) — framework-shaped — filed [semstreams#603](https://github.com/C360Studio/semstreams/issues/603)

`fusion.Impact` is counts-only (`{nodes, files, truncated}`): the blast radius is sized but
never named, so "what depends on this" costs a follow-up query per node. semsource's interim
(go-callgraph-recall D5) adds `WantRelations` to its impact verb so seed nodes carry the
reverse-role refs (caller/extended_by/implemented_by/referenced_by/embedded_by) — but that is
one product's want-set workaround, the per-role `maxRelationsPerNode` cap truncates SILENTLY,
and deeper closure members stay nameless. Ask: the Impact facet itself names at least the
direct dependents (bounded, per-role/per-facet truncation labeled, human `Ref`s like the
relations facet), so every fusion consumer gets named blast radii without widening `Want`.
Generalizes beyond code (docs backlinks, config dependents).
**Surfaced by:** audit 2026-07-19 graded Q7 — `code_impact` returned bare counts for
`SystemSlug`; the answer "5 nodes / 3 files" grades wrong when the asker needs *which* callers.
**Interim in semsource:** exact-seed decorator + WantRelations default (go-callgraph-recall).

### 20. `CONTENT` ObjectStore carries a hard-coded 24h TTL — verbatim bodies silently vanish — framework-shaped — filed [semstreams#600](https://github.com/C360Studio/semstreams/issues/600)

`objectstore.NewStoreWithConfigAndMetrics` creates every bucket with
`TTL: 24 * time.Hour` as a literal (`storage/objectstore/store.go:114`, beta.153). There is no
config field and no override — every semsource attach point routes through that one constructor
(`ast-source/bodystore.go:48`, `doc-source/component.go:157`, `code-context/component.go:223`,
`supersession/versiondiff_serve.go:40`).

**Consequence.** Shipped configs run `watch: false` (`configs/mvp.json`, `configs/semsource.json`),
so a body is `Put` once at ingest and never rewritten. On a deployment that does not restart
daily, **every verbatim body expires ~24h after ingest** while the referencing `ENTITY_STATES`
triples live forever. The failure is silent by contract: a resolver that cannot find the key
"yields a (nil, nil) body with NO error" (`graph/bodystore.go:5-8`), so `code_context`,
`doc_context`, and `code_changes` before/after bodies degrade to empty rather than erroring. A
container that restarts daily masks it completely, which is why it survives testing.

**It contradicts the substrate's own stated position.** ADR-0008:145 rejects reference-blind
NATS-policy retention on the live graph outright, and `ENTITY_STATES` is defended at boot by
`AssertNoLifecycleRetention` (`processor/graph-ingest/component.go:1107`). `CONTENT` gets no such
guard, and the TTL there is the framework default rather than a misconfiguration.

**Ask:** make the ObjectStore TTL configurable (default off for content-addressed body stores),
and extend the `AssertNoLifecycleRetention`-class guardrail to `CONTENT`.

**Coupling that must not be missed.** That accidental TTL is currently the *only* thing reclaiming
orphaned blobs: bodies are content-addressed (`doc:<sha>` / `code:<sha>`), so an edit writes a new
key **alongside** the old and strands the previous object with no refcount, sweep, or GC anywhere
in either repo. Raising or removing the TTL without an orphan-reclamation story converts the blob
store into genuinely unbounded growth. Both halves belong to the same retention design — see
ADR-0008 open item #5 (per-source retention depth, designed but unimplemented) and ask #15.

**Re-verified against beta.156, 2026-07-21** — the TTL literal, the once-only write, and the silent
`(nil, nil)` resolver miss are all still present.

**Surfaced by:** doc-passage-chunking storage review, 2026-07-20. Not a blocker for that change —
passages tile the source file exactly, so total stored body bytes are unchanged and per-edit blob
churn drops from O(file) to O(changed passage) — but it is a live production correctness bug on
its own.

### 21. Offloaded entities never embed their title — framework-shaped — filed [semstreams#601](https://github.com/C360Studio/semstreams/issues/601)

`graph-embedding` has two lanes for producing embedding text. The StorageRef lane returns
immediately once it has queued the offloaded body (`processor/graph-embedding/component.go:1150-1153`),
so the inline-triple lane below it is unreachable for any entity carrying a `StorageRef`.

**Consequence.** For every body-bearing entity — every code symbol and every doc passage SemSource
produces — `dc.terms.title` and every other short identifying fact is **excluded from the embedding
vector**. Only body bytes are embedded. A passage titled "Retry Policy § Exponential Backoff" whose
body never repeats those words is unreachable by a query naming them.

It also makes the `text_suffixes` configuration silently inert for exactly the entities it was tuned
for. SemSource restates the defaults and adds `.signature`/`.comment`
(`cmd/semsource/run.go:830-833`) specifically so code signatures and docstrings enter the semantic
index — but those predicates live on entities that also carry a `StorageRef`, so the setting has no
effect on them. The config appears to work and does nothing.

**Ask:** embed the title/identity triples alongside the offloaded body (concatenate, or embed both
and keep the better score), rather than letting the StorageRef lane short-circuit them away. At
minimum, make the exclusion visible — the current behaviour is indistinguishable from working.

**Surfaced by:** doc-passage-chunking design review, 2026-07-20. Independent of chunking; it applies
to every offloaded entity SemSource has ever produced.

### 22. The embedding text cap is hard-coded at 8000 characters — framework-shaped — filed [semstreams#602](https://github.com/C360Studio/semstreams/issues/602)

`maxTextLen` is a literal in `processor/graph-embedding/component.go:786-790` (8000, or 4000 for
bm25), passed to `WithMaxSourceTextLen`; `graph/embedding/worker.go:23` carries the same default.
There is no config field, so a producer cannot raise it, lower it, or discover it.

**Consequence.** Any entity whose body exceeds the cap is truncated at a word boundary and the
remainder is **silently dropped from the semantic index** — no error, no metric, no log at
producer level. Measured on the SemSource repository before passage chunking: 47 of 218 Markdown
files exceeded it, and ~252 KB of prose was unindexed, including roughly three quarters of the
project README.

**Ask:** expose the cap as component config (keeping today's value as the default), and count
truncations so the loss is observable rather than silent.

**Not blocking us.** SemSource now splits documents into passages sized well under the cap, so this
is no longer a correctness issue for docs — the split is bounded by a hard maximum precisely because
the framework's cap cannot be configured or detected. It still applies to any producer with
legitimately large single bodies, and the silence is the part worth fixing regardless.

**Surfaced by:** doc-passage-chunking, 2026-07-20.


### 23. Strict catch-up readiness is reachable after any finite write burst — framework-shaped — RESOLVED in beta.156 ([#592](https://github.com/C360Studio/semstreams/issues/592) / [PR #593](https://github.com/C360Studio/semstreams/pull/593)) — ADOPTED

**Status:** Shipped. Our evidence on [#590](https://github.com/C360Studio/semstreams/issues/590) was split
out as #592 (read path) from #591/ADR-082 (community detection). The research **rejected** the
bounded-stale read we proposed — retry-the-transient is the correct contract for exact consumers,
because a bounded-stale fusion resolve can return a just-written symbol as an authoritative MISS, the
false-negative ADR-066 exists to prevent. It found a real bug instead: `Fuse` handled
`ErrorCodeIndexNotReady` INCONSISTENTLY — the top gate degraded to an empty-honest envelope,
`collectEdges` swallowed it, but `Resolve`/`Entities` propagated it as a hard error. That is exactly
the "passes 5/5 alone, fails under full-suite load" shape we reported. beta.156 makes the degrade
consistent.

**Correction adopted locally:** PR #593 classifies on the stable code
(`errors.As` + `Code == graph.ErrorCodeIndexNotReady`), explicitly NOT `errs.IsTransient`, because a
real connection timeout is also transient and must propagate as a hard error rather than be silently
degraded. Our first fix used `errs.IsTransient` and was too broad — it would have retried a genuine
outage until the deadline, converting a reportable failure into a mute timeout.
`internal/governance/fuse_retry_integration_test.go` now matches their classification.

Original report retained below.

### 23a. Original report

`ComputeIndexStatus` gates on `ready := target > 0 && indexed >= target`
(`graph/index_status.go`), and `graph-index` answers queries below that bar with a classified
transient `ErrorCodeIndexNotReady` (`processor/graph-index/query.go:183`).

#590 reports this from a continuous-write load (semboids), where readiness never flips at all, and
carries a caveat: 8.5 minutes may not be enough to distinguish "the gate is unreachable by
construction" from "this load simply outruns graph-index."

**SemSource is the control case for that caveat.** Our integration suite writes a FINITE burst —
ingest a few entities, writes stop, then query — so lag cannot climb without limit. The gate still
returns not-ready to a query arriving right after the burst. The reachability problem therefore is
not only a firehose phenomenon; it opens a window after any ordinary write burst, independent of how
semboids' lag behaves over a longer soak.

Milder here than in semboids, in a way that isolates the mechanism: `indexBootstrapped` is sticky, so
we flip once and then run clean, and the failure is load-dependent (the failing test passes 5/5 alone
and fails only under full-suite load).

**Local impact, already fixed:** nine fusion poll loops in `internal/governance` called `t.Fatalf` on
this transient inside loops written to wait for exactly that condition, so a benign self-resolving lag
read as a red build. They now retry on `errs.IsTransient` — detection by classification, never message
text. See `internal/governance/fuse_retry_integration_test.go`.

**Why it surfaced now:** doc passage chunking multiplies doc entity count ~7x (218 files → 1497
entities on this repo), widening the catch-up window on every ingest. The gate was always reachable;
more entities made it reliably so.

**Position:** no ask of our own beyond the bounded-staleness tolerance already proposed in #590.
Evidence contributed; the decision is theirs.

### 24. Fusion silently drops the top-ranked entity from the first call after a query burst — framework-shaped — filed [semstreams#597](https://github.com/C360Studio/semstreams/issues/597)

`Fuse` can return an answer that omits the highest-scoring candidate entirely, with no error, no
log, and nothing in the response indicating anything was lost.

**Reproduction.** `scripts/scorecard/repro-order-dependence.sh` in semsource. In one MCP session:
21 diverse `doc_context` queries, then the target query four times.

```
call 1: ABSENT
call 2: rank 1
call 3: rank 1
call 4: rank 1
```

Only the first call after the burst fails, and it self-heals on the very next identical call.
Other passages of the same document survive the failing call (ranks 9 and 18) — it is specifically
the top-ranked entity that vanishes.

**Recall is intact.** At the same instant, `graph.query.semantic` — bypassing fusion — returns that
passage at rank 1, similarity unchanged. Running the burst and then making `graph.query.semantic`
the FIRST call after it also returns a pristine list: identical similarities in identical order.
So query embedding, cosine scoring and candidate selection are all fine, and the entity is lost
downstream of recall.

**Eliminated, so the investigation need not repeat them:**

- *Embedding service degrading under load* — see the pristine-recall test above; zero errors in the
  embedder log for the whole session.
- *Mixed statistical/neural vector population* — `embedder_type` resolves once at startup
  (`createEmbedder` is a hard `bm25`/`http` switch with no escalation and no replacement pass).
  Sampled stored vectors are 384-d, 0% zero components, ~50% negative; a BM25 vector from
  `computeBM25Vector` is sparse and non-negative (IDF clamped to +0.01, unhashed dimensions left at
  zero).
- *Unstable sorting over tied cosines* — repeated recall is byte-identical including at exact ties
  (two entities at 0.6124 keep their order across five runs).

**Most plausible remaining mechanism**, offered as a hypothesis and not a finding: seed hydration.
`fusionnats.Entities` calls `graph.query.batch`, whose contract silently omits IDs it cannot
return, under a 5 s per-graph-call timeout (`fusionnats.New(client, 0)`). A cold or slow first read
after a burst would drop the ID with no trace.

**Blast radius, stated precisely.** Reproduced on `doc_context`. `code_search` — also
embedding-backed — returned an identical top node across four consecutive calls after the same
burst. That is not proof the code lens is immune, only that the burst did not perturb it; the
lenses differ in how they seed (`ResolveModeNL` vs `exactSeedClient`), which is where to look if it
ever does.

**Ask, part 1:** seed hydration must not silently drop an ID. Either surface the omission in the
response envelope or fail the call. A caller cannot currently distinguish "this entity does not
match" from "this entity was dropped".

**Ask, part 2 — the more valuable half: fusion has no score observability at all.** Diagnosing this
required bypassing the product surface entirely and calling `graph.query.semantic` over raw NATS,
because:

- `fusion.Node` carries no score, rank, or similarity field.
- `pkg/fusion/engine_lens.go` has no logger and emits nothing; the `scored` struct is
  function-local and discarded.
- `fusionnats.resolveSemantic` receives `SearchResult.Similarity` and **decodes only `entity_id`**,
  throwing the similarity away (`pkg/fusion/fusionnats/client.go:153-168`).

Any consumer hitting a ranking surprise has no recourse. Even an opt-in debug field carrying the
resolve rank and the score contributions would have turned this investigation from a day into
minutes.

**Consumer impact.** This is not only a test-harness concern: an agent's first `doc_context` call
after a busy period can silently lose the single best piece of evidence and cite the second-best as
though it were the top result.

**Surfaced by:** semsource `repair-retrieval-scorecard`, 2026-07-20. Full measurement in
`scripts/scorecard/results/SUMMARY-instrument-diagnosis.md`.
