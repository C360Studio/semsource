# ADR-0005: BFO/CCO Ontology Alignment for Ranking

> **Status:** Accepted | **Date:** 2026-06-27

## Context

ADR-0004 establishes a deterministic fusion gateway whose `assemble` stage
ranks results. The spike's ranker was ad-hoc lexical (exact/prefix/contains +
visibility). We want **ontology-driven ranking** — the well-established
ontology-IR mechanism (weight by concept match, hierarchy specificity, and
semantic relatedness) — and our sponsors and academic/government stakeholders
specifically value **BFO/CCO** alignment ("standards at work," not standards
bolted on at export).

**semstreams already provides the foundation** (beta.116; confirmed not yet
used by semsource):

- `vocabulary/bfo` — full BFO 2.0 (ISO 21838-2) class tree + relations as
  OBO-PURL IRI constants (`Entity`→`Continuant`/`Occurrent`, `Object`, `Quality`,
  `Role`, `Process`, `PartOf`, `ParticipatesIn`, …).
- `vocabulary/cco` — Common Core mid-level on BFO, and it already covers our
  domain: `SoftwareCode`, `Algorithm`, `Document`, `Specification`, `Requirement`,
  `PlanSpecification`, `Person`/`Agent`, `Act*`, and relations `IsAbout`,
  `Prescribes`, `AuthoredBy`, `CreatedBy`, `Designates`.
- `vocabulary/export` — `Serialize(triples, Turtle|JSON-LD|N-Triples)` over our
  existing `message.Triple`, resolving registered predicate IRIs.

The framework's philosophy (semstreams ADR-042) is exactly ours: **IRI
alignment + export, no reasoner, no SPARQL, no OWL inferencing inside.** So this
is a *mapping* effort on a ready foundation, not an ontology build.

## Decision

**Adopt BFO/CCO by aligning semsource's internal model to the existing
`vocabulary/bfo` + `vocabulary/cco` IRIs, and use that alignment as the primary
ranking signal in the fusion engine. RDF/OWL/JSON-LD export is deferred to an
optional edge (`vocabulary/export`) built IF/WHEN a consumer needs it.**

### Alignment layer (`source/ontology/`)

A semsource-owned mapping package (the adopter maps its own entities, per
ADR-042; the IRI constants stay in semstreams):

- **Entity-type → class map + lookup.** `ClassFor(entityType) → IRI` and the
  inverse, over `bfo`/`cco` constants. MVP coverage (grows organically):

  | semsource entity | class |
  |---|---|
  | code function / method | `cco.Algorithm` (other code symbols → `cco.SoftwareCode`) |
  | file | `cco.InformationBearingArtifact` |
  | repo | `bfo.Object` (`cco.Artifact`) |
  | doc | `cco.Document` |
  | spec doc | `cco.Specification` |
  | requirement | `cco.Requirement` |
  | git author | `cco.Person` |
  | git commit | `cco.Act` |

- **Emit a class-alignment triple per entity** at the existing emission seam —
  predicate `entity.ontology.class`, object = the BFO/CCO class IRI. This is
  identity-level and **low cardinality** (one per entity, not a fan-out storm),
  and it lands the standard *in the graph* so later export is free.

- **Predicate IRI alignment** via `vocabulary.Register(..., WithIRI(...))` at
  registration, mapping our predicates to standard relations where they exist
  (e.g. `code.structure.contains` → `bfo.HasPart`; doc authored-by →
  `cco.AuthoredBy`; doc about → `cco.IsAbout`).

- **Minimal static subclass subtree** (Go map) of just the BFO/CCO classes we
  use (e.g. `SoftwareCode ⊑ ICE ⊑ GenericallyDependentContinuant ⊑ Continuant
  ⊑ Entity`). Pragmatic and performant — a fixed table, **not** a reasoner. Lets
  the ranker compute hierarchy distance/specificity. (Candidate to upstream into
  `vocabulary/bfo|cco` as a `SubClassOf` map later.)

