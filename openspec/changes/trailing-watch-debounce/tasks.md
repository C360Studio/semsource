# Trailing watch debounce — tasks

## 1. Approval

- [ ] 1.1 Hold: OPEN-Q proposal awaits owner approval.

## 2. Repro (fails on main)

- [ ] 2.1 Spread-out burst (gaps < delay, total > delay) emits one event.
- [ ] 2.2 Continuous writes past the cap flush before they stop.

## 3. Watcher

- [ ] 3.1 `pendingEvent{op, first, last}`, readiness check, `max(delay/4, 10ms)` tick, injectable clock.
- [ ] 3.2 `TestFSWatcher_Debounce` keeps its intent on the new rule (one event per burst).

## 4. Gates

- [ ] 4.1 `task verify`; `go test -race -tags=integration ./handler/...`; stress the fswatcher integration tests (`-count=20`).
- [ ] 4.2 Review via `ateam review request`; archive before ready.
