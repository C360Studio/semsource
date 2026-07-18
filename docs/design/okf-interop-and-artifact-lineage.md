# OKF Interoperability and Artifact Lineage

> **Status:** Roadmap direction; implementation pending governed OpenSpec slices
> **Date:** 2026-07-14
> **Implementation commitment:** Independently governed materialized-view, OKF, and optional workbench slices
> **Scope:** OKF ingestion and publication, artifact lineage, OpenSpec coexistence, brownfield
> reconciliation, and bounded materialized context views

## Summary

[Open Knowledge Format (OKF) v0.1][okf-spec] is a small, portable Markdown format for linked project
knowledge. It could give SemSource a useful interoperability surface: consume project knowledge from
repositories that already use OKF, and publish bounded text views for people and systems that do not
query a SemStreams knowledge graph.

OKF is not a replacement for the source knowledge graph. Authored OKF may contribute explanatory
knowledge to the graph. SemSource-generated OKF is a derived serialization of graph-native context
views and must not return to its originating evidence set as new knowledge.

This distinction makes the proposal broader than an OKF vocabulary package. A repository can contain
Git history, implementation sources, OpenSpec contracts, authored OKF, and generated OKF at the same
time. SemSource needs artifact lineage and exclusive semantic routing before it can interpret those
sources without duplication or feedback loops.

This document records the durable interop boundary and experiment gates behind the roadmap direction.
Implementation mechanics belong in bounded OpenSpec changes; this document is not an architecture
decision record or evidence that the capability has shipped.

## Motivation

Agents can already ask natural-language questions through SemSource's graph-backed query surfaces and
receive compact contextual answers. An OKF collection would materialize a selected set of those
answers as human-readable Markdown. The value is not duplicating every graph node; it is making a
bounded set of useful project views portable.

Brownfield repositories add a second use case. Existing OKF concepts can contain explanations,
operational context, and project terminology that are absent from code. Because MCP and GraphQL access
the graph rather than arbitrary repository files, those authored concepts are useful source knowledge
when they retain their provenance.

The representative dogfood case is SemStreams:

- Git and AST sources describe observed implementation.
- OpenSpec current specs describe current contracts.
- OpenSpec active changes describe proposed target state.
- Brownfield OKF, if present, describes explanatory project knowledge.
- SemSource may publish a bounded OKF projection for consumers that do not run SemSource.

This combination is useful only if a generated publication cannot become fresh evidence merely
because it was committed to the same repository.

## Product boundary

The boundary follows `openspec/config.yaml`.

SemSource owns:

- source and artifact discovery;
- semantic routing and parsing;
- deterministic entity-ID construction;
- source provenance and artifact lineage;
- source-graph publishing;
- candidate view selection and OKF rendering; and
- brownfield reconciliation of inputs and SemSource-owned outputs.

SemStreams owns:

- governed entity ingestion;
- graph storage, query, and indexing;
- `ENTITY_STATES`, graph mutation, and graph query contracts; and
- generic graph lifecycle or materialization primitives.

SemSource must not implement a parallel graph substrate to support OKF. Any framework-shaped gap is
recorded in `docs/upstream/semstreams-asks.md` and raised with the SemStreams team.

Graph contributions remain subject to SemSource's existing non-negotiables: deterministic six-part
entity IDs, a semantic envelope on every write, and retention-first graph lifecycle. Historical
source evidence is retained and related rather than reference-blindly evicted, as established by
[ADR-0008](../adr/0008-versioned-source-retention-supersession.md).

## Terminology

| Term | Meaning |
|---|---|
| Authored artifact | Project knowledge maintained as an input by a person or upstream system |
| Derived artifact | Content generated from other evidence |
| Materialized context view | A bounded answer produced by a registered view definition over the graph |
| Self projection | Output produced from the current SemSource graph |
| Foreign projection | Generated OKF imported from another producer or graph |
| Managed root | A filesystem scope that SemSource is explicitly allowed to write |
| Ownership inventory | Observed paths, fingerprints, provenance, and write authority |
| View registry | Desired materialized views and their generation policy |

## Authority lanes

The inputs have different epistemic roles. They do not form one global precedence order.

