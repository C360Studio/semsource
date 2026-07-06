## Context

The verified target is SemStreams `v1.0.0-beta.114` at peeled commit
`64fb300797bcfecbdf3b2fa3025927191cba4092`. SemSource currently pins
`v1.0.0-beta.59`.

A temp compile probe could not reach code compatibility because the local Go
toolchain is `go1.25.3` and SemStreams beta.114 declares `go 1.26.3`. That is
the first migration gate, not an incidental cleanup.

SemSource's current source path is mostly healthy for ADR-055: source processors
publish `message.BaseMessage` values carrying `semsource.entity.v1`, and the
payload implements `EntityID()` and `Triples()`. That means the fact-arrival
lane can still birth entities with an envelope.

The weak spot is runtime governance. Standalone SemSource manually constructs
graph-ingest, graph-index, graph-embedding, graph-query, and graph-gateway
component configs, but it does not create OWNER_CLAIMS / OWNER_PRESENCE or bind
SemSource projection contracts before graph-ingest starts. Current SemStreams
graph-ingest self-wires a claim reader from OWNER_CLAIMS on start; without the
bucket it gracefully skips foreign-edge classification and owner-lease checks.

SemOps pressure is specifically around binary/protocol source work. The current
SemSource migration treats that as a substrate concern only: SemSource proves
binary-by-reference storage and governed metadata, while KLV, MISB ST 0601,
STANAG 4609, SAPIENT, SKG, parser, translation, and protocol-conformance claims
belong to SemOps or a SemOps-owned worker.

## Goals / Non-Goals

**Goals:**

- Update SemSource to the latest released governed SemStreams tag.
- Keep SemSource source-focused and SemOps product-focused.
- Make standalone SemSource governance real, observable, and testable.
- Keep headless SemSource honest about which process owns graph governance.
- Prove opaque synthetic binary handling with bounded memory and
  graph-by-reference before any downstream protocol work.
- Preserve current source entity identity and query contracts unless tests show
  they conflict with SemStreams governed graph rules.

**Non-Goals:**

- Implement the binary protocol source in this migration.
- Move SemOps COP fusion or feed semantics into SemSource.
- Enable hard owner-lease enforcement by default on the first SemSource bump.
- Claim KLV, MISB ST 0601, STANAG 4609, SAPIENT, SKG, streaming-binary,
  parser, translation, or protocol conformance.
- Rewrite SemSource's source handlers before the dependency and governance
  gates are green.

## Decisions

### 1. Target the latest released tag, not SemStreams main

Use `github.com/c360studio/semstreams v1.0.0-beta.114` as the migration target.
The local SemStreams `main` is one commit past the tag, so implementation should
avoid unreleased APIs unless a separate SemStreams tag is cut.

### 1a. Preserve a SemSpec headless pin before SemSource breaks forward

SemSpec uses SemSource in headless mode, so SemSource should not force SemSpec
onto the governed migration before SemSpec is ready. The branch
`origin/semspec/headless-stable` points at pre-migration SemSource commit
`377e063b1c6b88ccf8df3441316b03ba2b04b71c` and can be used as the SemSpec pin
while `main` moves through the SemStreams beta.114 migration.

SemSpec's current compose files consume `ghcr.io/c360studio/semsource:latest`.
SemSource CI also published the pre-migration image as
`ghcr.io/c360studio/semsource:377e063`, so SemSpec can pin the image directly
without waiting for a branch build.

### 2. Treat Go 1.26.3 as phase zero

SemSource cannot compile against beta.114 until the module, CI image, developer
toolchain, and Docker build use Go `1.26.3`. This must land before API fixes are
meaningful.

### 3. Keep the Graphable source lane, then add governance around it

The `semsource.entity.v1` Graphable payload remains the first migration lane.
It already provides entity ID, triples, payload type, and optional storage
reference. The first governed contract can be broad:

```text
owner: semsource.source-service
message_type: semsource.entity.v1
entity_pattern: *.semsource.*.*.*.*
mode: replace-owned for source-owned current metadata
mode: append-evidence for evidence/provenance facts
foreign_edges: only after cross-subject audit names them explicitly
indexing_profile: content for documents/media text, control for source manifests,
                  trace for raw decode/replay metadata where applicable
```

The implementation may split this into source-specific contracts if predicate
ownership is too broad, but the migration should start by making ownership
visible and testable rather than pretending each source family is already a
separate governed producer.

### 4. Make indexing profile an explicit source contract

SemStreams index profiles are a good fit for SemSource because SemSource mixes
human-authored content, structural source metadata, future telemetry-like source
signals, and binary decode traces in one graph. The profile should decide
retrieval/indexing treatment, not whether the graph fact exists.

Recommended first-pass mapping:

- `content`: docs, URL text, extracted transcripts/OCR/descriptions, code
  comments, and other human-authored source text that should be semantic
  retrieval material.
- `control`: source manifests, repo/package/config metadata, service status,
  commit identity, and low-cardinality operational facts.
- `signal`: future observation-like facts from binary/protocol sources when
  SemSource projects sampled measurements rather than raw packets.
- `trace`: parser diagnostics, decode/replay records, keyframe extraction
  traces, packet offsets, byte ranges, and other mechanically generated audit
  artifacts.

SemSource should add an `indexing_profile` payload field and implement
SemStreams `message.IndexingProfiler` on `EntityPayload`. Source handlers must
set the field intentionally. Relying on the SemStreams registry default would
classify unknown message types as `control`, which is a safe floor but a bad
migration outcome for documentation and text sources because they would silently
fall out of the semantic retrieval corpus.

