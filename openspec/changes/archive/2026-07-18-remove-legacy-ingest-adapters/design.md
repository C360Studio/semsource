## Context

SemSource has one external-service runtime, yet `Config.Mode` still accepts/defaults `standalone` and
reads `SEMSOURCE_MODE`. The AST component supports canonical `watch_paths` but converts six deprecated
top-level fields through `GetWatchPaths`. Doc and URL watch paths can mix typed EntityStates with a
RawEntity fallback. These are compatibility adapters, not active product choices.

## Goals / Non-Goals

**Goals:**

- reject removed runtime and AST JSON keys through strict decoding;
- make owned configuration builders use the sole canonical shapes before deletion;
- make doc/URL watch create/modify typed-only while retaining path-only delete;
- remove a proven zero-caller watcher API; and
- correct current stale federation instructions without rewriting history.

**Non-Goals:**

- repository-wide RawEntity removal, synchronous ingest redesign, or exact-file repair;
- lifecycle, readiness, SafeConfig membership, ConfigManager, registration, or graph changes;
- NATS inventory/attestation, generalized audits, vocabulary reorganization, entity-ID redesign,
  speculative validator replacement, downstream edits, or historical-document cleanup.

## Decisions

### D1. Remove the runtime selector rather than deprecate it again

There is one runtime. Delete `Config.Mode`, `ModeStandalone`, defaulting, validation, environment
reads, schemas, examples, and behavior tests together. Existing strict top-level JSON decoding rejects
`mode` as an ordinary unknown field. `SEMSOURCE_MODE` becomes inert because the program no longer
reads it. No custom translator or removed-mode branch remains.

### D2. Decode AST component configuration strictly and accept only `watch_paths`

Repository-owned builders and fixtures move first to complete `watch_paths` entries. The component
then uses SemSource-owned strict unknown-field decoding and deletes `RepoPath`, `Org`, `Project`,
`Version`, `Languages`, `ExcludePatterns`, and `GetWatchPaths`. Validation requires at least one valid
entry and does not invent an implicit current-directory source. Global watcher controls remain.

### D3. Make doc and URL watch create/modify typed-only

For create and modify, doc and URL handlers used by their processors provide canonical EntityStates.
The processors publish only that typed collection and treat a non-delete event without valid typed
state as a visible contract error with no publication. Delete remains a path/operation signal and may
carry no state. RawEntity and `ChangeEvent.Entities` remain available to active raw handlers elsewhere,
and synchronous `Ingest` signatures are unchanged.

### D4. Delete `Watcher.IndexDirectory` only after positive caller proof

A bounded definition/reference check must show zero production and test callers and identify the
existing component initial-index path. The method is then deleted rather than deprecated. Discovery of
a caller stops this task for contract review.

### D5. Correct only current stale federation guidance

Current entry-point and operating documentation must describe supported SemStreams packages and the
external-service/query contract. Accepted ADRs, archived changes, and clearly labeled historical
specification text are history, not cleanup targets. This change does not broaden into general docs
rewriting.

## Risks / Trade-offs

- Existing configs with removed keys stop loading; strict rejection and direct old-to-new examples
  make the break explicit.
- Generic component decoding may ignore unknown keys; focused strict-decoder tests must fail before
  field removal and pass afterward.
- A handler could emit an untyped create/modify event; processors report the contract error rather
  than silently normalizing or dropping it.
- An overly broad RawEntity cleanup could break other handlers; ownership tests name the retained raw
  paths and scope deletion to doc/URL watch processing.

## Rollout Plan

Inventory definitions and callers, write failing strict-config and typed-event tests, migrate owned
AST configurations, remove runtime/AST adapters, remove doc/URL fallbacks, delete the proven unused
method, then correct the bounded current guidance. Publish breaking configuration examples and run all
unit, integration, race, static, and strict OpenSpec gates.
