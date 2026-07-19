# Tasks: Source Removal Integrity

## 1. Honest error path

- [ ] 1.1 Remove handler resolves handle against the manifest registry; unknown → reserved
      `NOT_FOUND` error reply (D1); unit tests incl. double-remove
- [ ] 1.2 MCP gateway surfaces the NOT_FOUND reply as a tool error (composes with
      honest-readiness-and-errors' envelope mapping)

## 2. Real deregistration

- [ ] 2.1 Wire component stop: ServiceManager despawn/stop (or context-cancel quiesce fallback);
      if the framework lacks a stop primitive, record the ask in
      docs/upstream/semstreams-asks.md (D2)
- [ ] 2.2 Delete the manifest entry and drop the aggregator's per-source record on remove; status
      reflects removal within one aggregation tick (document the bound in the tool description)
- [ ] 2.3 Unit tests: aggregator forgets removed sources; no further status reports accepted from
      a removed instance

## 3. Instance-scoped bookkeeping

- [ ] 3.1 Fix expanded-repo removal: manifest keyed per instance; removing one instance leaves
      siblings (audit: whole-repo erasure at processor/source-manifest/ingest.go:229); tests

## 4. Proof

- [ ] 4.1 Integration test: add git source → status shows it → remove → status drops it within
      the bound AND ingestion stops (no new entities after removal)
- [ ] 4.2 Add the MCP round-trip (add → remove → NOT_FOUND on unknown) to the compose smoke

## 5. Finalize

- [ ] 5.1 Release note (NOT_FOUND semantics); `openspec validate source-removal-integrity`; gates
      green (revive v1.15.0, gofmt, vet, `go test -race`)
