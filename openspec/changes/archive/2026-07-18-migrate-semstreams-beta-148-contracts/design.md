## Context

Beta.148 makes canonical predicate contracts fail closed. The reviewed SemSource inventory contains
90 invalid registered predicates and two emitted body-reference predicates with no registration.
Because old predicate identities can already exist in graph-derived JetStream/KV state, code and
persisted state have one coordinated compatibility boundary.

## Goals / Non-Goals

**Goals:**

- adopt the released beta.148 module without a replacement;
- cut all 92 vocabulary contracts over once, with declaration/production/query/fixture parity;
- type guaranteed entity relationships correctly in the three known producers;
- prove the existing binary composition still initializes; and
- rehearse a literal, minimum-deletion graph reset and canonical reseed.

**Non-Goals:**

- exact-file, lifecycle, readiness, SafeConfig membership, or ConfigManager changes;
- NATS attestation/inventory products or generalized audit tooling;
- vocabulary package reorganization, entity-ID redesign, or speculative validator replacement;
- state preservation, in-place translation, mixed writers, rollback adapters, or downstream edits.

## Decisions

### D1. Migrate one reviewed set of 92 predicate contracts

The implementation owns an exact ledger of 90 retired registered identities and their canonical
replacements, plus two canonical body-reference declarations. A contract test proves the 92 target
identities are unique, canonical, registered, and used consistently. The breaking rename updates
registrations, all producers, SemSource exact queries, and positive fixtures in one task. Negative
grammar fixtures may retain explicitly classified invalid examples; production/query code may not.

No runtime alias, lookup map, normalization, dual read, or dual write is permitted. A semantic
collision blocks implementation until distinct canonical identities are reviewed.

### D2. Use SemStreams reference semantics only for guaranteed relationships

The relationship objects emitted by `handler/git/entities.go`, `handler/video/entities.go`, and
`processor/supersession/edges.go` are canonical entity IDs and use SemStreams
`EntityReferenceDatatype`. Ordinary strings that merely resemble IDs remain literals. SemSource does
not copy SemStreams validation or redesign `entityid.*` and local validators absent a demonstrated
incompatibility.

### D3. Preserve composition and substrate ownership

The dependency is pinned to `v1.0.0-beta.148`, with no `replace`, followed by `go mod tidy`. Existing
component and payload registrations remain the intended binary inventory; an initialization and
composition smoke detects a missing or ambient registration. SemStreams remains authoritative for
predicate grammar, EntityState validation, graph buckets, indexes, and query behavior.

### D4. Treat persisted beta graph state as disposable derived state

The operational cutover is one-way:

1. stop every graph writer and verify the target NATS account;
2. capture a literal account/resource inventory and review every deletion target;
3. delete `semstreams_config`;
4. delete only observed incompatible `GRAPH`, enabled `FrameworkOwnedBuckets`,
   `ENTITY_SUFFIX_INDEX`, `GRAPH_INGEST_APPLIED_SEQ`, and `PREDICATE_CATALOG` when that catalog is
   actually observed;
5. preserve source inputs, source/content/media/object stores, component status, and unrelated state;
6. recreate configuration from the reviewed `semsource.json` through the normal startup path;
7. start only migrated writers, reseed, wait for ready, and run a canonical known-answer query.

Commands use literal account, stream, and bucket names from the captured inventory. Wildcards,
assumed framework resources, in-place rewrites, mixed writers, and graph-state preservation are not
valid cutover mechanisms. Recovery before reseed is restoring the pre-cutover deployment as a unit;
the migrated release does not interpret old graph state.

### D5. Downstream work is a handoff

The rename ledger records downstream exact-query consumers and owners. SemSource acceptance requires
the local production/query scan to be clean and the handoff to be published, but this change makes no
commit in another sem* repository.

## Risks / Trade-offs

- A missed exact-query literal would silently return no results; the old-identity production/query
  scan and known-answer query make it a release blocker.
- An over-broad reset could destroy authoritative inputs; the reviewed literal inventory and explicit
  preservation list bound deletion.
- A relationship typed by string shape could corrupt semantics; only the three guaranteed producers
  are changed.
- A transitive registration could hide a composition error; the explicit init/composition smoke
  compares the current intended inventory without redesigning it.

## Rollout Plan

Land the beta.148 pin and canonical code together, pass unit/integration and composition gates, rehearse
the cutover on disposable real NATS, then perform the stopped-writer reset and reseed. Release evidence
records readiness and the canonical known-answer query result. Downstream owners receive the exact
rename handoff before release sign-off.