| Source | Role in the graph |
|---|---|
| Git, AST, and config | Observed implementation |
| OpenSpec current specs | Normative current contract |
| OpenSpec active changes | Proposed target state |
| Authored OKF | Explanatory project knowledge |
| Self-generated OKF | Derived publication only |
| Foreign-generated OKF | External derived knowledge |
| Generic Markdown | Unstructured documentary evidence |

When the lanes disagree, the graph should preserve the disagreement with provenance. For example,
"OpenSpec requires X," "code implements Y," and "OKF explains Z" are three useful assertions. A
generic winner rule would discard information and misrepresent the source material.

Normative authority is also scoped. An OpenSpec change can be authoritative about a proposal without
describing current implementation. Authored OKF can be authoritative about project terminology while
remaining non-normative about runtime behavior.

## Reference architecture

```text
configured repository artifacts
        |
        v
discovery + exclusive semantic routing
        |
        +-- Git/AST ----------> observed evidence
        +-- OpenSpec ---------> normative/proposed evidence
        +-- authored OKF -----> explanatory evidence
        +-- foreign OKF ------> derived external evidence
        +-- managed output ---> publication inventory only
                                   |
                                   v
                            governed SemStreams graph
                                   |
                                   v
                     registered materialized context views
                                   |
                                   v
                              OKF renderer
                                   |
                                   v
                        managed repository output
```

The canonical derived abstraction is a materialized context view, not an OKF page. OKF is one
serialization of that view. Other renderers or direct MCP retrieval can reuse the same graph-native
artifact without parsing the published Markdown.

## Exclusive artifact routing

Each artifact body must have exactly one semantic interpreter. Without this rule, an OpenSpec file
could be interpreted as OpenSpec, generic Markdown, and repository content, creating overlapping
entities with unclear authority.

Git lineage is complementary rather than a second semantic interpretation. Git may record which
commit changed an OKF or OpenSpec path while the format-specific codec alone interprets the body.

Routing follows these principles:

1. Explicit source configuration wins over heuristic detection.
2. OpenSpec paths route to an eventual OpenSpec codec.
3. Configured authored OKF roots route to an OKF codec.
4. SemSource-managed output routes to publication tracking only.
5. Remaining Markdown routes to the generic document codec.
6. Code and configuration continue to use their specialized handlers.

Routing must occur before entity production. Deduplicating overlapping entities after ingestion would
be too late because provenance and identity would already be ambiguous.

## Brownfield discovery and reconciliation

Brownfield support is the default design case rather than a migration exception.

### Modes

| Mode | Behavior |
|---|---|
| `consume` | Ingest existing OKF and never write repository files |
| `augment` | Preserve existing OKF and publish bounded gap-filling views; recommended default |
| `manage` | Update only artifacts that SemSource can prove it owns; explicit opt-in |

### Initialization

Initialization is the first reconciliation:

1. Discover explicitly configured OKF roots, using format markers only as supporting evidence.
2. Inventory concept paths, links, metadata, fingerprints, and producer markers.
3. Treat every pre-existing unmarked page as externally owned for write protection.
4. Classify semantic authority from available provenance without inventing certainty.
5. Ingest useful authored and foreign-derived concepts into the graph.
6. Compare existing subject coverage with the bounded view registry.
7. Publish only missing views within a configured managed root and generation budget.

An ownership inventory is a reconciliation record, not a second source of project truth. It records
what SemSource observed, what it owns, what may be rewritten, and which source revision produced an
output. The view registry separately records which materialized questions should exist.

### Periodic reconciliation

Repository events can trigger a fast reconciliation, but a periodic full inventory check is still
needed because filesystem events may be missed and remote repositories are commonly polled by
revision.

| Observed change | Reconciliation behavior |
|---|---|
| Upstream concept added | Ingest it and suppress an overlapping generated view |
| Upstream concept modified | Reingest it and refresh dependent views without rewriting it |
| Upstream concept renamed or deleted | Reflect the current absence while retaining historical evidence |
| Managed output becomes stale | Refresh or retire only the SemSource-owned publication |
| Managed output edited externally | Record ownership drift and stop overwriting it |
| Path collision | Preserve the external path; suppress or relocate generated output |
| Same revision and fingerprints | No-op |

