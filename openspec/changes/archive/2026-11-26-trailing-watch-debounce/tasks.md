# Trailing watch debounce — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` + `design.md` (2026-11-26, sha256:166a0f418e50); line entered by claude at the owner's instruction.

## 2. Repro (fails on main)

- [x] 2.1 Spread-out burst (gaps < delay, total > delay) emits one event: fake-clock `TestDebounce_WaitsUntilThePathIsQuiet`. A real-time variant failed on main (second event) but was dropped after review (#420 r1): a scheduler pause past the delay can legitimately split a burst, so real-time tests assert no counts.
- [x] 2.2 Continuous writes past the cap flush before they stop: `TestDebounce_ContinuousWritesFlushAtTheCap` (fake clock).

## 3. Watcher

- [x] 3.1 `pendingEvent{op, first, last}`, readiness check, `max(delay/4, 10ms)` tick, injectable clock.
- [x] 3.2 `TestFSWatcher_Debounce` is now scheduler-tolerant: at least one event, and rewriting the final content emits nothing (the last flush saw it). Counts live in the fake-clock tests.

## 4. Gates

- [x] 4.1 `task verify`; `go test -race -tags=integration ./handler/... ./processor/...`; fswatcher integration `-race -count=20`.
- [x] 4.2 Archived before ready; review via `ateam review request`; archive before ready.
