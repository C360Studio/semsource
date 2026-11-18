# source-discovery Specification

## Purpose
Discover the branches of multi-branch repo sources and the materialized submodules of repo and git
sources at boot, reconcile them into the composition before it seals (SemEngine ADR-094), and
report, without writing anything, when a restart would admit or retire any of them (#216).
## Requirements
### Requirement: Discovery takes effect at the boot that finds it

At boot, before the service manager is built, SemSource SHALL reconcile the configuration in effect so that it holds a component set for each branch that matches a multi-branch source's patterns and each materialized submodule of a repo or git source, and holds no discovery-owned component whose branch, submodule, or parent source is gone. This SHALL hold whether the file or the retained configuration won arbitration. Each boot SHALL record the component names discovery admitted per parent in the next-boot manifest, and reconciliation SHALL delete only recorded names, including those of a parent that left the configuration. It SHALL NOT re-enable a discovery-owned component that a runtime removal disabled.

#### Scenario: A branch created between boots is admitted on a retained store

- **GIVEN** a deployment that booted once with a multi-branch source matching `feature/*`
- **WHEN** branch `feature/b` is created in the repository and SemSource restarts with the same file and an unchanged `config_revision`
- **THEN** `feature/b`'s components are admitted at that boot and its entities are ingested

#### Scenario: A deleted branch leaves the composition at the next boot

- **GIVEN** a boot that admitted `feature/a`
- **WHEN** the branch is deleted and SemSource restarts
- **THEN** `feature/a`'s components are not admitted
- **AND** components that no boot recorded are unchanged, even when a branch could produce their names

#### Scenario: A materialized submodule expands at boot

- **GIVEN** a repo source whose checkout holds a materialized submodule
- **WHEN** SemSource boots
- **THEN** the submodule's files are ingested under the submodule's URL-slug project and gitlink-SHA version

#### Scenario: A runtime removal survives rediscovery

- **GIVEN** a discovery-owned branch component that a runtime removal disabled
- **WHEN** SemSource restarts and the branch still exists
- **THEN** the component stays disabled

### Requirement: Runtime discovery is read-only and reports what a restart would change

While running, SemSource SHALL periodically compare each discovering source's current branches and submodules with the set admitted at boot, and report the difference on `/source-manifest/status` and `source_status` as a `discovery` block with `restart_required` and per-source `pending` entries (`kind`, `name`, `change`). The check SHALL NOT write configuration, create worktrees, or start or stop components. `restart_required` SHALL be true exactly when a restart would change the admitted set.

#### Scenario: A branch created at run time is reported, not started

- **GIVEN** a running deployment with a multi-branch source
- **WHEN** a new matching branch is created
- **THEN** status reports `restart_required: true` with a pending `branch` `added` entry
- **AND** no component is started and no worktree is created for it

#### Scenario: Nothing changed

- **WHEN** the repository's branches and submodules match the boot-admitted set
- **THEN** status reports `restart_required: false` with no pending entries

### Requirement: Discovery failures are typed status and never fail boot

A discovery failure SHALL NOT fail boot. Each discovering source SHALL report a discovery `state`: `ok`, `awaiting_checkout` (a remote source with no checkout yet), `listing_failed` (with the error), or `capped` (with the number of matches not admitted). On `listing_failed` or `awaiting_checkout`, the previously recorded discovery-owned components SHALL be kept. Discovery SHALL NOT be reported only as a log warning.

#### Scenario: First boot of a remote multi-branch source

- **GIVEN** a remote multi-branch source whose checkout does not exist yet
- **WHEN** SemSource boots
- **THEN** boot succeeds, the clone component is admitted, and its discovery state is `awaiting_checkout`
- **AND** once the clone exists the runtime check reports the matching branches as pending

#### Scenario: A listing failure keeps admitted branches

- **GIVEN** a boot that admitted `feature/a`
- **WHEN** the next boot cannot list the repository's branches
- **THEN** `feature/a` stays admitted and the state is `listing_failed` with the error