OKF concept identity is path-based, so an unannotated rename is normally a removal plus an addition.
A future stable-resource convention may support stronger correspondence, but the reconciler must not
infer identity from prose similarity by default.

The budget controls SemSource's additions, not the size of a pre-existing collection. If a repository
already contains hundreds of concepts, `augment` preserves them and may generate no new pages. Total
size remains an observable planning signal while `max_generated_concepts` and generated-byte limits
remain enforceable controls.

## OKF knowledge in the graph

Authored OKF can contain useful knowledge that code-oriented ingestion will not discover. An OKF
codec should therefore project at least:

- concept identity and bundle-relative path;
- declared OKF type and resource metadata;
- authored text or a body reference;
- source repository and revision provenance; and
- outgoing OKF links.

OKF links are deliberately untyped. They may be represented as a generic `linksTo` relation, but
SemSource must not infer domain predicates from wiki topology alone.

The original body and unknown extension metadata must remain recoverable. OKF v0.1 permits additional
frontmatter and expects round-tripping consumers to preserve unknown fields. Preservation is required
whenever SemSource is allowed to rewrite an owned artifact.

Current and historical source states follow the retention-first model from ADR-0008. A deletion or
rename changes what is current; it does not justify reference-blind eviction of historical graph
evidence.

## Materialized context views

A materialized context view is a named, bounded graph query or graph-assisted synthesis whose result
is useful enough to maintain. It prevents OKF publication from becoming an unbounded dump of entities
or every question an agent has asked.

A view should carry enough lineage to reproduce and evaluate it:

- stable view ID and view definition;
- evidence entity and body references;
- source revision and graph watermark;
- generator and model identity when synthesis is used;
- evidence hash;
- validation result and confidence; and
- publication path and content fingerprint.

Deterministic selection and validation should bound the candidate universe. SemInstruct may propose
or synthesize views, but a model does not grant itself authority. Generated prose remains derived even
when it is accurate.

The graph may expose materialized context views directly to MCP and GraphQL consumers. Rendering them
to OKF is useful for humans and external consumers, but is not required for graph-native use.

## Workbench relationship

The optional SemSource workbench is the live inspection and control surface for materialized context
views. It may display view freshness, evidence, authority, validation, and export preflight, but it does
not own the view definition, OKF codec, or publication policy. Those remain SemSource backend contracts
available to headless HTTP, MCP, or CLI consumers.

The workbench uses the reusable shell and canonical graph visualization maintained by `semstreams-ui`.
SemSource remains headless by default when embedded in another sem* product. A standalone operator
opts into the optional SemSource workbench through the `ui` profile; no UI dependency is required for
source ingestion, graph query, or agent access.

An OKF preview in the workbench renders the same bounded bundle returned by the backend exporter. A
future self-contained HTML viewer may render that bundle offline, but it remains presentation rather
than another graph, planner, or authority lane.

## Self and foreign projections

### Self projections

Self-generated OKF is an output serialization. SemSource may track its path, fingerprint, view ID,
producer, evidence hash, and graph watermark, but the content must not enter the originating planner's
evidence set.

Managed output roots and producer metadata provide two independent loop guards. An output-only commit
may still be visible in Git lineage, but it must not advance the semantic source watermark or trigger
regeneration from the generated prose.

If a person edits a managed page, SemSource records drift and relinquishes write authority until the
conflict is resolved. The edit does not silently promote generated prose into authored evidence.
Promotion requires an explicit ownership change, such as moving the content into an authored root.

### Foreign projections

OKF produced by another SemSource deployment is part of the interoperability use case. When producer
metadata is present, its concepts may enter the graph as external derived knowledge with their origin
and evidence watermark intact.

Unmarked brownfield content is preserved as externally owned. Its authorship may remain unknown; write
protection does not require pretending that unknown material is normative or human-authored.

Provenance must never be stripped in order to make a foreign projection look like primary evidence.

## SemStreams dogfood topology

