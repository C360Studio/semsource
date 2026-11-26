## 1. Contract

- [x] 1.1 Test first: the provenance integration test asserts `merge` per case (honoured/merged; not honoured with reason and code; absent at Tier 0).
- [x] 1.2 Update the three fusion tool descriptions to name `merge.honoured`; keep tools/list within its budget.

## 2. Gates

- [x] 2.1 `task verify`; `go test -race -tags=integration ./processor/code-context/... ./processor/mcp-gateway/...`.
- [x] 2.2 Review via `ateam review request`; archive before ready.
