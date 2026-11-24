# Refuse media sources that would share an entity-ID system — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` (2026-11-24, sha256:338bb96242ad); line entered by claude at the owner's instruction. Ruling: #366 comment 7136610830 (per-kind media spaces).

## 2. Media kinds

- [ ] 2.1 `idSpace` rows for image-, audio- and video-source, one space each; `CheckSourceCollisions` plans `image`, `audio`, `video` entries.
- [ ] 2.2 `TestCheckSystemCollisions` rows: same-kind overlap (`[/m/x,/m/b]` + `[/m/b]`) refused; two roots whose basenames slug to one system refused; image and audio over one root admitted.

## 3. Refusal wording

- [ ] 3.1 A narrowing re-add says remove, then add (both the system and the takeover message at add).
- [ ] 3.2 `CheckSourceCollisions` runs the same-instance check before the system check; test that the message names the two root sets, not one root twice.
- [ ] 3.3 "no discovered root is kept" says a restart reconciles the discovery record.

## 4. Gates

- [ ] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./internal/sourcespawn/...`, `task spec:check`.
- [ ] 4.2 Review via `ateam review request`; archive before ready.
