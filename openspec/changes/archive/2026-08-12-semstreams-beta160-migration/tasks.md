# Tasks: semstreams-beta160-migration

## 1. Pin and mechanical compile migration

- [x] 1.1 Pin `github.com/c360studio/semstreams v1.0.0-beta.160` (accepting transitive nats.go
      v1.52.0), `go mod tidy`, commit the pin separately so every later commit is bisectable
- [x] 1.2 Migrate all 8 source processors' port declarations to the strict `PortDefinition`
      envelope (typed `config` with `kind`; JetStream inputs carry explicit `stream_name` +
      nonempty `subjects`), following `semstreams/processor/ast-indexer` on beta.160 as the
      canonical shape; update the `component.go` readers off the removed flat fields
- [x] 1.3 Replace `semgraph.OpenCatalogBucket` with `OpenCatalogReader` in
      `internal/graphstatus/reader.go` (reader role; `GRAPH_STATUS` is a retained surface)
- [x] 1.4 `go build ./...` clean; `go vet ./...` clean (test binaries compile — fix
      `test/e2e/beta148_cutover_test.go` retired-wrapper references and any test doubles over
      deleted APIs)

## 2. Governance: ownership → local projection intent (D1)

- [x] 2.1 Rewrite `internal/governance/bootstrap.go`: delete the ownership registry/heartbeater
      bootstrap; declare local `projection.Contract` intent; no ownership bucket is created or
      read anywhere
- [x] 2.2 Migrate `internal/governance/predicates.go` seeding to typed CAS reconcile
      (`projection.ModeReconcile`, exact entity + KV revision read first)
- [x] 2.3 Handle `entity_not_found` / `revision_mismatch` / `commit_unknown` as distinct,
      loudly-surfaced outcomes with no blind retry; bootstrap fails startup naming the outcome;
      restart converges by reconcile (idempotence test)
- [x] 2.4 Update governance unit + integration tests to the new contract; delete
      ownership-substrate test fixtures outright (no compatibility doubles)

## 3. Config surface and strict startup

- [x] 3.1 Migrate every shipped config (`configs/*.json`, examples, tier configs): outer
      `services.<name>.enabled` flags, remove message-logger/metrics inner flags if present, bump
      top-level `version` on every touched file
- [x] 3.2 Prove strict flow validation passes: fresh compose bring-up reaches
      phase=ready + index.ready + embedding.ready on newly provisioned NATS storage
- [x] 3.3 Verify `StorageReference` exact-instance resolution: config names the `objectstore`
      registry entry exactly; offloaded-body path proven live (bodies resolve, none excluded)

## 4. Behavioral regression proofs (D5)

- [x] 4.1 Append-no-stub: regression test proving every SemSource append path whose target may
      not pre-exist (supersession relations first) births the entity with its envelope before
      appending; test fails on auto-vivify assumptions
- [x] 4.2 `response_too_large`: req/reply consumers classify it as a result-size failure distinct
      from timeout, with a unit test on the classification path
- [x] 4.3 Full suites green: `go test ./...`, `go test -tags=integration ./...`,
      `go test -race -tags=integration ./...`, `go test -tags=e2e ./test/e2e/`

## 5. No-shims gate and docs (D4)

- [x] 5.1 Retired-symbol grep gate — zero hits outside `openspec/changes/archive/` for:
      `pkg/ownership`, `ModeReplaceOwned`, `ReplaceOwned`, `BindAndHeartbeat`,
      `OpenCatalogBucket`, `OWNER_CLAIMS`, `OWNER_PRESENCE`, `SearchGraph`, `SummarizeGraph`,
      `NATSQuerier`, `StartService`, `StopService`, `RuntimeConfigurable`, and flat
      `Type:`/`Subject:`/`StreamName:` fields in `PortDefinition` literals
- [x] 5.2 Update `CLAUDE.md` roadmap facts (beta.160 pin, fresh-storage adoption) and any README
      deployment notes; draft the consumer adoption note (semspec canary first, then semteams,
      OSH): fresh NATS storage on upgrade, read contracts retained

## 6. Migration proof (D6)

- [x] 6.1 Fresh-stack scorecard rerun: all three arms on questions v3 over the same corpus
      commit; commit results + a SUMMARY comparing per band vs the 2026-08-09 baseline
      (annotated with the substrate pin change); recall regression = migration defect — fix
      before proceeding, do not annotate around it
- [x] 6.2 Record product proof per the upstream guide: tag + migration commit, schema/client
      regeneration, strict validation pass, retired-surface removal (5.1 output), green suites,
      e2e proof — as the PR description's proof section
- [x] 6.3 Post adoption status to our remaining open upstream ask #603 thread if the impact facet
      shape changed under beta.160 (verify against the new pin before commenting)

## 7. Specs

- [x] 7.1 Apply the `semstreams-governance` delta (target beta.160; projection-intent
      requirement); verify `entity-publish-integrity`, `ingestion-readiness`,
      `compose-deployment`, `runtime-configuration` against code and add deltas ONLY where
      requirement-level behavior changed; `openspec validate --all` green
