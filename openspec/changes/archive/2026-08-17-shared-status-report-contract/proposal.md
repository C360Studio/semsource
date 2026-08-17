# One status-report contract: kill the nine duck-typed copies

Tracks [#188](https://github.com/C360Studio/semsource/issues/188).

## Why

The `semsource.internal.status` wire shape is defined **nine times**: all
eight source components hand-roll anonymous inline structs in their
`publishStatusReport`, and source-manifest decodes into its own private
`SourceStatusReport`. `git-source/component.go:412` says it out loud:
*"mirrors source-manifest's SubmoduleStatus (reports are duck-typed)."*

The copies are synced by hand, and `json.Unmarshal` drops unknown fields
silently — so drift becomes silent data loss. It already has: ast-source
reports `backpressure` (the signal that separates a stalled publisher from a
slow one, added by ingest-observability precisely for status-side diagnosis)
and `boundaries_skipped` (nested git trees the walker refused to enter), and
the aggregator eats both before any surface. The quickstart's troubleshooting
had to route users through counter-flatness heuristics instead, and its spec
carries an explicit reservation for the day backpressure surfaces. The
`submodules` field only survived beta.8 because that change edited both
copies at once.

## What Changes

- **One shared report type** — `internal/sourcestatus.Report` (and
  `SubmoduleStatus`) carrying the full field union; all eight producers
  construct it, the aggregator decodes it. Re-declaring the wire shape
  becomes impossible by construction.
- **Strict decode at the aggregator** — semsource is a single process, so
  producer and consumer are always the same build; an unknown field can only
  mean code bypassed the shared type. That is a loud defect (error log, report
  dropped), never silent field loss.
- **Surface passthrough** — `backpressure` and `boundaries_skipped` join the
  per-source status served on `/source-manifest/status` and the MCP
  `source_status` tool; all eight producers wire `Backpressure` from the
  publisher they already embed.
- **Quickstart troubleshooting row** — the entry the onboarding-quickstart
  spec reserved for backpressure lands (prose only; no new executed blocks).

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `ingestion-readiness`: adds two requirements — publisher distress
  (backpressure) is visible per source on every status surface, and the
  internal status report is a single shared contract with strict decode.
- `onboarding-quickstart`: the troubleshooting requirement's observable-signal
  list gains `backpressure` (replacing the not-yet-served reservation).

## Impact

- **New package**: `internal/sourcestatus` (report + submodule types, tests).
- **Producers**: all eight `processor/*-source` components swap anonymous
  structs for the shared type; git-source's private `submoduleState` dies.
- **Aggregator/surfaces**: `processor/source-manifest` decodes strictly and
  passes the two new fields through `SourceStatus`; HTTP + MCP surfaces gain
  them additively (`omitempty` — no envelope break).
- **Docs**: `docs/QUICKSTART.md` troubleshooting gains the backpressure row
  (step counts unchanged — the doc-driven e2e tracks are unaffected).
- **Out of scope**: new instrumentation (e.g. doc-source seed-liveness
  counters — a separate gap the shared type makes cheap later); registering
  the internal report in the payload registry (monomorphic in-process
  chatter; the shared type + strict decode is what prevents recurrence).

## Non-goals

- No changes to readiness semantics, phases, or entity counting.
- No metrics-endpoint changes (backpressure transitions are already logged;
  a gauge can follow if wanted).
- No external-consumer contract breaks: all additions are optional fields.
