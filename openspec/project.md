# SemSource Project Context

## Purpose

SemSource is the source-knowledge ingestion service for the C360 graph stack. It
turns repositories, documents, URLs, configuration files, media, and future
binary/protocol sources into typed SemStreams graph facts.

SemSource should be useful both as a standalone graph service and as a headless
source sidecar for a host product such as SemOps. The repo should stay source
focused: protocol decoding, media metadata extraction, storage-by-reference, and
graph projection are in scope; product-specific COP fusion, alerting, or UI
behavior belongs in SemOps or another consuming product.

## Product Boundary

- SemSource owns source discovery, source parsing, binary/media by-reference
  handling, entity ID construction, source provenance, and source-graph
  publishing.
- SemStreams owns the governed graph substrate: Graphable ingestion,
  ENTITY_STATES, graph mutation/query APIs, projection contracts, ownership
  claims, indexing profiles, lifecycle primitives, and shared runtime services.
- SemOps owns COP product semantics, feed fusion, tactical UI, and product-level
  graph ownership. SemSource may support SemOps as a service but should not
  absorb SemOps product behavior.
- SemConnect owns OGC Connected Systems API bridge behavior unless a separate
  change explicitly assigns a source-ingest responsibility to SemSource.

## Standing Technical Conventions

- Graph entity IDs remain deterministic 6-part IDs.
- Raw binary bytes do not go into graph triples. Store binaries by reference and
  project metadata, provenance, hashes, offsets, and extraction findings.
- Every graph writer path must carry a semantic envelope. After SemStreams
  ADR-055, no source may rely on `triple.add` auto-vivifying an entity.
- Standalone SemSource should run a governed SemStreams graph substrate. Headless
  SemSource should publish governed source payloads to the host substrate and
  document which ownership responsibilities are host-owned.
- Large dependency migrations use OpenSpec before code changes.
