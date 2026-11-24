# Key cfgfile entities by their declaring file — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` + `design.md` (2026-11-24, sha256:b1d1fdb976aa); line entered by claude at the owner's instruction. Option A, all cfgfile types.

## 2. Repro (fails on main)

- [x] 2.1 Two POMs declaring one coordinate (scope `test` / none), and a POM plus a build.gradle with the same coordinate: today one `config.dependency` ID with flipping facts.
- [x] 2.2 Two Dockerfiles sharing a base image with different EXPOSE; `a/core/build.gradle` and `b/core/build.gradle`.
- [x] 2.3 The same tree ingested from two absolute checkout locations yields different IDs today (fallback paths).

## 3. Identity

- [x] 3.1 Compute `rel` (slash-separated, relative to the source root) once per file for the walk and watch paths (`declaringPath`); `TestConfigHandler_Watch_MintsTheSeedIDs` pins watch IDs to the seed walk's.
- [x] 3.2 Every cfgfile constructor appends `hash8(rel)`; the fallbacks hash `rel`.
- [x] 3.3 Update `handler/cfgfile` ID assertions and any integration/e2e fixtures that pin cfgfile IDs (none pinned: existing suites assert by type and predicate; mcp-gateway/governance fixtures are synthetic IDs).

## 4. Gates

- [x] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./handler/cfgfile/...`, `task spec:check`.
- [ ] 4.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
- [x] 4.3 Filed https://github.com/C360Studio/semsource/issues/375 (cross-root and within-manifest collisions; prerequisite for the #336 opt-in). graph-event-reviewer findings: root-level Gradle name and nameless go.mod fixed here.
