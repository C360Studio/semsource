## Context

Doc ingestion emits one entity per file carrying the whole body
(`handler/doc/entities.go:175-194`), and the substrate embeds one vector per entity from text
hard-truncated at 8000 characters. Everything past that is silently unindexed. The fix is
producer-side by written framework contract (`pkg/fusion/hydrate.go:71-79`: handles address a
pre-sliced body, the engine does no line math), and the working precedent already exists in this
repo for code (`processor/ast-source/bodystore.go:70-112` slices per symbol).

Four substrate facts were verified against semstreams `v1.0.0-beta.153` before designing, because
each one eliminates an option that would otherwise look reasonable:

1. **Embedding dedup is keyed by content hash, and the dedup bucket has no TTL.**
   `graph/embedding/storage.go:271` (`GetByContentHash`), `worker.go:371-386` (dedup checked
   *before* the embedder; hit returns the stored vector, `Generate` never called),
   `component.go:729-748` (buckets created with no TTL — vectors are permanent). Re-pointing an
   entity at content that was embedded before costs one KV get plus one KV put and **zero model
   calls**. Caveat: the *pending* record is keyed by entity ID (`storage.go:147,176`), so N
   entities on one blob still cost N worker passes — N × (KV write + KV read + body fetch), just
   not N × inference.
2. **The staleness lifecycle pass is path-based and groups by path.**
   `processor/supersession/lifecycle.go:161` groups entities by their `DocFilePath`/`CodePath`
   predicate, then decides mark/clear per *distinct path* via `os.Stat` (:166-176). Entities with
   no path predicate are skipped outright (:158-160).
3. **Re-publishing an entity replaces per (subject, predicate); it cannot delete an entity.**
   `graph/helpers.go:98-141` (`MergeTriples`), used by the ingest lane at
   `graph-ingest/component.go:2469`. `RemoveTriples` deletes predicates and runs before the merge
   (`graph/mutation_requests.go:101`). There is no notion of "the producer's current set" — an
   entity you stop publishing is simply never touched again.
4. **`StorageRef` can be swapped but not cleared.** `graph/mutation_requests.go:82-88` and
   `graph-ingest/component.go:2470-2472` (`if entity.StorageRef != nil { … }`) — a nil ref
   preserves the stored one. Upstream gh#260.
5. **The predicates and the lens hook for this already exist and are dead.**
   `source.CodeBelongs` (`source/vocabulary/predicates.go:233`) is registered and documented
   verbatim as "Used for document chunks → parent document relationships."
   `DocChunkIndex`/`DocChunkCount`/`DocSection` are registered (`predicates.go:44-50,332-345`) and
   never emitted. `source/fusion/lens/docs/docs.go:60` returns `nil` edges with a comment reserving
   the slot. The design below fills reserved slots rather than inventing vocabulary.

Fact 2 is the one that reshapes the change: **a document that shrinks from 10 chunks to 7 leaves
chunks 8, 9 and 10 live in the graph forever.** All ten carry the same `DocFilePath`, that file
still exists on disk, so the shipped staleness pass marks none of them. Chunking without a
retraction story is a corpus that silently accumulates deleted prose and keeps serving it as
current — the exact "phantoms indistinguishable from facts" failure `staleness-and-retraction` just
closed for files. Retraction is therefore in scope here, not a follow-up.

## Goals / Non-Goals

**Goals:**

- A paragraph-level question matches a paragraph, not an averaged whole document.
- No document content is silently unindexed, at any file size.
- Chunk identity is deterministic, total (every chunk gets one), and collision-free.
- A document that shrinks or is restructured leaves no orphaned passages serving as current.
- A passage hit can reach its parent document, and a document can reach its passages.
- Reserved-but-dead vocabulary is either emitted or deleted — nothing stays half-wired.

**Non-Goals:**

- Code chunking (already solved per symbol), semantic/LLM splitting, answer-side windowing in the
  framework, retrieval-budget redesign, chunk-level supersession lineage, and media/URL/config
  chunking. See proposal Non-goals.
- Sibling-to-sibling expansion in a single engine hop (see D5).

## Decisions

