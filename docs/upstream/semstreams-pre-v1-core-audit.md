# SemStreams pre-v1 core audit

Date: 2026-07-21

Status: proposed release program

Reviewed baseline: `C360Studio/semstreams` `main` at `a16a1b42`, plus the open issue queue through #609

Review basis: issue and PR triage, architecture and Go-path review, configuration-use search, and focused race tests
over clustering, embedding, graph-index, graph-query, and object storage. The focused race suite passed, which is
itself evidence that the missing contracts are outside the present test oracles.

## Decision summary

SemStreams should not answer the findings by adding another coordination layer. The highest-risk failures come from
state that looks healthy while its meaning has already diverged:

- a permanent semantic index can point at body content that expired after 24 hours;
- community partition, summaries, and enhancements share records but have different writers and lifecycles;
- readiness is often evidence that a watcher once caught up, not that the current result is usable;
- several configuration fields and tests describe behavior that production code does not implement.

The pre-v1 program should restore a few small invariants, delete misleading surfaces, and make the end-to-end
consumer path the release oracle. Passing package tests is not sufficient while tests can pass without exercising
the advertised behavior.

The recommended order is:

1. Make retained evidence and hydration honest.
2. Collapse communities to one real partition level and one owner per field.
3. Replace opaque lifecycle retention with explicit ownership and cleanup.
4. Make critical tests falsifiable and consumer-facing.

## Non-negotiable invariants

These are the release criteria against which individual issues should be judged.

1. A durable graph reference does not outlive the bytes it names.
2. A green readiness signal means the requested consumer result is usable now.
3. One logical field has one writer. Long-running enrichment never rewrites a newer partition snapshot.
4. A configuration option changes observable behavior, or it does not exist.
5. A critical-path test fails when its advertised behavior is removed.
6. Derived state has an owner and an explicit replacement or deletion rule.
7. Graph lifecycle is not controlled by an opaque NATS TTL or `MaxBytes` setting.

## Finding 1: embedding health stays green while current truth diverges

The three embedding buckets have different realities:

| Bucket | Key | Production behavior | Operational meaning |
| --- | --- | --- | --- |
| `EMBEDDING_INDEX` | entity ID | Written and watched by the in-memory vector cache | Authoritative vector lookup |
| `EMBEDDING_DEDUP` | content hash | Written | Embedding reuse/deduplication |
| `EMBEDDINGS_CACHE` | none | Created and required by validation, never written | No health meaning; empty by design |

`EMBEDDINGS_CACHE` is therefore a decoy, not a failed cache. The component creates it in
`processor/graph-embedding/component.go`, stores the handle, and never uses that handle for a write. The real vector
storage is constructed later from `EMBEDDING_INDEX` and `EMBEDDING_DEDUP`. Documentation and configuration still
claim that `EMBEDDINGS_CACHE` stores or caches entity vectors.

The dangerous failure is elsewhere. `storage/objectstore/store.go` creates the body store with a hard-coded
24-hour TTL while vectors are durable. After 24 hours semantic recall, rank, `embedding.ready`, and the indexed
revision can all remain correct while fusion returns ranked nodes with empty bodies. `pkg/fusion.Engine.nodeFor`
intentionally swallows body-reference and body-read failures. The existing `Response.Unhydrated` contract describes
missing entity-state seeds, not missing node bodies.

This is a Goodhart trap: every obvious embedding health signal stays green after the user-visible result has
degraded.

The durable vector path has additional reconciliation failures:

- an entity tombstone advances readiness but does not delete its vector;
- updating an entity so it has no embeddable text leaves the previous vector searchable;
- multiple embedding workers can finish revisions of one entity out of order, allowing an old vector to overwrite a
  newer result while retaining the newer content hash;
- dedup identity is not a complete embedding identity: inline input hashes extracted text, storage-reference input
  hashes the storage key rather than fetched bytes, and the model/extraction fingerprint is absent;
- a dedup hit can therefore relabel a cached vector with the current model even when another model produced it.

