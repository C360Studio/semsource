# How SemSource turns text into something searchable

_Status: descriptive, verified against code on 2026-11-06 (main at 4093d3c, SemStreams
v1.0.0-beta.160). This page explains the algorithms; the normative truth lives in the specs and
ADRs it links. When they disagree, the spec wins and this page is stale._

## The one-paragraph version

SemSource does feed an unstructured text index. It does not own one. Every source handler emits
entities whose body text the substrate embeds and indexes; prose gets there through the doc source,
which splits documents into passage entities by structure before size. The substrate (SemStreams
today, SemEngine after the port) turns each body into a vector and ranks queries by cosine
similarity. At tier 0 that vector is a feature-hashed BM25 bag of words with no external service;
at tier 1 it is a dense embedding from semembed. There is no inverted index anywhere in the stack,
so exact-phrase, boolean, regex, and wildcard queries over text do not exist. SemSource controls
what text becomes the vector. The substrate controls how it is indexed and ranked.

## Where the boundary sits

| Concern | Owner | Where |
| --- | --- | --- |
| Which files are sources, how they are watched | SemSource | `handler/*`, `processor/*-source` |
| How a document is split into passages | SemSource | `handler/doc/splitter.go`, spec `doc-passage-chunking` |
| What text and title each entity carries | SemSource | handler entity builders, spec `retrieval-ranking` |
| Salience nudges (tests below code, archives below canonical docs) | SemSource via governed predicates | spec `retrieval-ranking` |
| Which tier runs, and which model endpoint | SemSource config | `graph.embedder_type`, `model_registry`, ADR-0002 |
| Tokenizing, vectorizing, truncating body text | Substrate | `graph/embedding` (BM25 or HTTP embedder) |
| Storing vectors, cosine ranking, dedup | Substrate | `EMBEDDING_INDEX`, `EMBEDDING_DEDUP` KV buckets |
| Structural indexes (name, predicate, prefix, incoming) | Substrate | `graph-index` |
| Fusing structural and semantic results into agent context | SemSource | fusion gateway, ADR-0004, spec `fusion-gateway` |

The product boundary in `openspec/config.yaml` says it in one line: SemSource owns source
ingestion, not the substrate. A new kind of text index is substrate work.

## Ingestion: from a file to indexed bodies

### Code (ast-source)

Tree-sitter style parsers in `source/ast/` emit one entity per symbol for Go, TypeScript,
JavaScript, Java, Python, Svelte, and C-family headers. Each entity carries a deterministic six-part
ID, its signature and doc comment as body text, and typed edges (calls, defines, imports, belongs).
Edges are what make the graph a graph; the body text is what the embedder sees. Test and generated
symbols carry a presence marker that the ranking layer demotes.

### Prose (doc-source)

Markdown, plain text, and AsciiDoc are split by `handler/doc/splitter.go` into passages. The
algorithm, in order:

1. **Structure first.** Split on headings recognized in the document's own markup. Each passage
   remembers its immediate heading and the full ancestor chain.
2. **Merge the small.** Consecutive sections under the floor share a passage, so a run of one-line
   headings does not mint a passage each.
3. **Subdivide the large.** A section over the ceiling is split on paragraph boundaries, then on
   sentence boundaries.
4. **Fenced blocks are never cut.** A code block stays whole even above the ceiling. Two
   size-independent rules apply: a homogeneous `KEY=VALUE` list is split by key group at any size,
   because it dilutes its own embedding, and any other fenced block surrounded by substantial prose
   is isolated as its own passage, so a command sample does not compete with the paragraphs around
   it.
5. **Hard max.** No passage exceeds the substrate's embedding truncation limit, so nothing is
   silently unindexed past a cut.

Shipped bounds are a ceiling of 2000 bytes, a floor of 400, and a hard max of 6000. They were
chosen by a three-run live A/B over the graded question set, not guessed, and a four-times range
sweep changed nothing. Passages tile the document with no gap and no overlap; concatenating them
in ordinal order reproduces the input byte for byte.

Each passage becomes its own entity with a content-addressed body handle, ordinal, section heading,
and a `code.structure.belongs` edge back to the parent file. The parent stays as a body-less
navigational node. Passage titles qualify the parent title with the heading chain, because the title
is part of the text the embedding ranks by, and the ancestry measurably separates two same-named
sections. Passages left behind when a document shrinks are marked stale, not deleted.

### Everything else

Git commits, config manifests (go.mod, package.json, Dockerfile, POM, Gradle), URL fetches, and
media metadata each emit entities with short bodies. They are indexed the same way; they are rarely
what a natural-language query is looking for, and the default docs corpus excludes `openspec/**`
and `node_modules` so planning artifacts do not outrank canonical documentation.

## Indexing: what the substrate does with a body

The graph-embedding component watches entity state, reads each entity's body text (inline or
through its storage reference), truncates it at the substrate's limit, and asks an embedder for a
vector. The vector is written to `EMBEDDING_INDEX` keyed by entity ID, and a content hash goes to
`EMBEDDING_DEDUP` so unchanged text is never re-embedded. The `EMBEDDINGS_CACHE` bucket exists but
is never written; an empty one is healthy.

### Tier 0: BM25 as a vector

