# cli-onboarding Specification

## Purpose
`semsource init` (interactive and --quick) produces configurations whose every
source actually ingests on first run — git identity derives from the origin
remote or absolute path (never "."), and ID-shaped values are validated at the
prompt with actionable guidance, so the first five minutes never end in a
silently empty graph (audit 2026-07-19).

## Requirements

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

### Requirement: Non-interactive registration can declare explicit identity

`semsource add ast` and `semsource add repo` SHALL accept `--project` and
`--version` flags that set the source entry's explicit identity — the same
`project`/`version` pair the config file documents for version diffs and
cross-registration dedup. Omitting the flags SHALL preserve automatic
derivation (path-derived for local sources, URL-derived for remote), so
existing invocations are byte-for-byte unchanged.

#### Scenario: Registering one project at two versions from the CLI

- **WHEN** `semsource add ast --path ./depA-1.9.0 --project depA --version 1.9.0`
  and `semsource add ast --path ./depA-1.10.0 --project depA --version 1.10.0`
  run against one config
- **THEN** the config gains two `ast` entries sharing `project: depA` with
  distinct `version` values — the registered shape `code_changes` diffs

#### Scenario: A remote repo with explicit identity

- **WHEN** `semsource add repo --url <url> --project <slug> --version <rev>` runs
- **THEN** the repo entry carries both fields, and expansion applies them to
  the expanded code entries exactly as a config-file declaration would

#### Scenario: Omission changes nothing

- **WHEN** `semsource add ast --path ./src --language go` runs without the
  new flags
- **THEN** the written entry has no `project` or `version` keys and identity
  derivation behaves exactly as before
