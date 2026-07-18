## 1. Establish failing contracts

- [x] 1.1 Inventory runtime mode definitions/reads, the six AST fields and conversions, doc/URL raw
  fallbacks, `Watcher.IndexDirectory` definitions/callers, and current stale federation guidance.
  - Test: the bounded manifest reconciles every definition and production/test caller and proves
    `Watcher.IndexDirectory` has zero callers before deletion.
- [x] 1.2 Write failing strict configuration tests for `mode`, `SEMSOURCE_MODE` inertness, canonical
  `watch_paths`, all six rejected AST keys, empty watch paths, and runtime schema output.
  - Test: focused config and AST tests fail on current compatibility behavior and classify each old
    JSON key as unknown.
- [x] 1.3 Write failing doc/URL watch tests for typed create/modify, path-only delete, and missing typed
  state, using explicit synchronization.
  - Test: focused handler/processor tests fail while RawEntity fallback remains and assert publication
    plus existing error/health evidence.

## 2. Move owned inputs to canonical shapes

- [x] 2.1 Convert repository-owned AST builders, configs, examples, and positive fixtures to complete
  `watch_paths` entries before removing compatibility fields.
  - Test: owned composition tests pass using only `watch_paths`; remaining legacy hits are definitions
    or exact negative fixtures.

## 3. Remove configuration adapters

- [x] 3.1 **BREAKING** Delete `Config.Mode`, `ModeStandalone`, `SEMSOURCE_MODE` reads, defaults,
  validation, schemas, examples, and compatibility tests.
  - Test: config tests prove normal load without a selector, ordinary unknown-field rejection for
    `mode`, and unchanged behavior when `SEMSOURCE_MODE` is present.
- [x] 3.2 **BREAKING** Add strict AST component decoding and delete `RepoPath`, `Org`, `Project`,
  `Version`, `Languages`, `ExcludePatterns`, `GetWatchPaths`, conversion logic, and legacy schema
  properties.
  - Test: AST config/component tests prove old-key rejection, required valid `watch_paths`, retained
    global defaults, and one-/multi-path startup.
- [x] 3.3 Delete zero-caller `Watcher.IndexDirectory` while retaining the registered language-aware
  initial-index path.
  - Test: focused AST source/component tests pass and the symbol scan has zero definitions or calls.

## 4. Remove mixed doc/URL watch paths

- [x] 4.1 **BREAKING** Make doc and URL watch create/modify events typed-only, preserve path-only
  delete, and remove their processor RawEntity fallback and dual-population paths.
  - Test: doc/URL tests prove typed publication, deterministic delete signaling, no RawEntity values,
    and no publication on missing typed state.
- [x] 4.2 Inventory and preserve RawEntity ownership in other active handlers and synchronous ingest.
  - Test: a focused ownership test maps every remaining raw event path to a named handler and finds no
    doc/URL watch fallback.
- [x] 4.3 Review cancellation, channel closure, error reporting, and delete semantics.
  - Test: go-reviewer sign-off has no unresolved blocking or race finding.

## 5. Guidance and final gates

- [x] 5.1 Correct only current operational federation guidance and publish direct old-to-new runtime
  and AST configuration examples; leave accepted history intact.
  - Test: technical-writer review finds no current instruction to use removed federation APIs or old
    config keys and confirms historical material remains clearly scoped.
- [x] 5.2 Run bounded removed-surface scans, gofmt, vet, revive with warnings failing, unit,
  integration, race, and strict OpenSpec validation.
  - Test: scans are empty outside exact negative/historical classifications; gofmt is clean;
    `go vet ./...`, revive, `go test ./...`, `go test -tags=integration ./...`,
    `go test -race ./...`, and `openspec validate remove-legacy-ingest-adapters --strict` pass.
