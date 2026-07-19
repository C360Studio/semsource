# Delta: cli-onboarding

## ADDED Requirements

### Requirement: Default init produces sources that actually ingest

`semsource init` and `semsource init --quick` SHALL produce a configuration in which every
generated source yields valid entity identity by construction — in particular, the git source for
the current repository SHALL carry a resolvable identity (remote-derived slug, or path-derived
slug when no remote exists; never `"."`), so the default install ingests real git history.

#### Scenario: Init in a repo with an origin remote

- **WHEN** `semsource init --quick` runs in a git repo with an `origin` remote and the produced
  config is run
- **THEN** commit entities from that repo land in the graph and appear in source status counts

#### Scenario: Init in a repo without remotes

- **WHEN** `semsource init --quick` runs in a git repo with no remotes
- **THEN** the git source identity derives from the repository path (non-empty valid segment) and
  commit entities land

### Requirement: Onboarding failures are actionable

When an init or validation input would produce invalid entity identity, the CLI SHALL fail at that
surface with a message naming the field, the offending value, and the allowed segment alphabet —
never accept a value that is known to fail at publish time.

#### Scenario: Invalid namespace rejected at init

- **WHEN** the user enters a namespace containing a dot or space in the wizard
- **THEN** the wizard rejects it immediately, naming the allowed alphabet
