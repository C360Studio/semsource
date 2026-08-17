# Tasks — add identity flags

Fixes #189. Design D1–D3 govern.

## 1. Implementation

- [x] 1.1 `cli/add.go`: `--project` + `--version` on `parseASTFlags` and
      `parseRepoFlags`, passing through to `SourceEntry`. Table tests: both
      types with flags set (fields land in the written config), omitted
      flags (no keys written, behavior unchanged), mixed with existing
      flags.
- [x] 1.2 Docs: README Managing Sources gains the two-versions example and
      the Config File section cross-references the CLI form;
      `docs/QUICKSTART.md` multi-repo prose notes the flags (marked blocks
      untouched — step tables stable).

## 2. Wrap

- [x] 2.1 Full local gate green (fmt/vet/revive, `go test -race ./...`,
      quickstart tracks for the doc edit), `/opsx:verify`, sync delta +
      archive on the branch, PR referencing #189 (Closes #189).
      (Gate green 2026-08-17; quickstart tracks 42s — prose edit left step
      tables intact. Verify: three scenarios ↔ three tests, D1–D3
      followed.)
