# Debounce each watched path until it goes quiet, with a cap

Approved-by:

Track: design (changes when every fsnotify-based source emits watch events).

## Why

#419: `TestFSWatcher_Debounce` failed on CI (3 events for 5 rapid writes, limit 2). The watcher shared by the doc,
cfgfile, image, video and audio sources (`handler/internal/fswatcher`) flushes pending paths on a fixed
`time.NewTicker(DebounceDelay)` (`fswatcher.go:227`). That batches; it does not debounce. A burst whose events
cross N tick boundaries emits N+1 events for one file, so the test's bound depends on runner speed. The config
documents `DebounceDelay` as "how long to wait for more changes before processing" (`fswatcher.go:38`), which the
ticker does not do. On a real tree, an editor's save or a `git checkout` that writes one file over a few hundred
ms makes several watch events, each one a parse and publish.

## What Changes

- Each pending path remembers when it was first and last seen. A path is emitted once it has had no event for
  `DebounceDelay`, or once `5 × DebounceDelay` has passed since its first pending event, so a file written
  continuously still flushes.
- The flush check runs every `DebounceDelay / 4` (minimum 10 ms). The latency for a single write rises from
  "0 to DebounceDelay" to about `DebounceDelay`.
- The last operation for a path wins, as today. Hash suppression of unchanged content is unchanged.
- `DebounceDelay` / `coalesce_ms` keep their names and defaults (500 ms).
- Tests: a burst spread over several delay periods, with gaps shorter than the delay, emits one event (fails on
  main). A path written continuously past the cap flushes at least once before the writes stop.

## Decisions you are making by approving

- **Trailing debounce with a 5× cap** for all five fsnotify-based sources. The ast-source watcher is separate and
  untouched.
- **Single-write latency becomes about `DebounceDelay`** (500 ms default, up to 1.25× with the check interval),
  instead of 0 to 500 ms.
- **No new config knob** for the cap. It is derived from `DebounceDelay`.

## Out of scope

- The ast-source watcher, which has its own event loop.
- Changing defaults or the `coalesce_ms` surface.
