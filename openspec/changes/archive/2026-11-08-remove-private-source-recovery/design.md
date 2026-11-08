# Corrective source lifecycle boundary

## Status and retained facts

Owner-authorized design; independent component and graph/event contract reviews approved implementation.
This is contract approval only; final implementation review and qualification remain required.
Base: merged #213, `3604a9ce8d5aec717253d7a905389f7910efd25d`.
Keep SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` and every frozen corpus/result unchanged.
This corrective contract supersedes the unarchived `replay-source-removal` implementation direction;
its historical results remain evidence, including failures. No repaired projection is claimed here.

Source-management ingress is HTTP (`http_sources.go:70,90`) and core NATS
(`processor/source-manifest/ingest.go:92,102`), not unsettled JetStream delivery.
SemStreams #759 settlement APIs exist at the pin; #1147 remains the hierarchy for future work.
We introduce no command lane or alternative authority to remove the existing private one.

## Desired state and public responses

ConfigManager remains the only source desired-state authority. Keep full `Enabled:false` envelopes
(`internal/sourcespawn/sourcespawn.go:304`), desired manifest/count repair, deterministic handles,
unknown/repeated-handle errors, re-add `Enabled:true`, and local cancellation-aware serialization.
Use small unexported channel gates in source-manifest/supersession; retain no recovery package for a gate.
A request canceled before admission must make no desired write or graph mutation.
Source Add/Remove performs no graph readiness, graph tail, journal, receipt or effect lookup.
Successful writes return `desired_changed:true`, `restart_required:true`, `runtime_changed:false`.
Successful and partially committed desired-change replies also carry `projection_status:"unavailable"`.
Remove no longer emits `generation` or `projection_phase`; absent fields do not mean projection complete.
Readiness wording concerns post-restart ingestion only, never clearing retained source-removal markers.

Preserve existing nontransactional manifest/component-write boundaries. If a config write commits
before memory application or a later manifest write fails, return the committed desired facts and error.
Keep a narrow retained-config read adapter using `config.BucketName` and `graph.OpenCatalogReader`;
move it out of deleted lifecycle code. It observes ConfigManager-owned state and writes no raw KV keys.
Expose this read seam to source-manifest; no new record, background repair or cross-store transaction.
Manifest retry repair remains request-driven and must not turn a successful repeated remove into success.

Keep authenticated `GET /sources/{id}/lifecycle` solely as HTTP 410 with
`{"error":{"code":"SOURCE_LIFECYCLE_UNAVAILABLE","message":"source lifecycle projection is unavailable"}}`.
It performs no lookup and returns no progress, binding, epoch, seed or effect state.
Keep `graph.lifecycle.source` only as a stateless refusal using the same error code; no effects occur.
Legacy `graph.lifecycle.run` with reason `source_removed` also explicitly refuses the operation.
Neither surface points clients to an owner or retry worker that no longer exists. Update MCP tool
descriptions/replies to the same desired-only, restart-required and projection-unavailable semantics.

## Domain staleness and ingestion

Retain ordinary watched-file deletion/recreation and passage-shrink behavior, exact parent association,
retained history, governed writes, bounded full-page/error handling, and sticky existing `source_removed`.
Path presence or a new publication cannot clear that marker. Automatic source removal and positive
reactivation are deferred at this pin, even after restart. New desired removal preserves graph history
without claiming that retained entities acquired a new marker. Normal source publication is still
at-least-once; preserve immutable queued payloads, PubAck handling, loss accounting and accepted-batch Stop.
Delete private receipt retry/seals, observer binding and seed-proof-only handler context plumbing;
retain ordinary ingestion errors, source status, producer cancel/join and native subscription drains.

## One-way legacy-storage compatibility check

The old root provisioned `SEMSOURCE_SOURCE_LIFECYCLE` even without removal. Presence alone is not refusal.
Immediately after NATS connection, before stream/config provisioning, factories or HTTP serving, perform
one bounded check using existing-only `natsclient.Client.GetKeyValueBucket` then `bucket.Status(ctx)`.
Only `errors.Is(err, jetstream.ErrBucketNotFound)` or authoritative `Status().Values()==0` admits boot.
`Values` counts retained messages, including history/tombstones, not live records (nats.go v1.52.0).
Any nonzero count, unexpected lookup error, status error or expired context fails startup explicitly.
The guard creates no bucket and makes no KV/stream/config write; it owns no watcher or worker.

This intentionally rejects the whole account-wide product bucket, including other authorities,
terminal-looking records and malformed bytes. No key/payload/version/phase parser is retained.
The previous process/writer MUST be stopped before this check; concurrent old writers are unsupported.
The diagnostic names incompatible legacy storage and preserves it. No ignore/reset flag, deletion
command, elapsed-time resolution or inferred terminal outcome is supplied.
Supported operator action is a separately provisioned fresh deployment/account/storage boundary,
with old storage preserved and its writers stopped. Merely renaming a platform stem in the same account
is insufficient. Admitting existing nonempty state needs a separate independently proved migration;
this PR provides none. An inaccessible bucket is not absence, and a fresh store is not a repair claim.

## Developer boundaries

Worker A owns `internal/sourcelifecycle/**`, `internal/sourceintent/**`, `cmd/semsource/**`,
`processor/source-manifest/**`, `processor/supersession/**`, `internal/sourcespawn/**`,
`processor/mcp-gateway/{component.go,tools.go}` and affected
`internal/governance` integration tests. Remove coordinator/root factories and projection execution;
retain only stateless refusals, config observation and a small root-local compatibility guard.
Preserve supersession paging/error/sticky/typed-parent fixes by review; do not restore that package wholesale.
Worker B owns `internal/entitypub/**`, `internal/seedproof/**`, the nine source processors and affected
`handler/**` ingestion plumbing/tests. Remove receipts/epochs/seals while preserving publisher and
producer lifecycle fixes. B removes shared imports before A deletes `sourceintent` contracts.
The owner integrates shared build/CI/package-list changes and additive process tests; no worker
rewrites frozen fixtures or ledgers. Both developers are working concurrently and must preserve others.

## Proof and limits

Write failing public/behavioral tests first: desired change succeeds with graph degraded; partial writes
remain truthful; restart activates tombstones/re-add; explicit unavailability causes zero graph writes;
ordinary file/passage staleness and accepted publisher drain remain unchanged.
Real private-NATS tests cover absent/empty/nonempty/foreign/terminal-looking/tombstone/corrupt/unreadable
legacy storage and unchanged bytes/counts after refusal. Unit seams cover timeout and injected errors.
Run separate process checks for fresh boot, desired remove/re-add across checked restart and unavailable
projection; preserve exact content and sibling facts. Do not convert historical marker expectations to
passes. Source-removal-positive qualification stays deferred, independently of healthy desired changes.
Independent component and graph/event review plus full applicable gates precede integration.
