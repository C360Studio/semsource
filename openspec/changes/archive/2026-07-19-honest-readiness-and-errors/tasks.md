# Tasks: Honest Readiness and Errors

## 1. Phase truth

- [x] 1.1 Track per-source initial-seed completion in the status aggregator; aggregate `phase` =
      seeding until all complete, degraded on errored (D1)
- [x] 1.2 Unit test the mid-seed window explicitly (first-report ≠ complete); fix/replace the
      existing phase-transition test that sidesteps it
- [x] 1.3 Integration test: status polled during a slow seed shows `seeding`, flips to `ready`
      only after the last source completes

## 2. One status assembly, two surfaces

- [x] 2.1 Extract shared status composition (status + index + embedding + note) from the MCP tool;
      use it in the HTTP `/source-manifest/status` handler (D2)
- [x] 2.2 Explicit `{available:false, reason}` for failed sub-queries — never omit keys (D3); test
      with a downed responder
- [x] 2.3 Correct the readiness note + tool descriptions to the seed-window-scoped guarantee
- [x] 2.4 Update README + docs/integration/mcp-quickstart.md polling contract (now true); align
      configs/tiers/README.md stale `source_status` wording

## 3. Gateway error mapping

- [x] 3.1 Wrap gateway NATS requests: X-Status header inspection + ADR-060 envelope-shape
      detection → MCP `isError` with envelope message (D4)
- [x] 3.2 Unit tests: envelope reply → isError; success reply passes through with
      contract_version; if header access is unsupported by the client API, record the framework
      ask in docs/upstream/semstreams-asks.md
- [x] 3.3 Integration test: force a downstream handler error; assert isError through a real
      `tools/call`

## 4. Distinct-entity counts

- [x] 4.1 Per-source confirmed-ID sets; `entity_count`/`type_counts`/`total_entities` report set
      cardinality; keep `publish_total` separately named (D5)
- [x] 4.2 Test: repeated periodic reindex leaves counts stable (pin the audit's ×4 folder/repo
      inflation as a regression test)

## 5. Finalize

- [x] 5.1 Consumer heads-up (semspec: ready arrives later, truthfully) in PR description;
      `openspec validate honest-readiness-and-errors`; gates green (revive v1.15.0, gofmt, vet,
      `go test -race`)