### D1 — Chunk identity is the parent path slug plus an ordinal

`{org}.semsource.web.{system}.chunk.{path-slug}-{n}`, `n` zero-padded, built through
`entityid.Build` like every other ID.

*Why:* it is the only scheme that is **total and collision-free**. Every chunk gets an ID whether or
not it sits under a heading, and two identical headings in one file cannot collide.

*Alternatives rejected:*

- **Heading-slug IDs** (`…chunk.{path-slug}-{heading-slug}`) survive insertion without renumbering,
  which is genuinely attractive. Rejected because they are not total (prose before the first
  heading, and any file without headings, has no slug), they collide on repeated headings, and a
  heading *rename* mints a new ID while orphaning the old one. That last failure is precisely the
  content-derived-identity problem that `handler/doc/entities.go:46-56` deliberately moved doc IDs
  *away from* — reintroducing it one level down would be a regression against a just-shipped
  invariant.
- **Content-hash IDs** — same objection, worse: every edit orphans.

*The cost we are accepting:* inserting a section at the top of a document shifts every subsequent
passage's content by one ordinal. Fact 1 is what makes this affordable — the shifted content was
already embedded, so re-association is KV traffic and no inference. Renumbering also never orphans,
because IDs are derived from (path, index) and are reused in place under per-predicate replace
(fact 3). Insertion churn is bounded and correct; heading-rename orphans would be unbounded and
wrong.

### D2 — The parent stays, loses its body, and carries the chunk count

The file-level entity keeps its ID, `DcTitle`, `DocFilePath`, `DocMimeType`, `DocFileHash`, and
provenance. It **no longer carries** `DocBodyStore`/`DocBodyKey` or a `StorageRef`. It **gains**
`DocChunkCount` (registered, currently never emitted).

*Why keep a parent at all:* it is the stable navigational node every other surface already points
at — the lifecycle pass's path grouping (fact 2), source-manifest counts, and doc identity across
edits. Deleting it would break all three.

*Why strip its body:* if the parent keeps a whole-file body it keeps a whole-file vector, so the
diluted averaged embedding this change exists to eliminate stays in the corpus and competes with
the passages. Two entities would also return the same prose as separate evidence.

*The migration consequence, stated plainly:* fact 4 means a nil `StorageRef` on re-publish
**preserves** the stored one. Existing deployments cannot have parent bodies cleared in place by
reindexing — the stale whole-file ref would survive. Migration is therefore a rebuild from an empty
graph, not an in-place reindex. This is the same call `staleness-and-retraction` made for doc
identity on the same reasoning (pre-1.0, small install base). Upstream gh#260 is referenced, not
waited on.

### D3 — Chunk-count-aware retraction, fast path plus backstop

The tail-chunk gap (fact 2) is closed in two places, mirroring the architecture
`staleness-and-retraction` already established (fsnotify fast path + async graph-derived pass):

- **At ingest (fast path):** when a document is re-ingested with fewer chunks than before, the
  producer marks ordinals `[new_count, old_count)` with `entity.lifecycle.stale`. The prior count is
  the parent's current `DocChunkCount`, read from the graph — one read per *changed* document.
- **In the lifecycle pass (backstop, restart-proof):** `decideLifecycleActions` gains chunk
  awareness. For a path that **is** present on disk, any chunk entity whose `DocChunkIndex` is `>=`
  its parent's `DocChunkCount` is stale. This is graph-derived and needs no filesystem access beyond
  what the pass already does.

*Why both:* the fast path gives immediate correctness on the common edit; the pass survives a crash
between publishing the parent and marking the tail. This is the shipped pattern, not a new one.

*Why mark rather than delete:* retention-first is a non-negotiable, and `entity.lifecycle.stale` is
already the governed answer with `WithWeight(-3.0)`. Marking also uses the existing append lane and
its caller-side pre-filter (`lifecycle.go:224-229`); un-marking on regrowth uses `RemoveTriples`
(`lifecycle.go:255-265`), which is a silent no-op on unknown predicates and therefore safe to call
unconditionally.

