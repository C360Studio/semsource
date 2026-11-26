## ADDED Requirements

### Requirement: Config-file entities go stale with their manifest
The config-file source SHALL give every entity it publishes a lifecycle owner fact and its declaring manifest's file path, and SHALL trigger the staleness pass for its root after the initial seed and on a watch delete. When a manifest is deleted, every entity it declared SHALL carry `entity.lifecycle.stale` once the pass completes, and re-creating the manifest SHALL clear the marker. A dependency removed from a manifest that still exists is retained live, without the marker.

#### Scenario: A deleted manifest's entities are marked stale
- **GIVEN** a watched config source has ingested `svc/go.mod` with one dependency
- **WHEN** `svc/go.mod` is deleted and the staleness pass completes
- **THEN** the module entity and the dependency entity both carry `entity.lifecycle.stale`

#### Scenario: Re-creating the manifest clears the marker
- **GIVEN** the entities of a deleted `svc/go.mod` carry `entity.lifecycle.stale`
- **WHEN** `svc/go.mod` is re-created and the staleness pass completes
- **THEN** none of them carries `entity.lifecycle.stale`

#### Scenario: A dependency removed from a live manifest is retained
- **GIVEN** `svc/go.mod` requires `github.com/a/b`
- **WHEN** that requirement is removed and the manifest re-ingested
- **THEN** the dependency entity remains without `entity.lifecycle.stale`, and the manifest no longer has a `requires` edge to it
