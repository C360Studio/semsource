# Refuse media sources that would share an entity-ID system — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` (2026-11-24, sha256:338bb96242ad); line entered by claude at the owner's instruction. Ruling: #366 comment 7136610830 (per-kind media spaces).

## 2. Media kinds

- [x] 2.1 `idSpace` rows for image-, audio- and video-source, one space each; `CheckSourceCollisions` plans `image`, `audio`, `video` entries.
- [x] 2.2 `TestCheckSystemCollisions` rows: same-kind overlap (`[/m/x,/m/b]` + `[/m/b]`) refused; two roots whose basenames slug to one system refused; image and audio over one root admitted.

## 3. Refusal wording

- [x] 3.1 An add that rebuilds a registered instance from other roots says remove, then add (`addRemedy`, both the system message via `collision.movesRegistered` and the takeover message); a new source's own collision keeps `projectRemedy`.
- [x] 3.2 `groupCollisions` counts one writer listed twice once. Amended from "run the same-instance check first": that reorder made `TestValidateRefusesSourcesSharingASystem` and `TestLoadAndExpandConfigRefusesSourcesSharingASystem` fail, since the spec scenario wants the shared system named; proposal amended to match.
- [x] 3.3 "no discovered root is kept" says a restart reconciles the discovery record.

## 4. Gates

- [x] 4.1 `task lint`, `go test ./...`, `go test -tags=integration ./internal/sourcespawn/...`, `task spec:check`.
- [ ] 4.2 Review via `ateam review request`; archive before ready.