*Consequence to accept:* a document that shrinks permanently and never regrows keeps demoted chunk
entities forever. That is the same unbounded-accumulation trade-off already accepted in
`staleness-and-retraction`, pending upstream #15/#433.

### D4 — Structural splitting, heading-anchored, size-bounded, pure

The splitter is a pure function of file bytes — same input, same chunks, always. Order:

1. Split on Markdown ATX/setext headings; the heading and its prose form one chunk.
2. A section over the size ceiling splits again on blank-line paragraph boundaries, never
   mid-paragraph. A single paragraph over the ceiling is split on sentence boundaries, then hard-cut
   as a last resort so no chunk can exceed the ceiling.
3. Sections under a floor merge forward into the next sibling, so a run of one-line headings does
   not mint a chunk each.
4. Content before the first heading is a chunk (this is why identity cannot be heading-derived).
5. Fenced code blocks are never split across chunks.

The ceiling sits well under the substrate's 8000-character truncation so no chunk is ever
truncated; leaving headroom is the point, since the cap is not configurable. Exact ceiling and floor
values are tuning, not architecture — see Open Questions.

*Splitter provenance:* `source/parser/` has zero importers repo-wide and the live handler uses its
own local `extractTitle` (`handler/doc/handler.go:197`). The splitter is written fresh against the
live path and the orphaned package is deleted rather than revived — reviving dead code to satisfy a
new contract costs more than it saves, and the package's `GenerateChunkID` hardcodes a
`c360.semspec.…` ID that violates our own identity rules anyway (`source/parser/markdown.go:129-132`).

### D5 — `Edges()` declares chunk containment on `FacetRelations` only

```go
{Predicate: source.CodeBelongs, OutgoingRole: "parent_document",
 IncomingRole: "chunk", Facets: []fusion.Facet{fusion.FacetRelations}}
```

*Why this predicate:* it exists, is registered, and its doc comment already names this exact use.

*Why relations-only:* the code lens restricts `CodeContains` the same way and states the reason at
`pkg/fusion/lens.go:115-117` — containment belongs in the relations map, not in the impact walk.
A chunk→parent edge inside the impact BFS would flood every doc-adjacent query with the parent's
entire chunk set. Paths is excluded for the same reason.

*Sibling expansion is one hop short, deliberately.* `collectEdges`
(`pkg/fusion/engine_lens.go:195-210`) walks exactly one hop, so a chunk seed yields its parent but
not its siblings; siblings need outgoing-then-incoming, which no built-in walk performs. We accept
one hop and let a caller re-seed on the returned parent handle. Emitting explicit sibling predicates
would mint O(n²) triples per document to save a round trip — a bad trade at any document size.

### D6 — A chunk is named for its section, qualified by its parent

`DcTitle` on a chunk is `{parent title} § {section heading}`, falling back to
`{parent title} § {ordinal}` for headingless content. Chunks also carry `DocSection` (registered,
never emitted) with the bare heading.

*Why it must be set at all:* `refFor` (`engine_lens.go:212-221`) builds every neighbour reference
from `Label()`, and `source-vocabulary-contract` requires that name-reachable entities carry
`dc.terms.title`. An untitled chunk degrades every relation listing that mentions it. The parent
qualifier is what stops a results list from showing six identical `## Usage` entries.

### D7 — `Locator.Fragment` carries the section anchor

`docs.go:75-76` already reserves `Fragment` "for a section anchor once sections are emitted." Chunks
set it to the heading's anchor slug, so citations deep-link to the section instead of the file. Free
given D4 computes the heading anyway.

### D8 — The dead-code cascade is deleted wholesale, not trimmed

A clean-break license applies: no deprecated paths, no back-compat shims, no reserved-but-unemitted
vocabulary. Deleting `source/parser/` (zero importers repo-wide) cascades further than expected,
because it is the **sole importer** of the root `source` package:

- `source/parser/` — 6 parsers plus tests, all dead.
- `source/types.go` (360 lines) and `source/requests.go` (112 lines) — every type has zero external
  references once `source/parser/` is gone. This includes the `Chunk` struct, which was never
  instantiated anywhere, and `DocumentSource.ChunkCount` / `WebSource.ChunkCount`.
