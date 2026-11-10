---
name: entity-or-bucket
description: >-
  Decide whether a fact lives in the graph, as triples on an entity in ENTITY_STATES, or in a bucket of its own.
  Use when adding durable state, proposing a new bucket, or when other code must read a fact.
---

# Entity or bucket

Start from the fact the user names. "The graph" here is `ENTITY_STATES`, the NATS key-value bucket in which
graph-ingest keeps one current record per entity: its triples, each a subject, a predicate and an object.

## The default

A fact lives in `ENTITY_STATES` as triples about an identified entity. A bucket of its own is the exception, and it
names one of the grounds below. The rubric is SemStreams ADR-049, generalized by its ADR-055; neither record is
carried into `docs/adr/`, so this skill is its home here. A new subsystem that wants its own bucket writes an ADR that
defends the choice on these grounds.

## Own a bucket only when you can name the ground

1. **Compare-and-set over several fields** that one write of an entity's triples cannot express. The bar is high:
   graph-ingest stores each entity as one entry and changes it with one revision-checked write (`graph-entity-writes`
   spec, "The revision is the only fence").
2. **Ordering that the graph's replace rule cannot give.** The bar is high: a single-valued predicate is already
   replaced in timestamp order (`graph-entity-writes` spec, "Timestamp orders a replace").
3. **Retention or layout that differs from the graph**: a longer window, or a history that is the data itself.
   `STORAGE_REPORT` keeps ten revisions because they are a growth series (`graph/kvcatalog/kvcatalog.go`).
4. **A write rate that would swamp the graph** if mixed into it.
5. **Bulky content** that does not belong in the graph. Store the content elsewhere and put a triple in the graph
   that holds its key.
6. **Bootstrap circularity**: the fact says whether the graph may be read at all, so it cannot live in the graph.
   That was SemStreams' case for `GRAPH_STATUS`, whose readiness records a consumer could wait on before it read. It
   is history here: in SemEngine the worker that answers a query says in its reply whether it can answer yet, and no
   consumer reads a bucket before it asks ([ADR-109](../../../docs/adr/109-ready-after-boot.md), Decision 1).

Ground 6 is SemStreams' addition to ADR-049's first five: `GRAPH_STATUS` fits none of them. The bucket stays, as
information for operators and tools; graph-ingest writes its record there (`processor/graph-ingest/readiness.go`).
Nothing in SemEngine waits on it, and no value in it changes what a query returns (ADR-109, Decision 3).

## Keep it in ENTITY_STATES when

1. The data is facts the graph should reason over.
2. Graph queries or inference should see it. Community records (`COMMUNITY_INDEX`) are derived from entity IDs by
   `graph/clustering` (grouping by ID prefix, not detection) and rebuilt from `ENTITY_STATES`; they are not facts.
3. Several readers need one current state through their own interfaces. That state is not an audit trail:
   `ENTITY_STATES` keeps one revision per key (`graph/kvcatalog/kvcatalog.go`).
4. You need no compare-and-set finer than graph-ingest's one write per entity.

## The test that does not work

Do not decide by "does it accumulate, or is it replaced". Accumulation or replacement follows from the ground that
applies; it is never the test. The unit is the fact, not the subject: one subject can have facts in the graph and
content in a bucket, reached through a key that a triple holds. That is not a violation.

## Who reacts to the fact

Placement decides storage; who can react to the fact is a separate question. Do not have the owner watch its own
bucket and republish the changes it thinks are interesting: the producer then decides what every consumer depends on,
where SemEngine has the consumer declare it
([ADR-088](../../../docs/adr/088-readiness-is-per-producer-aggregation-is-the-consumers.md)). A fact with no strong
bucket ground that something must react to belongs in the graph.

How rules read facts (the pin's rule processor reads only `ENTITY_STATES` and messages) returns with the rule
processor in change 6.

## Before you propose a new bucket

- [ ] Which ground, 1 to 6, does it meet? If none, the fact goes in the graph.
- [ ] Is the ground about the fact, or about a subject that also has facts in the graph?
- [ ] Does anything need to react to it? Then how does it reach that reader: as a triple, or as a published message?
- [ ] Does the framework guarantee its ownership or retention? Then it gets a descriptor in `graph/kvcatalog`, naming
      its owner, write policy and retention.
- [ ] Is the ground not obvious? Write the ADR.

## See also

- `graph/kvcatalog/kvcatalog.go`: which buckets the framework owns, and the rule for adding one.
- [kv-or-stream](../kv-or-stream/SKILL.md): a different question, whether something is a fact or a request for work.
