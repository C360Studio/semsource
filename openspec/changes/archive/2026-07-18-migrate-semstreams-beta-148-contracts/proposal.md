## Why

SemSource cannot adopt SemStreams `v1.0.0-beta.148` with a dependency-only bump. Ninety registered
SemSource predicates violate the beta.148 canonical grammar, and two emitted body-reference
predicates are not registered. Beta.148 rejects those contracts, so registrations, producers, exact
queries, and positive fixtures must move together. Persisted beta graph state containing the old
identities is intentionally incompatible and must be rebuilt from authoritative source inputs.

## What Changes

- Pin `github.com/c360studio/semstreams v1.0.0-beta.148`, remove any local replacement, and tidy the
  module graph.
- **BREAKING**: replace the 90 invalid registered identities and register the two emitted
  body-reference predicates as one 92-predicate canonical vocabulary cutover.
- Update registrations, producers, SemSource exact-predicate queries, and positive fixtures
  atomically; ship no aliases, runtime rename maps, dual reads, or dual writes.
- Mark guaranteed entity relationships with SemStreams `EntityReferenceDatatype` in git entities,
  video entities, and supersession edges.
- Preserve SemStreams as the authority for predicate/entity validation and graph state. Preserve the
  current component and payload registration inventory.
- **BREAKING**: require a stopped-writer, literal-inventory, destructive graph-state cutover followed
  by reviewed configuration recreation, reseed, readiness, and a canonical known-answer query.
- Record downstream exact-query impacts for handoff; do not edit downstream repositories.

## Non-goals

- Preserving or rewriting incompatible graph state, supporting mixed-version writers, or adding a
  compatibility path.
- Redesigning entity IDs, local validators, graph lifecycle, source readiness, SafeConfig membership,
  ConfigManager behavior, or component/payload composition.
- Fixing the exact-file bug; adding a NATS attestation/inventory subsystem or generalized audit
  framework; reorganizing vocabulary packages; or replacing local validators without demonstrated
  beta.148 incompatibility.
- Editing downstream sem* products. SemSpec, SemDragon, SemOps, SemTeams, and workbench owners receive
  a handoff only.

## Consumers

SemSource handlers, supersession, fusion, MCP, and workbench/query paths consume the canonical
vocabulary. SemSpec, SemDragon, SemOps, and other downstream exact-query consumers receive the rename
handoff. SemStreams continues to own the governed substrate.

## Capabilities

### New Capabilities

- `source-vocabulary-contract`: the canonical 92-predicate migration set and its registration,
  production, query, fixture, and reference-datatype invariants.

### Modified Capabilities

- `semstreams-governance`: advances the released dependency target and defines the one-way beta graph
  state cutover.

## Impact

The change affects the Go module pin, predicate declarations and producers, SemSource exact queries,
positive fixtures, three entity-reference producers, operational cutover instructions, real-NATS
acceptance evidence, and downstream handoff documentation. Canonical source inputs and non-graph
stores remain the recovery authority.
