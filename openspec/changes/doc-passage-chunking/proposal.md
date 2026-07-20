## Why

Doc ingestion emits exactly one entity per file carrying the whole file body
(`handler/doc/entities.go:158,186`), and the substrate embeds one vector per entity from a body
hard-truncated at 8000 characters (`graph-embedding/component.go:786-790`,
`graph/embedding/worker.go:23`). The tail past 8000 characters is **silently dropped from the
semantic index** — on this repository alone that is 43 files and roughly 235 KB unindexed,
including about 74% of our own README. What survives is one averaged vector per file, so a
paragraph-level question matches a whole document or nothing; this is the measured cause of the
two remaining graded-interrogation residuals (Q13, Q17) on the 17/19 → 19/19 path.

Code does not have this problem because `processor/ast-source/bodystore.go:70-112` already slices
per symbol at ingest. Docs never shipped the equivalent, even though the vocabulary, IDs, and lens
hook for it were reserved years ago and left dead.

## What Changes

- **Doc ingestion emits passages.** `handler/doc/entities.go` splits each document on structure
  (headings, then size-bounded paragraph groups) and emits one chunk entity per passage in addition
  to the file-level parent entity.
- **Parent keeps identity, children carry bodies.** The parent doc entity retains its ID, title,
  path, hash, mime type, and provenance and no longer carries a body handle. Each chunk entity
  carries its own `StorageRef` and `doc:<sha-of-chunk>` blob, `chunkIndex`, and section heading, and
  is linked to its parent by `PropChunkOf` / `PropHasChunk`.
- **Chunk entities use a `chunk` type segment.** `{org}.semsource.web.{system}.chunk.{path-slug}-{n}`,
  built through `entityid.Build` like every other ID, deterministic and re-derivable from
  (path, index) alone.
- **The docs lens gains `Edges()`.** `source/fusion/lens/docs/docs.go` currently returns nil edges;
  it gains `PropChunkOf`/`PropHasChunk` so a passage hit can expand to its siblings and parent
  instead of returning an orphaned fragment.
- **BREAKING — doc body handles move.** `DocBodyStore`/`DocBodyKey` are emitted on chunk entities,
  not on the file entity. Any consumer resolving a doc body from the file-level entity must follow
  `hasChunk`. Migration is reindex, consistent with the doc-identity break already taken in
  `staleness-and-retraction`.
- **BREAKING — the verbatim body store becomes mandatory.** Today an unavailable store logs a
  warning and silently falls back to inline content, so every document loses its body handle and its
  embedding while the deployment looks healthy. Startup now fails loudly; the inline `DocContent`
  fallback is deleted.
- **BREAKING — `DocSummary` is deleted.** It carried a duplicate of the title "for back-compat" and
  its only reader is an unreachable lens fallback.
- **Dead code is deleted wholesale, under the clean-break license.** No deprecated paths, no
  back-compat shims, no reserved-but-unemitted vocabulary. `source/parser/` has zero importers and
  is the sole importer of the root `source` package, so its removal cascades to `source/types.go`
  (including the never-instantiated `Chunk` struct), `source/requests.go`, and the dead half of
  `source/vocabulary/enums.go`. Roughly 25 registered-but-never-emitted predicates go with them,
  along with the four `explicitRoles` entries classifying predicates nothing emits;
  `source/vocabulary/iris.go` collapses to four live constants. The doc `RawEntity` path
  (`Ingest`/`ingestFile`) has no production callers and is deleted.
- **False documentation is corrected.** `source/vocabulary/doc.go:35-43,70` documents a
  "Parent-Chunk Model" with `source.doc.{category}.{slug}.chunk.{n}` IDs that no code has ever
  produced and that contradicts the real six-part scheme. It is rewritten. Every "bypassing the
  normalizer" comment goes too — **no `normalizer` package exists in this repository**; all such
  comments reference something already gone.
- **Two framework gaps are filed upstream, not worked around.** See Impact.

## Capabilities

