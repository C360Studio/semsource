# ingestion-readiness Specification

## Purpose
The aggregate ingestion phase, per-source phases, readiness sub-signals, and
entity counts tell the truth on every surface (NATS, HTTP, MCP): `ready` means
every configured source finished its initial seed; index/embedding readiness is
present on HTTP and MCP alike (one shared composer) with explicit
{available:false, reason} on failure; counts are distinct-entity cardinality,
invariant under periodic republication (audit 2026-07-19).

## Requirements

### Requirement: Ready means seeded

The aggregate ingestion `phase` SHALL be `ready` only when every configured source has completed
its initial seed; while any source is still seeding the phase SHALL be observably `seeding`; any
errored source SHALL yield `degraded`. The documented consumer gate (poll until `ready`) therefore
guarantees the initial corpus is fully published.

#### Scenario: Mid-seed window is observable

- **WHEN** at least one configured source has reported but not yet completed its initial seed
- **THEN** `phase` is `seeding` on every status surface

#### Scenario: Ready after the last source completes

- **WHEN** the final configured source reports initial-seed completion
- **THEN** `phase` transitions to `ready`

#### Scenario: Errored source degrades the aggregate

- **WHEN** any source reports phase `errored`
- **THEN** the aggregate phase is `degraded`, not `ready`

### Requirement: Readiness sub-signals are present and honest on every surface

The structural-index and embedding readiness objects SHALL be present on the MCP `source_status`
tool AND the HTTP `/source-manifest/status` endpoint, composed by one shared assembly. When a
readiness sub-query fails, the corresponding object SHALL state `available: false` with a reason —
the key SHALL NOT be silently omitted.

#### Scenario: HTTP parity with the MCP tool

- **WHEN** a consumer fetches HTTP `/source-manifest/status`
- **THEN** the response carries the same `index` and `embedding` readiness objects (shape and
  values) as the MCP `source_status` tool at that instant

#### Scenario: Responder failure is explicit

- **WHEN** the index or embedding status responder is unavailable
- **THEN** the corresponding object reports `available: false` and a reason, and the remainder of
  the status payload is returned normally

### Requirement: Entity counts are distinct-entity truth

`entity_count`, `type_counts`, and `total_entities` SHALL report the cardinality of distinct
confirmed entities per source, invariant under republication of unchanged entities (periodic
reindex, restarts). Throughput counters, if exposed, SHALL be separately named.

#### Scenario: Periodic reindex does not inflate counts

- **WHEN** the periodic reindex republishes unchanged folder/repo/file entities
- **THEN** `entity_count` and `total_entities` are unchanged