The SemStreams repository is a useful experiment because it can contain all authority lanes at once.
SemSource would ingest the repository's code and OpenSpec artifacts, consume any authored OKF, and
optionally publish a bounded OKF bundle into an explicitly configured managed subtree.

The generated bundle can then serve people and tools that do not query SemSource. Committing it to the
repository is compatible with interoperability only if:

- the managed root is distinct from authored roots;
- generated concepts contain portable producer and lineage metadata;
- source routing recognizes those files before generic document ingestion;
- output-only changes do not trigger semantic replanning; and
- another deployment can recognize the files as derived external knowledge.

The experiment should choose the output directory rather than this design document. Directory layout
is a reversible implementation detail until it is tested against normal repository workflows.

## Safety invariants

- `consume` and `augment` never alter externally owned OKF bytes.
- Every artifact body has one semantic interpreter.
- Every graph contribution retains source and revision provenance.
- Self-generated content never becomes evidence for its own view planner.
- A generated path never overwrites an externally owned path.
- Equal source revision and evidence hash produce a no-op reconciliation.
- Page and byte budgets bound SemSource-generated growth.
- Code, OpenSpec, and OKF disagreements remain independently queryable.
- Generated claims remain visibly derived across publication and reingestion.
- Graph writes use deterministic six-part IDs and semantic envelopes.
- Graph lifecycle remains retention-first; publication cleanup is not graph eviction.

## Experiment plan

The experiment should evaluate four fixtures:

1. A greenfield repository with no OKF.
2. A partial brownfield bundle with a small set of authored concepts.
3. A foreign-generated bundle with producer metadata.
4. The SemStreams dogfood repository with Git, OpenSpec, and managed OKF output.

Mutation scenarios must include upstream edits, additions, renames, deletions, unknown frontmatter,
path collisions, managed-output drift, malformed concepts, and a generated write observed by the
repository watcher.

### Graduation gates

OKF codec, reconciliation, and publication implementation planning begins only if the experiment
demonstrates:

- byte-identical preservation of externally owned concepts;
- preservation of unknown frontmatter;
- retrieval of authored OKF knowledge through MCP with provenance;
- exclusion of self-generated content from planning evidence;
- derived classification of foreign-generated concepts;
- safe collision, drift, rename, and deletion behavior;
- no watcher or commit feedback loop;
- byte-identical and query-idempotent no-op reconciliation;
- stable view identities for a stable source graph;
- enforcement of generated page and byte ceilings; and
- independent visibility of conflicting authority lanes.

Successful evidence would justify proposing `add-okf-interop-mvp` after
`materialize-project-views` defines the bounded view contract. A durable OKF authority contract, if one
emerges, could then be distilled into an ADR. Neither the OKF implementation change nor an OKF-specific
ADR should be created before the experiment settles that boundary. This gate does not block the
independent opt-in workbench planning change.

## Non-goals

- Replacing the SemStreams knowledge graph with OKF.
- Treating OKF link topology as typed domain semantics.
- Replacing OpenSpec or implementing its future codec in this effort.
- Backfilling every graph entity or query as a Markdown page.
- Making generated prose normative.
- Requiring default human approval of generated manifests.
- Implementing SemStreams graph substrate inside SemSource.
- Committing the capability to SemSource v1.
- Selecting a final vocabulary package or output directory before experimentation.
- Automatically resolving disagreements among code, OpenSpec, and OKF.

## Open questions

1. Does SemSource need a generic artifact-lineage vocabulary before an OKF-specific vocabulary?
2. Should materialized view bodies live in graph entities, ObjectStore-backed text bodies, or both?
3. Which producer metadata is portable enough to distinguish foreign projections reliably?
4. When, and through what explicit operation, may a drifted managed page become authored content?
5. How should path renames relate concept revisions when OKF path is the default concept identity?
6. Should future OpenSpec entities reuse the generic artifact-lineage model or define a dedicated one?
7. Which view-registry decisions must be deterministic, and which may be SemInstruct-assisted?
8. Can the existing graph query and lifecycle APIs represent derived view lineage without an upstream
   SemStreams change?
9. Should generated views share one bundle with authored concepts or publish as a linked overlay?
10. How should consumers express trust policy for foreign-derived knowledge?

[okf-spec]: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