- **Operator override predicate** `entity.ontology.class` set explicitly on a
  source pins the class, bypassing the auto-map (mirrors ADR-042's
  `agent.capability.oasf_class`).

### Use: ontology-aware ranking (fusion engine `assemble`)

The fusion engine's scorer combines: embedding similarity (neural) + **ontology
signal** (query→class match; hierarchy distance via the subtree; specificity —
deeper/more-specific class weighted higher, per ontology-IR) + lexical fallback
+ triple `Confidence`. Lenses parameterize the weights. This supersedes the
ad-hoc `RelatedDomains` idea in ADR-0004.

## What this is NOT

To keep the core pragmatic/performant and the semweb baggage contained:

- **No OWL reasoner, no RDF Schema inferencing, no SPARQL** inside semsource.
  The subclass subtree is a static Go table read at rank time.
- **No requirement to author mappings in RDF/Turtle/JSON-LD.** Config is plain
  Go/JSON; the alignment is code + a low-cardinality triple.
- **No export built now.** Turtle/JSON-LD/N-Triples via `vocabulary/export` is an
  **optional edge for later**. The class IRI is in the graph, so it's *cheap* —
  but not literally free: `vocabulary/export` currently renders an absolute-IRI
  string object as a literal, not a resource, so it must first learn to emit
  `rdf:type` with an IRI-node object (upstream ask #4 in
  `docs/upstream/semstreams-asks.md`). Built only when a real consumer asks.
- **Not full ontology coverage.** Software-specific relations CCO lacks
  (`code.relationship.calls`, `code.dependency.imports`) stay domain-local or
  take a `semsource/`-prefixed **extension** IRI (ADR-042 extension scheme) —
  honest partial coverage that grows, not a forced bad mapping.

## Consequences

### Positive

- The internal model is BFO/CCO-aligned ("standards at work") for the
  gov/academic audience, on framework packages we didn't have to build.
- Ranking gains a principled, standard hierarchy signal instead of ad-hoc
  lexical/domain heuristics.
- RDF export becomes a cheap, additive edge later — the alignment triples are
  already in the graph.

### Negative / costs

- New emission step (one alignment triple per entity) + a maintained
  entity-type→class map and a small subclass table.
- Partial relation coverage; some software relations carry only extension IRIs
  until CCO grows or we propose extensions.
- A judgment call on a few mappings (e.g. function = `Algorithm` vs
  `SoftwareCode`); documented in `source/ontology/` and override-able.
- The class is a pure function of (domain, type), so the emitted triple is
  *redundant* with the ID and can go **stale** if the map changes until entities
  re-emit. Accepted deliberately: emitting makes the class graph-queryable by
  downstream consumers (SemSpec/SemDragon) and export-ready, which rank-time
  derivation would not. The ranker may still derive at rank time as a fallback.

## Implementation outline

1. `source/ontology/`: class map + lookup, static subclass subtree, override
   resolution, golden test against the `bfo`/`cco` constants.
2. Emission: stamp `entity.ontology.class` at the entity seam (ast-source + the
   handler `EntityState` paths); honor the override predicate.
3. Registration: add `WithIRI(...)` alignment to predicate registrations in
   `source/ast/vocabulary.go` and `source/vocabulary/`.
4. Fusion engine: ontology-aware scorer in `assemble` reading the class tag +
   subtree; lens-weighted (ADR-0004 Fusion A).
5. (Deferred) optional RDF export gateway via `vocabulary/export`.

## Related

- ADR-0004 — deterministic fusion gateway (this provides its ranking signal).
- semstreams ADR-042 — taxonomy-adoption pattern this is modeled on.
- semstreams `vocabulary/{bfo,cco,export}` — the reused foundation.
- Ontology-driven IR: concept weighting + hierarchy specificity + semantic
  relatedness (the mechanism this ranking implements).
