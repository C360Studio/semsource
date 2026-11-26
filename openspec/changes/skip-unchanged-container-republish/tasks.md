# Skip unchanged folder and repo republishes — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` + `design.md` (2026-11-26, sha256:f497a487db8d); line entered by claude at the owner's instruction.

## 2. Repro (fails on main)

- [x] 2.1 Second reindex of an unchanged tree publishes zero containers (main: every folder and the repo).
- [x] 2.2 Added file: its folder republishes with the new member; containers whose sets did not change do not (the proposal's "ancestors and the repo" was wrong: `a` contains the folder `a/c`, not its files). With the gate off, 2.1 and 2.2 fail as on main. `TestReindexKeepsSeedHierarchyOverAFold` now accepts an unpublished repo, since a re-added folded file would change the set and publish.
- [x] 2.3 Refused container publish on one pass is published on the next.

## 3. Gate

- [x] 3.1 Fingerprint (excluding `dc.terms.created`) and admitted-sum map in ast-source, `// shortcut:` naming the cfgfile/url-source copies.
- [x] 3.2 `publishContainer` skips on match only for the periodic reindex; seed and watch publishes record but never skip.

## 4. Gates

- [ ] 4.1 `task verify`, `go test -race ./processor/ast-source/...`, `go test -tags=integration ./processor/ast-source/...`.
- [ ] 4.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
- [ ] 4.3 Note on #407: re-measure idle writes/s and CPU on beta.12.
