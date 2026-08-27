---
name: kv-or-stream
description: Decide between KV Watch and JetStream Stream for a new communication path. Use when designing inter-component communication, adding new message flows, or choosing storage primitives.
argument-hint: [description of the communication being designed]
---

# KV Watch vs JetStream Stream Decision

## What are you designing?

$ARGUMENTS

## The 4-Test Heuristic

Apply these tests in order. The first clear answer is usually sufficient.

### Test 1: Restart Test (sharpest)

If this processor restarted, should it rehydrate current facts or resume
unacknowledged work?

- **Rehydrate current facts** --> **KV Watch**
- **Resume queued work without repeating acknowledged work** --> **JetStream Stream**

### Test 2: Fan-out vs Queue

Should multiple processors all react to this, or should only one handle it?

- **All react** (fan-out) --> **KV Watch**
- **Only one handles it** (queue) --> **JetStream Stream**

### Test 3: Processing Time

Is the processing fast and idempotent, or slow with real side effects?

- **Fast and idempotent** --> **KV Watch**
- **Slow or has side effects** --> **JetStream Stream**

### Test 4: Nature Test

Is this a fact about the world, or a request to do something?

- **Fact** (entity state, index entry, current status) --> **KV Watch**
- **Request** (execute task, call LLM, run tool) --> **JetStream Stream**

## Conflict Check

If any test gives conflicting answers, the concept may be two things conflated. Revisit whether it should be split into separate concerns.

## Mandatory KV Watch Owner Rule

KV Watch has no processing acknowledgment or redelivery. A derived owner that
chooses it must use idempotent desired-state apply, explicit failed-work
repair/redrive, and visible readiness/degradation. Bootstrap hydrates restart
inputs; it is not continuous retry. If the owner cannot accept those obligations,
redesign the seam. Never treat a KV watch as a work queue.

## Common Cases Reference

| Communication | Primitive | Reason |
|--------------|-----------|--------|
| Entity state changed | KV Watch | Current fact; fan-out; idempotent reaction |
| New task to execute | JetStream Stream | Request; queue; expensive; side effects |
| Index update | Owner KV write; admitted observers react | Only declared owner writes |
| LLM call | JetStream Stream | Request; queue; slow; costly |
| Loop current state | KV | Fact; queryable; recoverable |
| Tool execution request | JetStream Stream | Request; queue; side effects |
| Tool result returned | JetStream Stream | Response; at-least-once delivery |
| External telemetry | Graphable or typed ingest | graph-ingest owns authority write |

## Key Architecture Context

**The KV twofer**: every NATS KV bucket is backed by a JetStream stream. A
single KV write provides current state and change notification. Retained history
is optional and bounded per bucket; it is not automatically an audit ledger.

- **State**: `kv.Get(key)` returns current value
- **Events**: `kv.Watch(pattern)` fires on every change (fan-out)
- **History**: only revisions retained by that bucket's configured history depth

`ENTITY_STATES` has history 1. Its watchers bootstrap from current values, but
its history cannot reconstruct prior authority or serve as disaster recovery.

**Bootstrap phase**: when a KV watcher starts, it delivers all current matching
values, then a `nil` entry signals transition to live updates. This is current
state hydration, not replay of every historical change.

**JetStream consumers**: durable state tracks acknowledgments. Restart can
redeliver unacknowledged work; acknowledged work is not replayed.

**Using both is normal**: A component using KV for state AND JetStream for work items in the same process is the standard pattern (see agentic-loop).

Read `docs/concepts/03-streams-vs-kv-watches.md` for full documentation.
Read `docs/concepts/02-kv-twofer.md` for KV Twofer details.
