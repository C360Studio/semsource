## Context

`graphStreamConfig` (`cmd/semsource/run.go:966-1029`) builds GRAPH and then applies `cfg.Streams["GRAPH"]`.
PR #394 fixed retention to workqueue with no override, and left the posture and storage for this change
(archived `2026-11-25-graph-stream-consume-then-remove/design.md` D2, D3). The compose NATS already stores JetStream
on the `nats-data` volume (`docker-compose.yml:165-180`), so file storage needs no deploy change.

## D1 Config shape

`graph_stream` is an object, `posture` plus an optional `max_bytes`. Unknown fields are rejected, and so is the old
top-level `streams` key: the strict decoder (`config/loader.go:29`) refuses it once the field is gone, and the error
names it. `posture` is `durable` (default) or `ephemeral`; anything else fails validation. `max_bytes`, when set,
must be positive.

Rejected: a bare string `"graph_stream": "durable"`. It would need a second key for `max_bytes`, which the e2e
ceiling test and large corpora need.

## D2 What a posture fixes

| | durable | ephemeral |
| --- | --- | --- |
| storage | file | memory |
| subjects, retention, discard, age, replicas | five `graph.ingest.*`, workqueue, new, 1 h, 1 | same |
| max_bytes | 256 MiB unless set | 256 MiB unless set |

The postures differ only in storage. The subject guard in `config` (tier0 spec) is deleted because no subjects
override remains. The `StreamOverride` type goes with it.

## D3 Measuring file storage before the default flips

Seed the e2e retention fixture (`writeRetentionFixture`, 1,041 entities) five times under each posture on this
laptop (M3 Pro, Docker Desktop NATS). Record wall time from start to `phase: "ready"`, and GRAPH bytes at ready.
Report the median and the spread on the PR.

Bar: a durable median within 15% of ephemeral, or within 2 s, keeps durable as the default. Anything worse goes to
the owner with the numbers before the default flips.

## D4 Drift and migration

The server refuses a storage change on an existing stream, and SemEngine's drift check names it at boot. Both
directions refuse: beta.11 memory to durable, and durable to an operator who picks ephemeral later. The remedy for
both is `nats stream rm GRAPH`; GRAPH holds only transport, never graph state.

## Risks

- File storage writes the backlog to disk. With workqueue retention the disk holds only unapplied messages, at most
  max_bytes.
- The measurement runs on one machine. The Intel dogfood host (#407) is not measured here.