- The dead half of `source/vocabulary/enums.go` — `DocCategoryType`, `DocSeverityType`,
  `DocScopeType`, `StatusType`, `TypeValue`, `DomainType`; `source/types.go` was their only consumer.
- ~25 registered-but-never-emitted predicates in `source/vocabulary/predicates.go` (`DocCategory`,
  `DocAppliesTo`, `DocSeverity`, `DocScope`, `DocDomain`, `DocRelatedDomains`, `DocKeywords`, and the
  `Web*` analogues including `WebChunkCount`, `WebSection`, `WebChunkIndex`), plus their assertions
  in `predicates_test.go` and `beta148_contract_test.go`.
- Four `explicitRoles` entries in `processor/source-manifest/status.go:299,301-304` that classify
  predicates nothing emits (`DocRequirements`, `WebSummary`, `WebRequirements`, `WebContent`).
- `source/vocabulary/iris.go` collapses to four live constants (`DcAbstract`, `DcFormat`, `DcType`,
  `MaNamespace`); every `Class*`/`Prop*` in it — including the chunk IRIs this change was expected to
  adopt — is referenced only from comments.
- `source/vocabulary/doc.go` is 103 lines of package comment describing the unimplemented
  "Parent-Chunk Model" with a `source.doc.{category}.{slug}.chunk.{n}` ID scheme that contradicts the
  real six-part scheme, and a usage example emitting four dead predicates. Rewritten from scratch.

*Consequence for this change's own vocabulary:* the chunk IRIs in `iris.go` are dead constants, not
a usable reservation. Passage containment therefore uses `source.CodeBelongs`
(`predicates.go:233`) — registered, live-adjacent, and already documented for exactly this purpose —
and the chunk predicates this change emits (`DocChunkIndex`, `DocChunkCount`, `DocSection`) are kept
from the dead list precisely because this change makes them live. Everything else on that list goes.

### D9 — The doc `RawEntity` ingestion path is deleted

`handler/doc/handler.go:110` `Ingest()` and `:159` `ingestFile()` have **zero production callers** —
only `handler_test.go` (12 sites) and `exclude_test.go`. The live path is
`processor/doc-source/component.go:213` → `IngestEntityStates` → `ingestFileEntityState`. Both are
deleted along with the tests that exist only to exercise them; exclusion behaviour is already covered
on the live path by `isDefaultExcludedDocDir` (`entities.go:144`).

`Ingest` is declared on the `handler.SourceHandler` interface (`handler/handler.go:70`), but that
interface **has no dispatcher** — `Supports` is called only from tests, `doc.Handler` is not even
asserted against it, and no registry of `[]SourceHandler` exists. Dropping `Ingest` from the
interface is the clean-break move but touches seven other handlers whose `Ingest` *is* live
(image/video/audio call their own internally). **Decision: delete doc's dead implementations now;
leave the interface alone in this change** and note the unenforced-contract finding for a separate
handler-surface change. Widening this one into a seven-handler refactor would bury the feature.

*Also deleted:* the ID-parity comments that exist solely to justify two paths agreeing
(`entities.go:48-50`, `handler.go:167-170`), and every "bypassing the normalizer" comment in the doc
path — **there is no `normalizer` package in this repository at all**; all ~30 mentions are comments
referring to something that no longer exists.

### D10 — The verbatim body store becomes mandatory

Today `processor/doc-source/component.go:157-164` logs `"verbatim body store unavailable; doc bodies
will not be offloaded"` and continues, falling back to inline `DocContent`
(`entities.go:88-93`). That is a silent-degradation path of exactly the class the audit closed
elsewhere: the deployment appears healthy while every document loses its body handle, and — per fact
1's StorageRef lane — its embedding.

**Decision: the body store is required. Startup fails loudly when it is unavailable**, and the inline
`DocContent` fallback is deleted along with `Entity.Content`'s post-offload role. NATS is already a
hard dependency and the object store rides on it, so a store that cannot be created signals a broken
deployment, not a degraded-but-usable one. This also removes a branch that chunking would otherwise
multiply — N inline chunk bodies per document in triples.