`graph.embedder_type: "bm25"` is the default and needs no external service. It is not an inverted
index. The embedder:

1. Lowercases and splits on non-alphanumeric characters, drops tokens under two characters,
   removes English stopwords, and stems.
2. Computes term frequencies per document.
3. Hashes each term into a fixed 384-dimension vector (feature hashing).
4. Weights each slot with BM25 (term frequency saturation at k1 1.5, length normalization at
   b 0.75, IDF from running corpus statistics).
5. L2-normalizes so the result is cosine-comparable with everything else in the index.

The consequence: tier 0 is good when the query shares words with the code or prose, and weak on
paraphrase. "Where do we retry with backoff" will not find a `withRetry` helper unless the words
line up.

A known limitation lives upstream as semstreams#619: the corpus statistics are process-local,
unpersisted, and order-dependent, so a restart forks the vector space. SemEngine carries the same
embedder and the same open decision. The query side has already been made read-only so searches no
longer mutate the corpus.

### Tier 1: semembed

`graph.embedder_type: "http"` routes the `embedding` capability through the model registry to
semembed (an OpenAI-compatible endpoint serving arctic-embed-s). Vectors are dense and
paraphrase-robust. The query prefix must be set; with it, tier 1 beats tier 0 on every paraphrase
case the scorecard holds. This is the shipped default in `configs/mvp.json` and the Docker Compose
stack.

### Tier 2: clustering and summaries

Tier 2 adds graph-clustering with LLM community summaries through seminstruct. It changes the
community and summary routes, not text retrieval. No MCP tool requires a generative model; tiers 0
and 1 answer with no LLM in the loop. See ADR-0002.

## Retrieval: how a query becomes an answer

1. **Classify.** The fusion gateway (ADR-0004) is deterministic: it maps a tool call to a lens with
   a default scope, with no LLM. Structural tools (`code_context`, `code_impact`, callers, byName,
   prefix) read the graph indexes directly and work at every tier, before embeddings are ready.
2. **Embed the query.** For `code_search`, `doc_context`, and `graph_search`, the query text goes
   through the same embedder as the documents (tier 0 symmetric, tier 1 with the query prefix).
3. **Rank by cosine.** The substrate returns the nearest entities. Ranking is essentially the
   embedding's own cosine order; governed salience is deliberately weighted small and acts as a
   tie-breaker. That is why SemSource's lever is the body text it emits, not a ranker.
4. **Nudge.** Production symbols outrank their tests through a demotion marker on the test
   presence predicate, not filtering. Canonical docs outrank archived planning artifacts through
   corpus scoping. Body-less parents never displace passages with content.
5. **Fuse and bound.** The gateway hydrates the winners with structural context, applies the
   per-tool byte budget, and renders. `graph_search` renders bounded value properties; the
   full-entity shape is never the default because of size and ordering.

Readiness gates all of this: consumers poll `graph.query.status` for `index.ready` before
structural queries and `embedding.ready` before semantic ones. Entity counts are not a readiness
proxy.

## What this stack cannot do

- **Exact phrase, boolean, regex, wildcard.** Both tiers are vector similarity. There is no posting
  list, no positions, no term-level query language.
- **Aggregations over text.** No facets, counts, or histograms over passages.
- **Log and event volumes.** Measured corpora are 21.6k entities (Go dogfood) and 32k (Open Sensor
  Hub Java). The design is agent-driven QPS over a living graph, not petabytes of append-only
  records. The Quickwit comparison in the team workspace covers that boundary.

The loose-phrasing band of the scorecard (issue #170, seven paired questions) ran keyword-only and
scored 33 of 33, so none of these gaps has cost a measured question yet. The gaps are real; the
pressure is not.

## If the team wants a true lexical index

It belongs to the substrate, which after the port means SemEngine. The relevant work already open
there:

- **semengine#10** (Slice 04B): tier-0 lexical retrieval, BM25 "completes tier 0", with
  known-answer ranking, scope-before-limit, stale and update semantics, rebuild, and restart. This
  is where the semstreams#619 decision lands: an explicit lexical index over an immutable
  revisioned snapshot, or a stateless hashed vector described honestly.
- **semengine#506**: a keyword index per entity-ID prefix in graph-index, so free-text search
  over a large community is not cut short at one page. Filed as tier-0 follow-up; a metric on
  cut-short searches decides when it is due.
- **semengine#11** (Slice 04C): tier-1 neural retrieval. Named as SemSource's default workload;
  SemSource's mainline leaves SemStreams only after it passes.

An ask for phrase and boolean query semantics should be filed against semengine#10 or #506 as a
requirement with a scorecard question that needs it, not as a new SemSource processor. The
ingestion side already feeds every body a lexical index would need. What SemSource would owe is
the question that proves the gap.

## Related

- Spec: `openspec/specs/doc-passage-chunking/spec.md`
- Spec: `openspec/specs/retrieval-ranking/spec.md`
- Spec: `openspec/specs/fusion-gateway/spec.md`
- ADR: `docs/adr/0002-tier-support-and-model-registry.md`
- ADR: `docs/adr/0004-deterministic-fusion-gateway.md`
- Tier configs: `configs/tiers/README.md`
- Upstream ledger: `docs/upstream/semstreams-asks.md`
- Scorecard: `scripts/scorecard/`
