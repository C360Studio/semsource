# GRAPH stream: a named durability posture, file storage by default

Approved-by: Coby Leuschke (2026-11-26) sha256:f694bb12b705
Track: design (config surface break and a GRAPH stream reset, both riding the beta.12 reseed).

## Why

#395, ruled 2026-11-26 (follow-up to #160 / PR #394).

1. **Memory storage loses published facts on a NATS restart.** GRAPH is `storage: memory`
   (`cmd/semsource/run.go:975`). A fact that NATS accepted but graph-ingest has not yet applied is gone after a NATS
   restart. entitypub has already counted it as published (`internal/entitypub/publisher.go:544`), so nothing
   reports the loss.
2. **The override surface is raw stream knobs.** `streams.GRAPH` (`config/config.go:72-86`, applied at
   `run.go:1007-1026`) exposes subjects, storage, max_bytes, max_age, replicas and discard. Each one can undo a fixed
   decision: discard "new" (`run.go:986-1004`) or the subject guard (tier0 spec, "GRAPH stream overrides stay on the
   ingest subjects"). No shipped config, deploy file or doc sets it.

beta.12 already requires a fresh store (#393), so the one GRAPH reset that switching storage needs costs nothing
extra now (the server refuses storage drift).

## What Changes

- **BREAKING:** the `streams` config key is removed and the strict decoder rejects it. It is replaced by
  `graph_stream: {"posture": "durable" | "ephemeral", "max_bytes": <optional>}`.
- `durable` (the default) is file storage; `ephemeral` is memory storage. Both postures keep the fixed shape: the five
  `graph.ingest.*` subjects, workqueue retention, discard new, a 1 h age bound and 1 replica.
- `max_bytes` stays tunable (default 256 MiB) because the backlog bound is deployment-sized and the e2e ceiling test
  (`test/e2e/graph_retention_test.go:68`) needs it.
- The first-ingest cost of file storage is measured before the default flips. A regression above the bar in design D3
  goes back to the owner.
- Release note on #408: a NATS server holding the beta.11 memory GRAPH stream refuses boot; `docker compose down -v`
  (already required for beta.12) or `nats stream rm GRAPH`.

## Capabilities

- `entity-publish-integrity`: MODIFIED "The ingest transport stream refuses rather than evicts" (posture replaces raw
  overrides; storage defaults to file).
- `tier0-consumer-qualification`: REMOVED "GRAPH stream overrides stay on the ingest subjects" (no subjects override
  remains to guard).

## Non-goals

- Retention, discard, subjects, age bound and replicas stay fixed; no other knob is added.
- Other streams and KV buckets are SemEngine-owned and untouched.
- Clustered or replicated NATS (MVP is a single LAN node).