*This resolves the "do not delete" caveat in the code inventory:* the fallback is only load-bearing
while the store is optional, and this decision makes it not optional.

### D11 — `DocSummary` is deleted, not kept as a title duplicate

`entities.go:83-85` emits `DocSummary` carrying the title "for back-compat". Its only reader is an
unreachable fallback in the lens (`docs.go:69` — unreachable because `extractTitle` can never return
empty, falling back to the filename stem) plus a role-map entry. Emit, fallback, role entry, and the
three tests pinning them are deleted; `Label()` reduces to `DcTitle`.

### D12 — The parent contributes no content embedding

Resolving the design's own open question under the clean-break license: the parent document entity
MUST NOT contribute a content embedding at all — it carries no body handle, no `StorageRef`, and no
content-indexing profile. It remains name-reachable through `dc.terms.title`. Leaving it
content-indexed with nothing to index is how the empty-body config nodes became lexical noise in the
audit; a navigational node should be honestly typed as one.

## Risks / Trade-offs

- **Corpus node count grows several-fold** (211 Markdown files here would become on the order of a
  thousand-plus chunk entities) → graph-index write amplification is a known, already-tracked
  upstream scale concern. Mitigation: the floor-merge in D4 suppresses trivial chunks; measure node
  count and readiness time before and after on this repo, and treat a large regression as a blocker,
  not a footnote.
- **N chunks on one document cost N embedding worker passes** even when every vector is a dedup hit
  (fact 1's caveat: the pending record is entity-keyed). Mitigation: none needed for correctness;
  it is KV and store traffic, not inference. Watch first-seed wall-clock.
- **Insertion at the top of a document renumbers every following passage** → bounded churn, no
  inference cost, no orphans (D1). Accepted.
- **Parent and its own chunks could both surface as evidence for one query** → the `retrieval-ranking`
  delta must state that a result set does not return both; the body-less parent (D2) makes this
  mostly self-solving since the parent has no passage to offer.
- **A body-less parent is a titled node with nothing behind it** — the exact lexical-noise shape the
  audit found with empty-body config entities. Mitigation: this is a stated requirement in the
  `retrieval-ranking` delta, and it must be verified by graded re-run, not asserted.
- **Existing deployments cannot clear parent `StorageRef` in place** (fact 4 / gh#260) → migration is
  a graph rebuild, documented as such. Risk is limited by the pre-1.0 install base.
- **The splitter is a new correctness surface.** A bad split is silent — it degrades answers without
  erroring. Mitigation: golden-file tests over real repository documents, plus the determinism
  property (same bytes → same chunks) as an explicit test, plus the graded interrogation as the
  end-to-end check.

## Migration Plan

1. Ship behind no flag — chunking is the ingestion behaviour, not an option.
2. **Rebuild the graph from empty.** Fact 4 makes in-place reindex insufficient: parent entities
   would retain their whole-file `StorageRef` and keep emitting the diluted vector this change
   removes. Document that a reindex-in-place leaves a stale parent body.
3. Consumers (SemSpec, SemDragon, the workbench) read `doc_context` and the governed graph; neither
   resolves `DocBodyKey` from the file entity today, so no consumer change is required. The
   workbench must not assume one evidence node per file.
4. Rollback is a revert plus a rebuild; there is no on-disk state to undo.

## Open Questions

- **Chunk size ceiling and floor.** Architecture is settled; the numbers are not. These should be
  chosen empirically the way the tier-1 embedding arc was settled — an A/B over the graded
  interrogation, not a guess. Q13 and Q17 are the specific targets. This is the one genuinely open
  item.

**Resolved under the clean-break license** (previously open, now decided — see Decisions):

- Body-less parent excluded from content indexing entirely → **yes**, D12.
- `WebChunkCount` / `ClassWebChunk` and the rest of the unemitted vocabulary → **deleted**, D8.
- Inline `DocContent` fallback and optional body store → **removed; store is mandatory**, D10.
- `DocSummary` title duplicate → **deleted**, D11.
- Doc `RawEntity`/`ingestFile` path → **deleted**; `SourceHandler.Ingest` interface change
  deliberately deferred to a separate handler-surface change, D9.
