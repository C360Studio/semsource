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
binding safe by construction. Low urgency: semsource's own stream binds only the five
data-plane subjects, and the headless guard `warnIfHostStreamCapturesRPCReplySubjects`
(`cmd/semsource/run.go`) now warns on both the curator and read-path RPC subjects.
**Surfaced by:** Fusion full-pipeline integration test (ADR-0004) — a `graph.ingest.>`
test stream zeroed the fused response; root cause confirmed against the existing
run.go guard, which previously probed only the curator subjects.
