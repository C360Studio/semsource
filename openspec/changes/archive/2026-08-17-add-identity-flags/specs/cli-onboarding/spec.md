# cli-onboarding Delta

## ADDED Requirements

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