### New Capabilities

- `doc-passage-chunking`: How a document becomes retrievable passages — split determinism and
  stability, parent/child entity split, chunk identity, body-per-chunk storage, and what a
  re-ingest of an unchanged file must not churn.

### Modified Capabilities

- `fusion-gateway`: `doc_context` evidence becomes passage-scoped rather than whole-file. The docs
  lens gains chunk expansion edges. Today's requirement set assumes one node per document.
- `source-vocabulary-contract`: the chunk predicates move from registered-but-never-emitted to
  emitted vocabulary, and the requirement that queryable entities carry `dc.terms.title` must state
  how a chunk is named relative to its parent. (Its `TBD` Purpose is filled while we are in the
  file — an unfilled Purpose is the drift this workflow exists to prevent.)
- `retrieval-ranking`: the docs corpus gains N passage nodes per file and loses the file's body.
  Ranking must state that a body-less parent doc entity does not become lexical noise (the failure
  mode already observed with empty-body config title nodes) and that a result set does not return
  both a parent and its own chunks as separate evidence.

## Impact

**Code**

- `handler/doc/entities.go` — `ingestFileEntityState` emits parent + N children; `offloadDocBody`
  becomes per-chunk.
- `handler/doc/` — a splitter, either revived from `source/parser/markdown.go` or written fresh
  against the live path.
- `source/fusion/lens/docs/docs.go` — `Edges()` implementation.
- `source/vocabulary/doc.go`, `source/vocabulary/iris.go`, `source/vocabulary/predicates.go` —
  emitted chunk vocabulary, corrected prose.
- `source/parser/`, `source/types.go` — dead-code removal.
- `processor/doc-source/` — body-store wiring for multi-blob puts.

**Consumers**

SemSpec and SemDragon read doc bodies through `doc_context` and the governed graph; both see
passage-scoped evidence after this change and neither resolves `DocBodyKey` directly today. The
workbench renders `doc_context` evidence and must not assume one node per file.

**Upstream (filed, not blocking)**

Both are logged in `docs/upstream/semstreams-asks.md` and communicated as GitHub issues only:

1. **Offloaded entities never embed their title.** `graph-embedding/component.go:1150-1153` returns
   immediately on the `StorageRef` path, so `DcTitle` and `DocSummary` are excluded from the vector
   of every body-bearing entity. This is a recall bug independent of chunking and it also silently
   makes the `text_suffixes` config we set in `cmd/semsource/run.go:830-833` inert for those
   entities.
2. **The 8000-character embedding cap is not configurable.** Hardcoded at
   `graph-embedding/component.go:786-790`. Chunking keeps us under it, so this change does not
   depend on the ask, but any producer with legitimately large bodies is silently truncated.

**Not blocked on either.** `pkg/fusion/hydrate.go:71-79` states as written contract that a handle
addresses a pre-sliced body and that the engine performs no line math, and the embedding schema is
one vector per entity — so passage retrieval is the producer's responsibility by design. This is
product-shaped work under the Product Boundary, not substrate.

## Non-goals

- **No chunking for code.** `processor/ast-source/bodystore.go` already slices per symbol; this
  change does not touch code bodies or the AST path.
- **No semantic/LLM-derived splitting.** Structural splitting only (headings, paragraph groups,
  size bounds). Tier-2 summary-based passages remain a separate roadmap item.
- **No answer-side windowing in the framework.** We do not ask SemStreams to slice bodies; the
  hydrate contract explicitly forbids engine-side line math.
- **No retrieval-budget redesign.** `defaultMaxNodes`/`defaultMaxBytes` and the always-admit-one-node
  rule stay as they are; chunking makes them behave sanely without changing them.
- **No rename/move detection across chunk boundaries**, and no chunk-level supersession lineage.
  Version intelligence stays file-and-symbol-scoped.
- **No media, URL, or config chunking.** `WebChunkCount` is in scope only as dead-code disposition,
  not as a new web-source capability.
