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

### 8. Runtime config writes skipped by engine-owned-revision watcher — framework-shaped — filed [semstreams#388](https://github.com/C360Studio/semstreams/issues/388)
`ConfigManager.handleUpdate` skips events whose revision `<= engineHighWaterRev`
and `return`s **before notifying subscribers** — contradicting its own doc comment
("the skip only affects the in-memory cache update, not subscriber notification").
`PutComponentToKV`/`DeleteComponentFromKV` bump the high-water without applying
in-memory, so a runtime add is written to KV but **never spawned**, and a remove is
**never torn down** (the `ComponentManager` is never notified to reconcile).
**Stopgap (semsource):** runtime *add* mutates the in-memory config + bumps the
config version + `PushToKV` (which DOES notify) → spawns correctly. Runtime *remove*
can't use the same trick — driving the reconcile-stop from the request handler
deadlocks — so remove-teardown stays broken pending the fix. **Blocks:** gating CI
on e2e (`TestE2E_RuntimeSourceAdd` remove-teardown assertion) — deferred until #388.
**Surfaced by:** wiring e2e into CI (curator runtime add/remove, ADR-040 / ADR-0006).
