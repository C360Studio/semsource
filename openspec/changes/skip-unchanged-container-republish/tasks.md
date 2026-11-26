# Skip unchanged folder and repo republishes — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval.

## 2. Repro (fails on main)

- [ ] 2.1 Second reindex of an unchanged tree publishes zero containers (main: every folder and the repo).
- [ ] 2.2 Added file: its ancestors and the repo republish with the new member.
- [ ] 2.3 Refused container publish on one pass is published on the next.

## 3. Gate

- [ ] 3.1 Fingerprint (excluding `dc.terms.created`) and admitted-sum map in ast-source, `// shortcut:` naming the cfgfile/url-source copies.
- [ ] 3.2 `publishContainer` skips on match only for the periodic reindex; seed and watch publishes record but never skip.

## 4. Gates

- [ ] 4.1 `task verify`, `go test -race ./processor/ast-source/...`, `go test -tags=integration ./processor/ast-source/...`.
- [ ] 4.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
- [ ] 4.3 Note on #407: re-measure idle writes/s and CPU on beta.12.