### 5. Bootstrap ownership before graph-ingest in standalone mode

Standalone SemSource must create ownership buckets and bind projection contracts
before graph-ingest starts, otherwise graph-ingest will run in graceful-skip mode.
The implementation should either reuse SemStreams service helpers when they are
generic enough, or call `ownership.EnsureBuckets` plus
`projection.BindAndHeartbeat` directly with SemSource owner IDs.

SemStreams ownership claims require exact predicate strings; predicate wildcards
are rejected. The first bound SemSource contract therefore depends on the
predicate inventory and cross-subject audit. Do not register a pretend wildcard
contract just to make OWNER_CLAIMS exist.

The beta.114 migration classifies every predicate returned by
`internal/governance.OwnedPredicates()` as `replace-owned` under
`semsource.entity.v1`. That inventory expands exact registered `source.*` and
`code.*` predicates plus the AST capability, signature, and `dc.*` predicates
SemSource emits. There are no append-evidence predicate groups in this phase.
Foreign references are represented as relationship objects on the emitting
entity's own subject, not as foreign-subject triples, so they remain covered by
the replace-owned source-entity contract.

Predicate families by source type:

| Source family | Predicate families | Ownership class |
| --- | --- | --- |
| Documents and URLs | `source.doc.*`, `source.web.*` | replace-owned |
| Git | `source.git.*` | replace-owned |
| Config files | `source.config.*` | replace-owned |
| Media and generated frames | `source.media.*` | replace-owned |
| AST/code | `code.*`, `dc.*`, capability predicates | replace-owned |

`enforce_owner_lease` should remain false for the first migration release. The
acceptance signal is zero owner-lease mismatch and zero unexpected
entity-not-found rejections under the live graph smoke before considering an
enforcement flip.

### 6. Headless mode declares the host contract

In headless mode the host app owns graph infrastructure. SemSource should not
silently assume ownership buckets exist. It should expose or log whether it is:

- publishing governed source payloads to a host that owns OWNER_CLAIMS, or
- running governance-disabled because the host has not adopted the substrate.

This is especially important for SemOps, where the source sidecar may not be the
same process that owns the COP graph.

### 7. Binary source support is metadata plus storage reference

Opaque binary source bytes stay in media/object storage. Graph triples contain
deterministic identity, content hash, byte ranges, storage reference, and
format-neutral extraction/proof findings. The migration fixture is synthetic and
must not be used to claim KLV, MISB ST 0601, STANAG 4609, SAPIENT, SKG,
streaming-binary, parser, translation, or protocol conformance.

The governed migration adds a streaming write path for SemSource-owned local
filestore storage: handlers call `handler.StoreFile`, and `filestore.Store`
implements `PutReader`. Stores that only implement the SemStreams `Store.Put`
byte-slice API still fall back to full-file reads, so SemOps binary claims must
name the configured storage backend until SemStreams has a generic streaming
write interface.

KLV/MISB/STANAG/SAPIENT processing is a SemOps product concern. A downstream
SemOps worker should consume SemSource storage references, perform demux/parsing
and schema translation, and publish governed derived facts or operational
schemas such as CoT, CS API JSON, or COP-specific projections.

Recommended fixture ladder:

1. Opaque synthetic binary fixture for SemSource storage/governance only.
2. Public KLV sample smoke in SemOps, subject to license/provenance review.
3. Deterministic MISB ST 0601 fixture in SemOps with truth JSON round-trip.
4. Formal STANAG 4609 certification as a separate validator/lab track.

## Rollout Plan

1. Add Go `1.26.3` toolchain support and update CI/Docker.
2. Bump SemStreams to beta.114 and fix compile/API drift.
3. Update graph subsystem component config to current SemStreams ports and query
   surfaces.
4. Add ownership bootstrap and a SemSource projection contract in standalone
   mode.
5. Add headless governance status and docs.
6. Audit source triples for cross-subject edges and must-exist ordering.
7. Run unit tests, integration tests, and a live NATS graph smoke that proves
   SemSource entities land in ENTITY_STATES with envelopes.
8. Add the binary-source proof spike only after the governed graph migration is
   green.

## Risks / Trade-offs

- The broad `semsource.entity.v1` contract may be too coarse for future
  multi-writer SemOps usage. Starting broad is acceptable only if follow-up
  tests make predicate ownership visible and splitting cheap.
- Headless mode can hide governance problems if the host does not expose
  ownership status. Make the contract explicit before SemOps relies on it.
- SemSource-owned local filestore has a streaming write path, but generic
  SemStreams `Store.Put` backends remain byte-slice based until an upstream
  streaming-write contract exists.
- Current docs mention old federation placement and WebSocket raw export
  assumptions. These need cleanup after the graph migration lands.
- A dependency bump without live graph smoke is dangerous here; compile success
  is not enough.
- SemStreams beta.114 graph-gateway still routes `capabilities` GraphQL queries
  to `graph.query.capabilities`, but graph-query no longer registers that
  responder. SemSource should not advertise the subject until upstream resolves
  the query contract.

## Open Questions

- Should SemSource keep one `semsource.entity.v1` payload or introduce
  source-specific payload types for governance and indexing profile precision?
- Which SemSource predicates are replace-owned versus append-evidence?
- Does SemSource standalone need its own ownership service wrapper, or should it
  reuse SemStreams service boot helpers after a small upstream generalization?
- Should SemSource expose a health/status endpoint that reports governance
  readiness, OWNER_CLAIMS visibility, and owner-lease mismatch counters?
