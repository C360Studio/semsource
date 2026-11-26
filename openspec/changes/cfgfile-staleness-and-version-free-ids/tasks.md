# Config-file staleness and version-free IDs — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval.

## 2. Repro (fails on main)

- [ ] 2.1 Go and npm version bump leave one dependency entity with the new version; Maven and Gradle likewise.
- [ ] 2.2 Two tags of one image in one Dockerfile are one entity; the registry port is kept in the name.
- [ ] 2.3 Deleting a watched manifest marks its module and dependency entities stale; re-creating clears it.

## 3. Identity (D1)

- [ ] 3.1 Version-free instances in the four dependency builders; `imageName`; multi-valued version/reference folding.

## 4. Staleness (D2)

- [ ] 4.1 Owner fact and declaring file path on every cfgfile entity.
- [ ] 4.2 cfgfile-source lifecycle trigger on seed and on watch delete.
- [ ] 4.3 `pathOf` reads `source.config.file-path`; the stat oracle handles absolute paths.
- [ ] 4.4 Document retained removed-dependency entities in the cfgfile spec text.

## 5. Gates

- [ ] 5.1 `task verify`; `go test -race -tags=integration ./handler/cfgfile/... ./processor/cfgfile-source/... ./processor/supersession/...`.
- [ ] 5.2 `graph-event-reviewer` pass; review via `ateam review request`; archive before ready.
- [ ] 5.3 Identity-break and reseed note recorded on #408 for the beta.12 ROADMAP/adoption note.
