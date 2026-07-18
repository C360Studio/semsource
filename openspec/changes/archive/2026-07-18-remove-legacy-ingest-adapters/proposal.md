## Why

SemSource still carries three compatibility surfaces after their replacement contracts are established:
a no-choice runtime `mode`, six legacy single-path AST fields that are converted into `watch_paths`,
and mixed typed/RawEntity doc and URL watch events. These adapters make invalid old configurations and
event producers appear supported. An unused `Watcher.IndexDirectory` method and current stale
federation guidance reinforce the same ambiguity.

## What Changes

- **BREAKING**: remove `Config.Mode`, `ModeStandalone`, and all `SEMSOURCE_MODE` processing; stale
  `mode` JSON is rejected by strict top-level decoding.
- **BREAKING**: make `watch_paths` the sole AST source shape; remove `RepoPath`, `Org`, `Project`,
  `Version`, `Languages`, `ExcludePatterns`, and `GetWatchPaths`, with strict rejection of old keys.
- **BREAKING**: require typed EntityStates for doc/URL create and modify watch events, retain path-only
  delete, and remove only those processors' RawEntity fallback paths.
- Preserve RawEntity for active handlers elsewhere and preserve synchronous ingest contracts.
- Delete zero-caller `Watcher.IndexDirectory` after caller proof.
- Correct only current operational guidance that still directs readers to removed federation APIs.

## Non-goals

- Removing RawEntity repository-wide or redesigning synchronous ingestion.
- Fixing the exact-file bug; redesigning lifecycle, source readiness, SafeConfig membership, or
  ConfigManager; adding NATS attestation/inventory or generalized audit frameworks.
- Reorganizing vocabulary packages, changing component/payload registration, redesigning entity IDs,
  or replacing local validators absent demonstrated incompatibility.
- Editing downstream repositories or rewriting historical ADRs and clearly historical evidence.

## Consumers

SemSource operators and composition builders consume the strict runtime and AST configurations.
Doc/URL processors consume typed watch events. Downstream sem* products receive migration guidance
only when they own a removed configuration key; no downstream code is changed here.

## Capabilities

### New Capabilities

- `runtime-configuration`: the single external-service runtime with no selector.
- `ast-source-configuration`: strict `watch_paths`-only AST configuration.
- `typed-source-change-events`: typed doc/URL watch create/modify and path-only delete behavior.

### Modified Capabilities

- `semstreams-governance`: removes obsolete standalone/headless selector language while preserving
  external-service governance bootstrap.

## Impact

The change removes dead config/schema/environment surfaces, updates owned AST builders and fixtures,
tightens doc/URL watch contracts, deletes one unused API, and corrects narrowly scoped current
federation guidance. It requires breaking configuration notes but no persisted-state migration.