`EMBEDDING_DEDUP` is a real store, unlike `EMBEDDINGS_CACHE`, but “written” is not the same as “safe.” Its key
must identify the final normalized input and stable model/extraction contract. If that identity is not made exact,
removing dedup before v1 is safer and simpler than preserving a cross-model correctness hazard.

### Required change

Before v1:

- extend the no-lifecycle-retention boot guard to body ObjectStores referenced by live entities;
- default body retention to persistent and reject an effective TTL or `MaxBytes` lifecycle policy at boot;
- surface body hydration outcome separately from entity-state hydration, with a bounded reason set and metric;
- make tombstone and no-text updates delete the entity vector before their revision is reported complete;
- serialize generation per entity or conditionally commit only the latest pending revision;
- remove `EMBEDDING_DEDUP`, or key it by normalized input plus a stable model/extraction fingerprint;
- define one canonical input as normalized inline identity/title/signature plus referenced body;
- hash after canonical composition and the explicit size policy, and read `limit+1` so truncation is observable;
- remove `EMBEDDINGS_CACHE`, its required output port, `BucketEmbeddingsCache`, and its false documentation;
- remove `cache_ttl`; do not keep an inert compatibility knob before v1;
- document `EMBEDDING_INDEX` and, only if retained, `EMBEDDING_DEDUP` as the real persistence buckets.

Removing the 24-hour TTL prevents silent loss but does not solve reclamation. Reclamation should follow live
reference ownership, not another timer: persistent by default, then explicit reference-aware deletion when the
owning entity is replaced or retired.

