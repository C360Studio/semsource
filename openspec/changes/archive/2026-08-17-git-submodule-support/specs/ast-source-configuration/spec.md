# ast-source-configuration Delta

## MODIFIED Requirements

### Requirement: AST sources use only watch paths

The AST source component MUST configure sources through one or more `watch_paths` entries and MUST
strictly reject top-level `repo_path`, `org`, `project`, `version`, `languages`, and
`exclude_patterns`. It MUST NOT translate those fields through a compatibility accessor.

The single sanctioned derivation is submodule expansion: when a configured
watch path's repository declares git submodules, resolution MAY derive
additional scoped watch entries from `.gitmodules` (each carrying the
submodule's own project and gitlink-SHA version, per the
git-submodule-ingestion capability). Derived entries are system-generated —
never authored config keys, never a translation of the removed legacy shape —
and the authored entry continues to govern the parent tree minus the submodule
directories.

#### Scenario: AST sources are configured

- **WHEN** a composition configures one or more repositories
- **THEN** each source is a complete validated `watch_paths` entry
- **AND** runtime uses the entries without precedence logic, the only
  sanctioned derivation being submodule expansion of a configured entry

#### Scenario: A removed AST key is supplied

- **WHEN** strict component decoding encounters any of the six removed top-level keys
- **THEN** component creation fails as invalid configuration
- **AND** the key is not ignored, defaulted, or translated

#### Scenario: No watch path is supplied

- **WHEN** a runnable AST component has no `watch_paths` entry
- **THEN** validation fails before start
- **AND** no implicit current-directory source is invented

#### Scenario: A configured repository declares submodules

- **WHEN** a configured watch path's repository declares git submodules with
  materialized trees
- **THEN** resolution yields one derived scoped entry per submodule (own
  project, gitlink-SHA version) alongside the authored entry, which excludes
  the submodule directories
- **AND** the derived entries are observable on the source status surfaces,
  not silent internal state
