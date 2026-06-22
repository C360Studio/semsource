## 0. Compatibility Pin

- [x] 0.1 Publish `origin/semspec/headless-stable` at pre-migration HEAD `377e063`
- [x] 0.2 Verify GHCR image pin `ghcr.io/c360studio/semsource:377e063` exists

## 1. Toolchain And Pin

- [x] 1.1 Update SemSource `go.mod` from Go `1.25.3` to Go `1.26.3`
- [x] 1.2 Update Docker/CI/tooling so `go version` is `go1.26.3` before tests run
- [x] 1.3 Bump `github.com/c360studio/semstreams` to `v1.0.0-beta.114`
- [x] 1.4 Run `go mod tidy` and review indirect dependency churn
- [x] 1.5 Run `go test ./...` and record compile/API breakpoints

## 2. SemStreams API Compatibility

- [x] 2.1 Fix compile breakage from SemStreams package or interface changes
- [x] 2.2 Update payload registry usage if beta.114 requires stricter type/schema matching
- [x] 2.3 Update component factory registration and service dependency wiring as needed
- [x] 2.4 Update graph-query/gateway request subjects for batch, prefix, summary, and searchGraph
- [x] 2.5 Update docs that still describe old federation or raw WebSocket-only assumptions
- [x] 2.6 Track upstream SemStreams `graph.query.capabilities` route/responder mismatch
  - Filed C360Studio/semstreams#315.

## 3. Governed Graph Boot

- [x] 3.1 Complete the predicate inventory before binding SemSource contracts
- [x] 3.2 Add SemSource projection contract definitions for `semsource.entity.v1`
- [x] 3.3 Add ownership bucket bootstrap in standalone mode before graph-ingest starts
- [x] 3.4 Bind SemSource projection contracts with `projection.BindAndHeartbeat`
- [x] 3.5 Start/stop the static owner heartbeater with SemSource service shutdown
- [x] 3.6 Keep graph-ingest `enforce_owner_lease` false for the first migration release
- [x] 3.7 Add governance readiness logs/status for standalone and headless modes
- [x] 3.8 Add an integration smoke proving OWNER_CLAIMS exposes SemSource ownership

## 4. Indexing Profiles

- [x] 4.1 Enumerate SemSource entity categories by source handler and retrieval intent
- [x] 4.2 Add an `indexing_profile` payload field to `graph.EntityPayload`
- [x] 4.3 Implement SemStreams `message.IndexingProfiler` for `graph.EntityPayload`
- [x] 4.4 Set profiles in source handlers: `content`, `control`, `signal`, or `trace`
- [x] 4.5 Add tests proving document/text entities are `content`
- [x] 4.6 Add tests proving source manifests and generated trace artifacts are not `content`
- [x] 4.7 Add live-smoke assertions that indexing-profile default metrics stay zero

## 5. Triple And Predicate Audit

- [x] 5.1 Enumerate all SemSource predicates by source type
- [x] 5.2 Classify predicates as replace-owned, append-evidence, or foreign-edge candidates
- [x] 5.3 Audit every emitted triple for `Subject != EntityPayload.ID`
- [x] 5.4 Add tests that fail when a source emits an undeclared cross-subject edge
- [x] 5.5 Add tests proving all emitted entity IDs remain valid 6-part NATS KV keys
- [x] 5.6 Add tests proving storage references remain envelope-bearing on graph ingest

## 6. Live Graph Verification

- [x] 6.1 Add an integration smoke that starts NATS and governed graph-ingest
- [x] 6.2 Publish representative git, doc, config, URL, and media source entities
- [x] 6.3 Query ENTITY_STATES and assert MessageType `semsource.entity.v1` is present
- [x] 6.4 Assert no `entity_not_found` mutation rejections occur during the smoke
- [x] 6.5 Assert foreign-edge and owner-lease mismatch metrics stay zero or are explicitly explained
- [x] 6.6 Verify `graph.query.prefix` pagination and `graph.query.summary` work for SemSource entities
- [x] 6.7 Verify indexing profiles are stored and semantic-index defaults are not used unexpectedly

## 7. Binary Source Service Proof

- [x] 7.1 Clarify the SemOps "binary protocol SKG source" target and fixtures
  - SemSource proof is opaque synthetic binary only; KLV/MISB/STANAG/SAPIENT/SKG belongs in SemOps.
- [x] 7.2 Add a tiny binary fixture proof that stores raw bytes by reference, never as graph triples
- [x] 7.3 Prove memory-bounded hashing/extraction for the fixture
  - Local filestore now streams writes via `PutReader`; generic SemStreams Store remains byte-slice based.
- [x] 7.4 Emit governed metadata entities with storage refs, hashes, provenance, and extraction findings
- [x] 7.5 Add a SemOps handoff note stating what SemSource proves and what remains unclaimed

## 8. Review And Rollout

- [x] 8.1 Run go-developer implementation review for API and context/error handling
  - Added publish-boundary validation for payload ID shape, self-subject triples, and required indexing profile.
- [x] 8.2 Run go-reviewer governance review against ADR-055/056 requirements
  - Verified exact-predicate ownership, no wildcard ownership assumption, and standalone owner heartbeat bootstrap.
- [x] 8.3 Run graph-event-reviewer review for entity identity, triples, and foreign-edge behavior
  - Live graph smoke covers representative source entities and synthetic binary proof predicates.
- [x] 8.4 Update README, AGENTS.md, and integration docs after implementation
- [x] 8.5 Publish a migration PR with test evidence and a SemOps compatibility note
  - Draft PR: https://github.com/C360Studio/semsource/pull/2
