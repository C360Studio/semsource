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

If this processor restarted, should it re-process messages it already handled?

- **Yes** (re-process is correct recovery) --> **KV Watch**
- **No** (re-process would be wrong) --> **JetStream Stream**

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

## Common Cases Reference

| Communication | Primitive | Reason |
|--------------|-----------|--------|
| Entity state changed | KV Watch | Fact; fan-out; fast; idempotent |
| New task to execute | JetStream Stream | Request; queue; expensive; side effects |
| Index update | KV Write (others watch) | Fact; fan-out; fast |
| LLM call | JetStream Stream | Request; queue; slow; costly |
| Loop current state | KV | Fact; queryable; recoverable |
| Tool execution request | JetStream Stream | Request; queue; side effects |
| Tool result returned | JetStream Stream | Response; once; push delivery |
| Workflow trigger | JetStream Stream | Request; queue; expensive |
| Workflow execution state | KV | Fact; queryable; recoverable |
| Sensor telemetry | KV Write | Fact; latest-value semantics |

## Key Architecture Context

**The KV Twofer**: Every NATS KV bucket is backed by a JetStream stream. A single KV write gives you three interfaces simultaneously:
- **State**: `kv.Get(key)` returns current value
- **Events**: `kv.Watch(pattern)` fires on every change (fan-out)
- **History**: Replay from any revision for audit trail

**Bootstrap phase**: When a KV watcher starts, it delivers ALL current values matching the pattern, then a `nil` entry signals transition to live updates. Processors must distinguish bootstrap from live to avoid treating existing entities as "new" events on restart.

**JetStream consumers**: With `DeliverPolicy: "new"` on a durable consumer, restart resumes from last ack. No replay of already-handled messages.

**Using both is normal**: A component using KV for state AND JetStream for work items in the same process is the standard pattern (see agentic-loop).

Read `docs/concepts/03-streams-vs-kv-watches.md` for full documentation.
Read `docs/concepts/02-kv-twofer.md` for KV Twofer details.
