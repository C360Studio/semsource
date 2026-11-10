---
name: kv-or-stream
description: >-
  Decide between a KV watch and a JetStream stream for a new communication path. Use when designing communication
  between components, adding a message flow, or choosing a NATS storage primitive.
---

# KV watch or JetStream stream

Start from the communication the user names. A KV watch is a subscription to changes in a NATS key-value bucket; a
JetStream stream is a durable log that a consumer reads and acknowledges message by message.

## Four tests

Apply them in order; the first clear answer usually settles it.

1. **Restart.** If the component restarted, should it reload current facts, or resume work it had not acknowledged?
   Reload facts: **KV watch**. Resume unacknowledged work: **stream**. A stream redelivers what was not
   acknowledged, not what was not finished: work done before a crash cut off its acknowledgement is delivered again,
   so the owner makes the work safe to repeat or detects the repeat. The cited graph-ingest recovery test demonstrates
   that reapplying the input preserves the graph's statements (`graph-ingest-recovery` spec, "Recovery on a file
   stream is redelivery"; `TestIntegration_Settlement_ProcessKilledBetweenApplyAndAck`).
2. **Fan-out or queue.** Should every interested component react, or only one? Every one: **KV watch**. One:
   **stream**.
3. **Processing time.** Is the reaction fast and idempotent (safe to repeat), or slow with side effects? Fast and
   idempotent: **KV watch**. Slow or with side effects: **stream**.
4. **Nature.** Is it a fact about the world, or a request to do something? A fact (entity state, an index entry,
   current status): **KV watch**. A request (run a task, call a model, run a tool): **stream**.

If the tests disagree, the concept may be two things; split it before choosing.

## What an owner of a KV watch takes on

A KV watch has no acknowledgement, and SemEngine promises no second delivery of a change a handler failed on. An
owner that derives state from a watch applies the desired state idempotently, repairs and re-drives failed work
itself, and reports readiness or degradation where a reader can see it. Treat the start of a watch as loading the
inputs a restart needs, not as a retry loop. If the owner cannot take those on, change the design. Never use a KV
watch as a work queue.

## Common cases

| Communication | Primitive | Reason |
| --- | --- | --- |
| Entity state changed | KV watch | Current fact; fan-out; idempotent reaction |
| A task to run | Stream | Request; one handler; side effects |
| Index update | The index owner's KV write; readers watch it | Only the declared owner writes |
| A slow or costly call (a model, a tool) | Stream | Request; one handler; slow |
| Its result | Stream | A reply that must arrive at least once |
| Outside telemetry becoming graph facts | A `graph.Graphable` payload to graph-ingest | graph-ingest is the only writer of `ENTITY_STATES` |

## What the primitives give you

- Every KV bucket is stored as a JetStream stream named `KV_<bucket>` (`natsclient/backing_stream_prefix.go`), so a
  KV write is both the current value and a change that watchers see.
- A bucket keeps as many revisions per key as its `History` setting. `ENTITY_STATES` is configured to keep one
  (`graph/kvcatalog/kvcatalog.go`); do not use its history to rebuild earlier state or as a backup.
- A stream consumer acknowledges each message, and a message it does not acknowledge is delivered again
  (`natsclient/delivery_settlement_integration_test.go` drives this against the pinned server).
- How nats.go marks the end of the current values when a watch starts is not promised here: no test in this tree
  drives it (ADR-112). A design that depends on it cites a probe.
- Many local readers of one bucket share one `graphview.View` (`pkg/graphview`), never one `WatchAll` each: the view
  decodes each write once and gives every reader a snapshot followed by its changes. A reader that needs its own
  filter, history replay, every revision or its own acknowledgement still opens a real consumer.

Using both in one component, KV for state and a stream for work, is the normal shape.