Primary issue: [#600](https://github.com/C360Studio/semstreams/issues/600). Related body-quality issues:
[#601](https://github.com/C360Studio/semstreams/issues/601) and
[#602](https://github.com/C360Studio/semstreams/issues/602).

## Finding 2: community storage combines unrelated ownership domains

The community subsystem currently presents three capabilities that the implementation does not separate:

- label propagation owns membership and partition identity;
- statistical summarization owns derived descriptions;
- LLM enhancement owns optional enriched descriptions.

All write complete community records to `COMMUNITY_INDEX`. Detection, summary transfer, enhancement success, and
enhancement failure can overwrite one another without compare-and-set protection. A slow enhancement can write an
old membership snapshot after a new detection cycle. The enhancement watcher also requeues work produced by the
system's own writes.

The advertised hierarchy is not a hierarchy. The detector reruns LPA over the same full entity set three times;
there are no parent communities or supernodes. Community IDs are seed entity IDs, so the same ID can occur at
multiple levels. The semantic tier changes summarization, not partitioning: no embeddings enter LPA. Base edge
weights are always `1.0`, which also makes configured sibling and system-peer weights ineffective at the component
boundary.

This is one root problem expressed by [#606](https://github.com/C360Studio/semstreams/issues/606),
[#607](https://github.com/C360Studio/semstreams/issues/607),
[#608](https://github.com/C360Studio/semstreams/issues/608), and
[#609](https://github.com/C360Studio/semstreams/issues/609): shared mutable state is standing in for a data model.

### Required simplification

Before v1:

- run and serve level `0` only;
- remove the other two pseudo-hierarchy levels and their collision surface;
- define `COMMUNITY_INDEX` as partition-owned state: membership, level, partition revision, and statistical facts;
- stop LLM enhancement from rewriting partition-owned fields;
- either store summaries separately by exact membership hash or disable LLM enhancement until that separation lands;
- run the first detection immediately after readiness instead of waiting for the first interval tick;
- model cache state as current lifecycle state, not a one-way `ready=true` latch;
- replace the bespoke community and embedding watchers with the already-shipped `pkg/graphview` lifecycle;
- recover when the LLM endpoint is unavailable at startup rather than disabling enhancement for the process lifetime;
- remove the unused relationship-inference path and any configuration used only by that path.

An exact membership hash is preferable to the current `0.8` summary-transfer threshold. It provides deterministic
reuse and lets the threshold, archive dance, and approximate identity rules disappear. If real hierarchy becomes a
demonstrated product requirement, design it later as a distinct capability with explicit parent IDs. Do not preserve
three levels merely for compatibility with behavior that never existed.

## Finding 3: configuration contains inert promises

The audit found configuration whose schema, defaults, and tests exist without a production behavior:

| Surface | Promise | Reality | Recommendation |
| --- | --- | --- | --- |
| embedding `cache_ttl` | vector retention | no consumer | remove |
| embedding `batch_size` | batching | becomes `batch_size / 10` workers | use validated workers |
| clustering `batch_size` | event threshold | timer-driven | remove |
| clustering `min_community_size` | formed-community floor | not sent to detector | remove |
| enhancement workers | positive count | negative can start none | validate or remove |
| clustering levels | hierarchy depth | three repeated LPA passes | keep level `0` |
| sibling/system weight | topology weighting | base weight is always `1.0` | remove for v1 |
| graph-index `batch_size` | batched writes | no consumer | remove |
| graph-index outputs | internal layout | unknown buckets created, then ignored | use constants |
| duration strings | strict durations | malformed values can default | reject malformed |

The clean pre-v1 compatibility policy is removal. Keeping a deprecated field that is still accepted but ignored
preserves the footgun. If a removed field must be recognized for migration, validation should reject it with a
specific upgrade message.

## Finding 4: passing tests do not distinguish working behavior

Several tests optimize for component activity or stored artifacts rather than the contract named by the test:

- `TestIntegration_ClusteringMinSize` ends with `communityCount >= 0`, which cannot fail for an integer count;
- the hierarchical LPA test permits equal counts at successive levels and never asserts parent/coarsening identity;
- the statistical e2e scenario turns failed GraphRAG queries and insufficient communities into warnings;
- that scenario queries level `1`, not the primary level `0` path;
- community structure checks read `COMMUNITY_INDEX` directly and bypass the query cache being validated;
- current embedding tests assert that `EMBEDDINGS_CACHE` is configured, not that a consumer can retrieve evidence;
- fresh-stack tests do not cross the 24-hour body-retention boundary;
- fusion lacks an end-to-end path covering semantic rank, entity hydration, and body hydration together.

This explains how a green suite can coexist with missing behavior. The correction is not a higher coverage target;
it is a stronger oracle.

### Pre-v1 verification matrix

| Contract | Setup | Required result | Failing mutation |
| --- | --- | --- | --- |
| evidence lifetime | accelerated TTL/clock | readable, or boot rejects policy | restore 24h TTL |
| usable semantic hit | vector exists; body missing | explicit partial result and metric | swallow body error |
| real persistence | ingest; semantic query | ranked entity and body | stop index write |
| deletion reconcile | delete or remove all text | absent before ready | retain old vector |
| monotonic revision | block A; finish B; release A | only B remains | blind-write A |
| exact dedup | change model/extraction | no old vector reuse | omit fingerprint |
| complete input | offloaded body plus title | both rank; truncation reported | choose one source |
| minimum size | graph below/above threshold | exact presence/absence | ignore threshold |
| one partition | two changed-topology cycles | new membership wins | save stale summary |
| cache lifecycle | close/recreate watcher | unavailable, then recovered | latch ready |
| enhancement recovery | LLM down, then healthy | statistics serve; LLM resumes | startup disable |
| primary GraphRAG | level `0` via gateway/cache | expected result or hard failure | direct KV bypass |
| fusion consistency | ranked ID lacks entity state | `Unhydrated` plus metric | silently omit |
| retained indexes | update/delete/re-add | no stale owner rows | skip cleanup |

For critical tests, warnings are failures. Tests must use the public consumer path unless the test explicitly targets
a storage primitive. Each behavioral configuration test should demonstrate that changing the value changes the
result; schema/default assertions alone do not establish a feature.

Primary gaps: [#599](https://github.com/C360Studio/semstreams/issues/599),
[#597](https://github.com/C360Studio/semstreams/issues/597), and #606-#609.

## Finding 5: the default BM25 path is stateful but not a stable index

The current BM25 embedder mutates corpus statistics while generating both documents and queries. A document vector is
calculated before the new document updates those statistics; older vectors are not recomputed as document frequency
changes. Restart can restore permanent dedup vectors without restoring the corpus statistics that produced them.
Ingestion order and queries can therefore change future vector meaning.

This is neither a deterministic lexical BM25 index nor a stable stateless embedding. Before v1, choose one:

- implement BM25 as an explicit lexical index/ranker whose queries read an immutable, revisioned corpus snapshot; or
- replace the default with a stateless hashed term-frequency vector and describe it honestly.

Do not add synchronization around the current hybrid and call it fixed. The semantic contract has to be deterministic
across ingestion order, query traffic, and restart. Add a known-answer restart/order test before retaining BM25 as a
supported v1 tier.

## Finding 6: retention design and current truth have drifted

The current graph-index work substantially improved owner-keyed replacement for incoming, predicate, name, context,
and outgoing indexes. Two design artifacts still disagree about the next step:

- ADR-068 describes a central reverse index and sweeper;
- ADR-073 replaces that with per-owner durable reverse knowledge and owner-reactive cleanup;
- `openspec/specs/graph-retention/spec.md` still refers to a later per-entity reverse index and covers KV guardrails,
  but not the `CONTENT` ObjectStore or all reachability-bearing derived stores.

Both ADRs remain proposed/design-only. The repository should accept one simple ownership model and update the
current-truth spec before implementing more machinery. ADR-073's owner-local model better matches existing Go and
NATS KV idioms: literal owner keys, idempotent replacement, bounded cleanup, and no central scan as the primary
correctness mechanism.

The current implementation still calls out `ALIAS_INDEX` as outside replacement and deletion because it has no
owner-complete axis. Alias rename or entity retirement can therefore retain stale aliases. Embedding index/dedup
entries and referenced bodies also need explicit ownership decisions. The old ADR inventories should be re-baselined
against current `main`; several index families changed in the graph-index hardening work.

### Required retention increment

1. Record a current owner/replace/delete ledger for every durable graph artifact.
2. Use one create/open-and-assert helper for all live graph KV buckets and referenced body ObjectStores; inspect
   existing stores, not only requested creation defaults.
3. Add owner-complete cleanup for aliases, embedding rows, and bodies where product operations can replace or delete
   their owners.
4. Keep a sweeper as repair/backstop only if measurements show it is needed.
5. Do not add distributed transactions or a global snapshot facade. Expose bounded partial-state reasons instead.

Related issues: [#527](https://github.com/C360Studio/semstreams/issues/527),
[#525](https://github.com/C360Studio/semstreams/issues/525), and
[#526](https://github.com/C360Studio/semstreams/issues/526). The latter two are explicitly measurement-gated and
should be closed without implementation if production evidence does not justify them.

## Finding 7: SemSource still exposes the same persistence footguns

The framework cleanup has a product-side adoption wave in this repository:

- `cmd/semsource/run.go` still declares `EMBEDDINGS_CACHE` as the graph-embedding output;
- tier docs and ADRs tell operators that vectors and first-index progress live in that bucket;
- the beta-148 cutover test treats the dead bucket as meaningful state;
- `config.Streams` publicly exposes raw `storage`, `max_age`, `max_bytes`, and `replicas` settings;
- SemSource defaults its `GRAPH` ingest stream to memory storage, 256 MiB, and a one-hour age limit.

`GRAPH` is an ingest buffer rather than the materialized live graph, but silent eviction can still discard facts
before graph-ingest persists them. Raw JetStream retention mechanics should not be product configuration. Replace the
map of opaque overrides with one SemSource-owned durable posture. If an ephemeral development posture is needed, make
it a named mode with explicit data-loss semantics, not an arbitrary mix of age, bytes, and storage knobs.

The SemSource adoption change should:

1. remove the dead embedding output from generated component config, docs, tests, and operator instructions;
2. remove `StreamOverride.MaxAge` and `MaxBytes`, preferably the entire generic `streams` map;
3. use one safe fixed `GRAPH` stream contract and expose backlog/pressure rather than silently dropping input;
4. strictly reject retired configuration keys during the pre-v1 break;
5. correct stale issue status in `docs/upstream/semstreams-asks.md`, including closed/subsumed #433.

## Issue disposition

The queue should be narrowed as part of the program rather than treated as an additive backlog:

- #600, #601, #602, and #599 are release blockers under Epic A/D;
- #597's ranking/order work was largely improved by PR #604; retain only the explicit cross-store residual;
- #606 should become the community decision epic;
- #607 can close as subsumed if enhancement is disabled or partition/summary ownership is split;
- #608 should shrink after the one-level and no-dead-inference decisions;
- #609's `(level, ID)` cache-key item is shipped; move remaining lifecycle work under #588;
- #527 is partly obsolete after ADR-077 and current replacement work; retain alias ownership, deletion policy, blob
  reachability, and pre-v1 cleanup only;
- #579's base `graphview` primitive shipped in PR #585; close it and track the two proven migrations in #588;
- #525 and #526 remain measurement gates, not architecture commitments;
- #603 is useful product work but should not dilute the semantic-core release gates.

Before v1, wipe and reseed derived indexes from authoritative state. Do not build dual-read compatibility for
pre-v1 index formats.

## Proposed epic structure

### Epic A: graph evidence cannot silently expire

Release priority: P0

Issues: #600, #601, #602, #597, #599

Deliverables:

- body-store lifecycle guardrail;
- persistent-by-default content storage;
- explicit body-hydration outcome and metrics;
- vector reconciliation on tombstone/no-text and revision-safe commits;
- an exact dedup fingerprint, or removal of dedup;
- canonical title/signature/body input with explicit truncation evidence;
- a deterministic decision for the BM25 tier;
- end-to-end semantic-to-body test;
- removal of `EMBEDDINGS_CACHE` and `cache_ttl`;
- documentation and operational checks that name `EMBEDDING_INDEX` and any retained dedup store accurately.

Exit criterion: an indexed result never looks fully healthy while returning a missing retained body without an
explicit partial-state signal, and no stale or cross-model vector remains searchable after its owner changes.

### Epic B: one community truth

Release priority: P0

Issues: #606, #607, #608, #609

Deliverables:

- level-0-only partition;
- partition-owned records with monotonic revision or compare-and-set protection;
- separately owned summaries, or statistical-only operation until that split is available;
- immediate first detection;
- `pkg/graphview`-owned lifecycle for the shared community and vector views;
- recoverable LLM lifecycle, if retained, and non-latching cache availability;
- removal of dead inference paths and inert community configuration;
- two-cycle, consumer-path, and fault-injection tests.

Exit criterion: a slow or failed enhancer cannot change membership, and a consumer cannot observe ready while its
community view is absent or stale after a dependency transition.

### Epic C: derived-state ownership and retention

Release priority: P1, except the P0 body-store guardrail in Epic A

Issues: #527; follow-ups from the owner ledger

Deliverables:

- accepted retention decision and synchronized OpenSpec truth;
- artifact owner/replace/delete ledger;
- alias, embedding, and body cleanup increments;
- backstop repair only where owner-reactive cleanup is insufficient.

Exit criterion: every persisted projection names its owner and can be replaced or retired without leaving a live
false result.

### Epic D: consumer-path release gates

Release priority: P0

Issues: #599 plus verification work from #600 and #606-#609

Deliverables:

- the verification matrix above in CI;
- hard assertions in place of warnings for v1 contracts;
- time-horizon, restart, dependency-flap, and concurrent-cycle coverage;
- a config-use audit that rejects inert knobs;
- an operator health checklist based on consumer results, not bucket names alone.

Exit criterion: deliberately removing each critical behavior makes at least one release test fail for the intended
reason.

### Epic E: SemSource clean-cut adoption

Release priority: P0

Repository: `C360Studio/semsource`

Deliverables:

- remove `EMBEDDINGS_CACHE` wiring and all operational claims about it;
- replace generic stream overrides with the fixed safe ingest-stream contract;
- add backlog/pressure observability and a no-silent-ingest-loss test;
- reject removed pre-v1 config keys;
- update upstream-ask status and the tier/cutover documentation.

Exit criterion: a SemSource operator cannot configure lifecycle loss through opaque NATS settings or use a dead
bucket as evidence of embedding health.

## Sequencing and dependencies

```text
CONTENT guardrail + hydration signal
                |
                +--> semantic-to-body e2e gate

level-0-only partition
        |
        +--> partition/summary ownership split
                    |
                    +--> enhancement recovery and concurrency gates

current artifact ledger
        |
        +--> retention spec decision --> owner-local cleanup increments

upstream embedding/config clean cut
        |
        +--> SemSource generated config and operator-doc adoption
```

Shared watcher consolidation ([#588](https://github.com/C360Studio/semstreams/issues/588)) is worthwhile after the
community ownership contract is stable. It should consolidate proven duplicate watchers for `COMMUNITY_INDEX` and
`EMBEDDING_INDEX`, not become a generic cache framework. The existing `pkg/graphview` direction from
[#579](https://github.com/C360Studio/semstreams/issues/579) is sufficient.

## Delete instead of build

The following removals reduce both code and misleading operational surface:

- `EMBEDDINGS_CACHE` and embedding `cache_ttl`;
- `EMBEDDING_DEDUP` unless its full input/model identity is made exact;
- stateful query-mutating BM25 embedding, in favor of one explicit lexical or stateless contract;
- unused embedding `NATSCache` and `ContentFields` surfaces;
- community levels `1` and `2` until real hierarchy is required;
- clustering `batch_size` and unused `min_community_size` configuration;
- dead `InferRelationshipsFromCommunities` production surface;
- approximate summary-transfer threshold once summaries use exact membership identity;
- docs that claim embeddings participate in community partitioning;
- measurement-gated optimizations #525/#526 when measurements do not support them.

## Explicit non-goals

- no distributed transaction layer across NATS stores;
- no new central garbage collector as the primary lifecycle mechanism;
- no preservation of fake hierarchy for compatibility;
- no generic cache abstraction before the two proven duplicate-watch cases are closed;
- no SemSource-local repair of framework-owned graph persistence behavior;
- no new configuration option without a behavior test that distinguishes at least two values.

## New issue candidates revealed by code review

These gaps are not cleanly represented by the current queue and should be filed under the epics rather than as a new
independent program:

| Epic | Candidate issue | Priority |
| --- | --- | --- |
| A | embedding tombstones and no-text updates retain stale vectors | P0 |
| A | out-of-order embedding revisions can commit stale vectors under a newer hash | P0 |
| A | dedup identity omits model/extraction fingerprint and hashes StorageRef key, not bytes | P0 |
| A | choose a deterministic v1 contract for the stateful BM25 tier | P0 decision |
| A | remove unwritten `EMBEDDINGS_CACHE` and inert `cache_ttl` | P0 cleanup |
| B | make LPA ordering and tie-breaking deterministic | P1 |
| E | remove SemSource raw stream retention overrides and adopt a safe fixed ingest contract | P0 |

## Release recommendation

Treat Epics A, B, D, and E as pre-v1 gates. Start Epic C's decision and inventory before v1, with implementation
gated by which replace/delete operations v1 actually exposes. This keeps the sweep rigorous without turning it into
an unbounded rewrite: repair silent evidence loss, make communities single-owner and single-level, force tests to
observe the consumer contract, remove product-level retention footguns, then add only the retention machinery the
supported lifecycle requires.
