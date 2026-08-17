# Tasks — shared status-report contract

Design D1–D5 govern mechanics. Fixes #188.

## 1. Shared contract

- [x] 1.1 `internal/sourcestatus`: `Report` (full field union per D2) +
      `SubmoduleStatus`, doc comments naming the contract rule (one
      definition, no re-declaration), JSON round-trip unit test including
      every field.
- [x] 1.2 Aggregator: decode `semsource.internal.status` into
      `sourcestatus.Report` with `DisallowUnknownFields`; on failure log
      error + drop (D3) with a unit test proving an unknown-field report is
      rejected, not leniently accepted. `sourcemanifest.SourceStatusReport`
      / `SubmoduleStatus` become aliases (D1).

## 2. Producers

- [x] 2.1 All eight `processor/*-source` components construct
      `sourcestatus.Report` instead of anonymous structs; every producer
      wires `Backpressure: c.publisher.InBackpressure()`; ast-source keeps
      its extra counters (now landing in real fields); git-source's private
      `submoduleState` deleted in favor of the shared type.

## 3. Surfaces

- [x] 3.1 `SourceStatus` passthrough: `backpressure` +
      `boundaries_skipped` flow aggregator → `StatusPayload` → HTTP
      `/source-manifest/status` and MCP `source_status` (additive,
      `omitempty`). Component test: a report with both fields set round-trips
      to the served payload.

## 4. Docs + wrap

- [x] 4.1 `docs/QUICKSTART.md`: backpressure troubleshooting row (D5,
      prose-only; marked-block step counts unchanged).
- [ ] 4.2 Full local gate green (fmt/vet/revive, `go test -race ./...`,
      CI integration packages, quickstart tracks), `/opsx:verify`, sync
      deltas + archive on the branch, PR referencing #188 (Closes #188).
