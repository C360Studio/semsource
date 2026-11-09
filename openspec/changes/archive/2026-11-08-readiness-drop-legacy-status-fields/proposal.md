# Readiness drops the legacy status fields

Closes [#227](https://github.com/C360Studio/semsource/issues/227). Milestone 1 of the SemEngine port.

## Why

The readiness decoder copied `revision` and `last_synced` from the graph index/embedding status
reply into every readiness object SemSource emits. Both duplicate `indexed_revision` at the frozen
SemStreams pin and were never load-bearing. SemEngine ADR-109 deletes the legacy
`Phase`/`Revision`/`LastSynced` trio from graph-index (semengine#110 item 1), so keeping them would
break the first SemEngine build on a field nobody gates on.

## What Changes

- The canonical readiness object (MCP `source_status`, HTTP `/source-manifest/status`, workbench
  capabilities) carries only `available`, `ready`, `state`, `indexed_revision`, `target_revision`,
  `lag`, and `reason`. `revision` and `last_synced` are no longer emitted; `phase` was never decoded.
- The UI capabilities contract stops reading both fields and the workbench stops rendering
  "Last index progress".
- **Breaking** for any reader of `revision` / `last_synced` on those objects. SemSource's own UI
  is the only known reader; freshness comparison already uses the numeric `indexed_revision`.

## Non-goals

- The fusion response `index` object (`ui/src/lib/contracts/fusion.ts`) is the upstream
  `pkg/fusion` shape, not SemSource's readiness object; it moves with the fusion port (SemEngine
  change 5).
- ADR-109's full adoption — consumers reading readiness from query replies instead of the
  `GRAPH_STATUS` bucket — happens at the port; recorded on #226 for the ledger.
