# Design: trailing per-path debounce

`pending` becomes `map[string]pendingEvent{op fsnotify.Op; first, last time.Time}`, still guarded by `pendingMu`.

- `handleFSEvent`: on an existing entry, update `op` and `last`; on a new one, set `first = last = now`.
- `processEvents`: the ticker interval is `max(DebounceDelay/4, 10ms)`. On each tick, `flushPending` takes only
  the entries with `now-last >= DebounceDelay` or `now-first >= 5*DebounceDelay` out of the map, and processes
  them exactly as today (submodule check, hash suppression, event send). Entries that aren't ready stay put.
- A clock function field (`now func() time.Time`, default `time.Now`) lets unit tests drive readiness without
  sleeping. The integration test keeps real time.

Ordering across paths stays as today: map iteration, with no cross-path order guaranteed.
