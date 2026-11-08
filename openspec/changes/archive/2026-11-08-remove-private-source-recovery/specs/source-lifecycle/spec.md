## MODIFIED Requirements

### Requirement: Removal is real and observable

Source removal SHALL persist the complete component envelope with `Enabled:false` and the corresponding
next-boot manifest/count change through ConfigManager. It SHALL leave the running composition unchanged
until controlled restart, preserve graph history, and report desired persistence separately from runtime
activation. Source desired changes SHALL NOT depend on graph readiness or private recovery state.
Automatic `source_removed` projection and positive reactivation SHALL be explicitly unavailable.

#### Scenario: Removed source leaves the next boot

- **WHEN** an enabled source is removed and desired writes succeed
- **THEN** the reply has `removed:true`, `desired_changed:true`, `restart_required:true`, `runtime_changed:false`
- **AND** `projection_status` is `unavailable`, with no `generation` or `projection_phase`
- **AND** the current producer remains admitted until restart, after which it is absent

#### Scenario: Degraded graph does not prevent desired disable

- **GIVEN** configuration storage is healthy and graph indexing/query is degraded
- **WHEN** a source removal is requested
- **THEN** the desired disable and manifest write can succeed without graph-tail or graph-readiness calls
- **AND** the receipt makes no graph projection completion claim

#### Scenario: Unknown or successfully removed handle fails loudly

- **WHEN** removal names an unknown or already-disabled handle with no owed manifest repair
- **THEN** the reply is `NOT_FOUND`, not `removed:true`

#### Scenario: Expanded-repo removal is instance-scoped

- **WHEN** one repo child is disabled
- **THEN** its siblings remain enabled and the next-boot manifest/count describes the remaining children

#### Scenario: Partial persistence remains truthful

- **GIVEN** a config write commits before a memory error or a subsequent manifest write fails
- **WHEN** the request returns
- **THEN** authoritative retained config determines committed desired flags and the error is retained
- **AND** request retry repairs the incomplete manifest without claiming live activation

#### Scenario: Re-add is desired-only

- **WHEN** a disabled source is re-added successfully
- **THEN** its desired envelope is enabled and restart is required
- **AND** `projection_status:unavailable` does not promise to clear retained source-removal markers

### Requirement: The manifest mirrors the running set

Runtime manifest/status SHALL describe the admitted running source set. Desired configuration changes
SHALL update the next-boot manifest without claiming that additions/removals already changed runtime.

#### Scenario: Add/remove sequence activates on restart

- **WHEN** desired source changes complete while the application is running
- **THEN** runtime source membership stays unchanged until controlled restart
- **AND** after restart the admitted set matches retained enabled configuration, including file-defined tombstones

## ADDED Requirements

### Requirement: Retired projection surfaces are explicitly unavailable

The lifecycle status route SHALL remain authenticated and return HTTP 410 with error code
`SOURCE_LIFECYCLE_UNAVAILABLE`, without reading or exposing private lifecycle state.
`graph.lifecycle.source` and legacy lifecycle requests with reason `source_removed` SHALL refuse effects.

#### Scenario: Client requests retired lifecycle status

- **WHEN** an authorized client requests `/sources/{id}/lifecycle`
- **THEN** it receives HTTP 410 and the unavailable error, not a pending phase or a missing-intent lookup

#### Scenario: Client invokes source-removal projection

- **WHEN** either retired source-projection RPC path is invoked
- **THEN** the operation explicitly refuses and performs no graph mutation
