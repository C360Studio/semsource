---
name: entity-or-bucket
description: Decide whether a fact lives in the graph as entity triples or in a private/operational KV bucket, and how a rule reads it. Use when adding any new durable state, proposing a new bucket, or when a fact needs to be rule-readable.
argument-hint: [description of the fact being stored]
---

# Entity or Bucket

## What fact are you storing?

$ARGUMENTS

## The default

**Live in `ENTITY_STATES` as triples about an identified entity.** A private or
operational KV bucket is an *exception* that must name its ground.

This is not a preference. It is the load-bearing rubric from
`docs/adr/049-lifecycle-harness-prime-schema-over-entity-states.md` (generalized
beyond lifecycle by ADR-055), and it carries a burden of proof: **new subsystems
wanting a private bucket file an ADR that defends the choice on the rubric.**

## Own a bucket only when you can name the ground

1. **CAS atomicity** over multi-field state that cannot be expressed as a single
   triple-batch write. *Bar: high — `AddTriplesBatch` IS atomic per-entity.*
2. **Strict-ordering replace** semantics that per-predicate latest-wins cannot
   satisfy. *Bar: high — `ENTITY_STATES` already has per-predicate latest-wins.*
3. **Retention or topology genuinely differs** from the graph — e.g. a longer
   compliance window, or a history depth that IS the data (`STORAGE_REPORT`
   History 10 exists to be a growth series, not to preserve accretion).
4. **Write rate would dominate or pollute the graph** if mixed in.
   *Example: agent trajectories — defensible.*
5. **Bulky payload** that does not belong in the graph — content to ObjectStore
   or a private bucket, with a ref-triple in the graph pointing at it.
6. **Bootstrap circularity** — the fact gates whether the graph plane may be read
   at all, so it cannot itself live on that plane. *Example: `GRAPH_STATUS`
   readiness; the rule processor's own graph evaluation is gated on it.*

Ground 6 is an addition to ADR-049's original five, recorded here because
`GRAPH_STATUS` fits none of the first five and its real justification was
unwritten.

## Live in ENTITY_STATES when

1. The data IS facts the graph should reason over.
2. You want graph queries, inference, or community detection to see it.
3. You want one queryable current state shared by graph consumers. This does
   **not** give you durable audit: `ENTITY_STATES` is History 1.
4. **Multiple consumer surfaces — rules, GraphQL, dashboards, inference — need
   the same state through their natural interface.**
5. You do not need fine-grained CAS that graph-ingest's batched write cannot
   provide.

## The test that does NOT work

**Do not classify by "does it accrete or is it replaced".** It mis-predicts.
`TOOL_CALL_OUTCOMES` and `AGENT_LOOPS`' `COMPLETE_{loopID}` records are both
write-once and identity-bearing — never wholly replaced — yet both correctly live
in buckets, on grounds 4 and 5. Accretion versus replacement is a *consequence* of
which ground applies, never the test.

**The unit is the FACT, not the subject.** One subject legitimately has facts on
both planes. An agent loop has facts *about the run* in the graph (outcome, model,
tokens, cost) and the *content the run produced* in `AGENT_LOOPS`, retrieved by a
key the graph handed you. That is not a violation — it is "rules carry references,
not payloads" working as designed.

## Consumption: what can read the fact once it is stored

Placement decides storage. Consumability is a *separate* question, and this section
records what is true today rather than prescribing a workaround — the design is
under active review.

**What rules evaluate today.** Two typed sources: authoritative `EntityState`
(decoded triples — `$entity.triple.*`, `.length`, `.triples`) and messages
(dotted-path over an arbitrary payload, `$message.*`, ADR-039).

**Why operational KV is not among them — and what the reason is NOT.** The rule
processor declares `KVWatchPort{Bucket: ENTITY_STATES}` (`processor/rule/config.go:225`)
and its evaluator is typed on `*graph.EntityState` (`processor/rule/interfaces.go:50`).
The watch pattern must satisfy `ValidateEntityIDPattern`, so a key like
`COMPLETE_7c9e6679-7425-40de-944b-e07fc1f90ae7` cannot be expressed as a watch pattern.

Three things are commonly assumed to be the obstacle and are NOT:

- **Not transport.** A KV bucket IS a JetStream stream (`KV_<bucket>`,
  `natsclient/backing_stream_prefix.go:22-23`). Every KV write is already a message
  on a stream; the bucket is not a dead end.
- **Not the port abstraction.** `component.KVWatchPort` (`component/port_kv.go:6-11`)
  is a general port type carrying `Bucket`, `Keys`, `History`, and an
  `InterfaceContract`. Six components declare one. The rule processor's is pinned by
  its own declaration, not by a framework limit.
- **Not trigger semantics.** Transition state is keyed `(rule ID, entityKey)`
  (`processor/rule/stateful_evaluator.go:136`) and `DetectTransition` is a pure
  boolean over the previous and current match, with revision-based replay protection
  at `:155`. None of that is entity-specific. A rewrite whose *classification* did
  not change yields `TransitionNone` and does not fire — which is why a condition
  should carry a classification rather than a raw measurement.

**The actual gap is a typed decoder.** A `StorageResource` or a readiness envelope is
not triples, so `$entity.triple.*` has no referent. That is precisely what
`processor/rule/entity_pattern_contract.go:17` names: *"typed operational-KV rule
adapters require a separately designed decoder and evaluator."* Work not done, not a
prohibition.

**What NOT to do while that is undecided.** Do not have the owning component watch its
own bucket and materialize a message stream of "interesting" changes. That puts the
PRODUCER in charge of predicting which transitions a consumer cares about, which is a
framework-declared threshold in disguise — the thing ADR-088 rules against
(*"declaring a key you do not depend on defers you on someone else's outage"*). It is
also strictly lossier than the report it summarizes. The consumer declaring what it
depends on is the shape this framework already uses for readiness, and it is the shape
any future lane should keep.

**Meanwhile**, placement is the lever you do control. If a fact has no strong bucket
ground and something needs to react to it, that is ADR-049's item 4 — *"multiple
consumer surfaces (rules, GraphQL, dashboards, inference) need to read the same
state"* — and it belongs in the graph as triples.

## Before you propose a new bucket

- [ ] Which numbered ground (1–6) does it meet? If none, it goes in the graph.
- [ ] Is the ground about the FACT, or about a subject that also has graph facts?
- [ ] Does anything need to react to it? If so, how does it reach the entity or
      message plane — as a triple, or as a published message?
- [ ] Is it framework-owned state? Then it needs a catalog descriptor
      (`framework-bucket-catalog`) binding owner, write policy, and retention.
- [ ] Ground 1–6 met and non-obvious? ADR-049 says file an ADR defending it.

## See also

- `docs/adr/049-lifecycle-harness-prime-schema-over-entity-states.md` — the rubric
- `openspec/specs/framework-bucket-catalog/spec.md` — ownership, retention, and
  the must-exist reader seam, once you have decided to own a bucket
- `/kv-or-stream` — a different question: fact-vs-work-request, not where state lives
