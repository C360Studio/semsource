# Key cfgfile entities by their declaring file — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval. Owner approves `proposal.md` + `design.md` (Approved-by line with the spec-queue digest).

## 2. Repro (fails on main)

- [ ] 2.1 Two POMs declaring one coordinate (scope `test` / none), and a POM plus a build.gradle with the same coordinate: today one `config.dependency` ID with flipping facts.
- [ ] 2.2 Two Dockerfiles sharing a base image with different EXPOSE; `a/core/build.gradle` and `b/core/build.gradle`.
- [ ] 2.3 The same tree ingested from two absolute checkout locations yields different IDs today (fallback paths).

## 3. Identity

- [ ] 3.1 Compute `rel` (slash-separated, relative to the source root) once per file for the walk and watch paths.
- [ ] 3.2 Every cfgfile constructor appends `hash8(rel)`; the fallbacks hash `rel`.
- [ ] 3.3 Update `handler/cfgfile` ID assertions and any integration/e2e fixtures that pin cfgfile IDs.

## 4. Gates

- [ ] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./handler/cfgfile/...`, `task spec:check`.
- [ ] 4.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
- [ ] 4.3 File the follow-up: cfgfile opts into the #336 reconcile and the `idSpace` refusal.
