# SemOps Binary Source Proof Boundary

SemSource's current binary-source proof is intentionally narrow. It uses an
opaque synthetic fixture to prove source-service behavior, not domain protocol
behavior.

## What SemSource Proves

- Raw bytes are stored by reference through `message.StorageReference`.
- Graph triples contain metadata only: hash, size, byte range, storage key, and
  extraction/proof finding.
- The entity is published as `semsource.entity.v1` with a semantic envelope.
- The entity uses indexing profile `trace`.
- SemSource ownership covers the source-owned metadata predicates.
- Local filestore persistence can write from a stream via `PutReader`.

## What SemSource Does Not Claim

This proof does not claim support, parsing, translation, interoperability, or
formal conformance for:

- KLV
- MISB ST 0601
- STANAG 4609
- SAPIENT
- SKG
- streaming-binary service behavior
- protocol parser behavior
- protocol conformance

## SemOps Product Boundary

KLV/MISB/STANAG/SAPIENT interpretation belongs in SemOps or a SemOps-owned
worker. That worker should consume SemSource storage references, demux or parse
the binary artifact, and publish governed derived facts or operational schemas
such as CoT, CS API JSON, or COP-specific projections.

SemSource remains the substrate: it proves that binary artifacts can be
addressed, hashed, stored by reference, and represented as governed metadata
without placing raw binary payloads in graph triples.

## Recommended Fixture Ladder

1. Opaque synthetic binary fixture: proves SemSource storage/governance only.
2. Public KLV sample smoke: proves a SemOps worker can process a public sample,
   subject to license and provenance review.
3. Deterministic MISB ST 0601 fixture: truth JSON to encoded KLV to MPEG-TS to
   parsed output, proving engineering support for that worker.
4. Formal STANAG 4609 conformance: separate certification/validator track.
