# Fold same-coordinate declarations within one cfgfile manifest — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` + `design.md` (2026-11-25, sha256:3de5c126b709) as recommended: D1 fold, D2, D3; line entered by claude at the owner's instruction. Re-approved 2026-11-25 at sha256:a36676ef0de3 after the PR #383 review corrected the design's graph-search note (#385); line re-entered by claude at the owner's instruction.

## 2. Repro (fails on main)

- [x] 2.1 One `build.gradle` with one coordinate under `implementation` and `testImplementation`: today two states under one ID and a duplicate `requires` edge (`handler/cfgfile/entities.go:520`, `:678`).
- [x] 2.2 One `pom.xml` with one `g:a:v` under two scopes; and a plain jar plus `<type>test-jar</type>` (`entities.go:371`, `parsers.go:247`).
- [x] 2.3 One `package.json` with one `name@version` in both sections (`entities.go:203`, `parsers.go:142-147`).
- [x] 2.4 One Dockerfile with `FROM foo/bar:1` and `FROM foo-bar:1` (`entities.go:247`).

## 3. Identity and folding

- [x] 3.1 Per-file builders group dependency states by ID and merge usage facts, values sorted; `DepIDs` lists each ID once (`entities.go:594`, `:614`, `:648`, `:678`). Go `indirect` = AND, npm kind = prod if any.
- [x] 3.2 `pomDepXML` gains `type` and `classifier`; instance includes them when type ≠ `jar` or classifier set; facts emitted under new `source.config.dependency-type` / `-classifier` predicates (registered in `source/vocabulary/config.go` with its tests). Plain jar ID unchanged (assert against a pinned ID).
- [x] 3.3 Image instance = `slugify(image)` + `-` + `hash8(image)` before `declared`.

## 4. Gates

- [x] 4.1 `task lint`, `go test -race ./handler/cfgfile/...`, `go test -tags=integration ./handler/cfgfile/...`, `task spec:check`.
- [x] 4.2 `graph-event-reviewer` pass at 886e41c: no blockers; non-blocking notes (32-bit hash collision would fold silently, pre-existing; legacy RawEntity path computes old-rule IDs on watch but never publishes, #244) recorded on the PR. Review via `ateam review request`; archived before ready.
- [x] 4.3 beta.11 notes recorded on the PR for milestone C (image IDs change; `dependency-scope`/`-configuration` may be multi-valued; new `dependency-type`/`-classifier`). #336 reconcile opt-in follow-up filed as #384.
