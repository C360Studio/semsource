# Design: GRAPH removes consumed messages

## Context

Pin semengine `v0.1.0-alpha.8`; main 34dc268. NATS: compose `nats:2.14.10` (`docker-compose.yml:173`), e2e `nats:2.12`
(`test/e2e/e2e_test.go:334`), module `nats-server v2.15.3`.

**Stream.** `graphStreamConfig` (`cmd/semsource/run.go:966-1020`): five subjects (`:968-974`), memory, 256 MiB, 1 h,
discard `new` (`:994`), no retention, so limits. The override (`:997-1017`) has no retention field, and
`StreamOverride` (`config/config.go:73-86`) has none either. The loader decodes strictly (`config/loader.go:29`), so a
`retention` key is already refused. Two test fixtures mirror the bounds (`internal/sourcespawn/sourcespawn_test.go:41-53`,
`internal/governance/source_removal_integration_test.go:47-59`).

**Subjects (Q1).**
- `graph.ingest.entity`. Producer: entitypub `PublishToStreamWithMsgID` (`internal/entitypub/publisher.go:35,531`),
  used by every source and by supersession (`processor/supersession/config.go:102`). Its PubAck counts the entity as
  published (`:544`). Consumer: graph-ingest only (`run.go:828-835`).
- `.manifest/.status/.predicates`. Producer: source-manifest `PublishToStream` (`processor/source-manifest/component.go:
  228,242,272,402,582`), with status every 30 s (`:388`). No consumer in this repo or the pin. The consumer guide's
  Option A tells adopters to subscribe on GRAPH (`docs/integration/m5-consumer-integration.md:43-46`).
- `.batch`. No producer and no consumer. It is kept because the "exactly five subjects" Requirement
  (`tier0-consumer-qualification`) pins it.
- Sister repos declare GRAPH on their own NATS (`semspec/cmd/semspec/main.go:821-825`,
  `semdragon/config/semdragons-semsource.json:175-177`), not ours.

**graph-ingest consumer (Q2).** Durable `graph-ingest-graph-ingest-entity` (semengine
`processor/graph-ingest/consume.go:93-96`, `natsclient/stream.go:855-857`), explicit ack (`stream.go:879-886`), no
inactive threshold, never deleted at Stop (no `DeleteConsumer` in natsclient or graph-ingest). It acks after the
ENTITY_STATES write and the durable guard stamp (`keyed_ingest.go:294-336`). It waits for the stream, then binds
(`consume.go:83-86,153`). Components start concurrently (`service/component_manager_lifecycle.go:71-85`), so sources
can publish before the bind.

**Server facts.** On interest retention with no consumer, the message is skipped and still PubAck'd
(nats-server `server/stream.go:7357-7412`). Workqueue requires deliver-all (10101) and explicit ack (10098), and refuses
overlapping consumers (10099/10100) (`jetstream_errors_generated.go:792-795`). The framework refuses any retention
drift, and the server also refuses a change to or from workqueue (semengine `config/stream_drift.go:28-39,136-140`,
`config/streams.go:549-558`).

## Goals / Non-Goals

Goals: occupancy equals unconsumed backlog; a warm restart within the age bound never refuses; no silent loss is added.
Non-goals: see proposal.

## Decisions

### D1. Retention: workqueue (recommended)

| Option | Restart within 1 h | First boot / fresh stream | Forecloses |
| --- | --- | --- | --- |
| **A. workqueue** | passes: acked entities removed | retained until consumed | a second consumer of entity messages (refused loudly); any later move off workqueue needs a stream recreate |
| B. interest | passes: the durable holds interest across restarts | **silent loss**: publishes before the bind are skipped and PubAck'd | nothing structural; needs consumer-before-producer, which is upstream (rule 2) |
| C. limits, bigger cap or shorter age | moves the wedge, doesn't remove it; a shorter age evicts undelivered messages silently | as today | nothing; costs RAM that grows with corpus × restarts per hour |

Cost of A: one field (`Retention: "workqueue"`) plus reworking the e2e tests that consume GRAPH. Messages on
non-entity subjects with no consumer stay until the age bound (status every 30 s; bytes not measured).

Adopter seam: an adopter never needs to know the retention. If they do nothing, nothing changes for them. The one
outcome they can observe: a GRAPH reader they add is refused at bind (10100/10101), which is loud. Option A of the
consumer guide becomes "poll status" (Options B/C).

What would change my mind: a planned second consumer of entity messages (an audit tap, say). That needs B plus the
upstream ask.

### D2. Storage stays memory for beta.11

A, recommended: keep memory. A NATS restart then loses PubAck'd, unconsumed facts that entitypub already counted as
published (`publisher.go:544`). That silent-loss path exists today. Cost: none now; the later move to file storage
needs another stream reset, since the server refuses storage drift (`stream_drift.go:28-31`).

B: file storage now, on `nats-data` (`docker-compose.yml:175-177`). It rides the beta.11 break for free. Disk holds only
the backlog. Seed-time cost is unmeasured.

What would change my mind: the owner preferring one break over a smaller scope.

### D3. Retention is fixed; the named posture follows

A, recommended: hard-coded, with no override field. The strict decoder already refuses `retention`.

B: add a raw `retention` override. That widens exactly the knob surface #160 asks to remove.

C: ship the named posture now. That pulls validation and docs into beta.11.

### D4. Bounds keep 1 h / 256 MiB / discard new

The age bound must stay finite (semengine `config/streams.go:20,29`) and at least the duplicate window
(`streams.go:48-50`). Once consumed messages are removed, expiry only hits unapplied messages. That is a silent loss,
but it needs graph-ingest stalled for an hour, which is reachable today too. The refusal rationale (`run.go:978-993`)
survives. Its "terminal" wording is corrected to match the code, where 10077 is retried within the 90 s budget
(`internal/entitypub/classify.go:60-65`, `publisher.go:63,67`).

## Risks / Trade-offs

- **Workqueue messages that hit MaxDeliver.** Whether the server keeps them until the age bound is not established.
  graph-ingest sends poison messages to `Term`, so the exposure is small.
- **The e2e tests mask the first-boot path today.** Their consumer exists before semsource starts. After D1-A it doesn't
  matter, since workqueue retains without a consumer.
- **A wrong NATS on the host.** An operator who keeps NATS across the upgrade hits a boot refusal, not data loss.

## Migration Plan

Fresh stream, already required by beta.11. The boot refusal names the retention drift and its remedy
(`stream_drift.go:35-39`). Adoption note line (#386): "GRAPH now removes applied messages; a NATS server holding the
beta.10 GRAPH stream refuses boot. `docker compose down -v` (already required), or `nats stream rm GRAPH`. GRAPH holds
only transport, never graph state."

## Open Questions

1. **Owner ruling: D1.** Workqueue, or interest with an upstream ask?
2. **Owner ruling: D2.** File storage in beta.11, or the follow-up?
3. **Owner ruling: #160 closure.** This change leaves the named posture for later. Should PR #394 say `Addresses #160`
   and file a follow-up, or should #160 be retitled?
4. **Owner ruling: D4.** Keep the 1 h age bound, now that every expiry is an unapplied fact?
5. Measured in task 2, not ruled: GRAPH bytes for one fixture seed, and the non-entity bytes per hour under workqueue.
