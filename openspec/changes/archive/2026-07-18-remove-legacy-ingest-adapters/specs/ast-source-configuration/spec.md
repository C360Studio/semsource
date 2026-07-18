## ADDED Requirements

### Requirement: AST sources use only watch paths

The AST source component MUST configure sources through one or more `watch_paths` entries and MUST
strictly reject top-level `repo_path`, `org`, `project`, `version`, `languages`, and
`exclude_patterns`. It MUST NOT translate those fields through a compatibility accessor.

#### Scenario: AST sources are configured

- **WHEN** a composition configures one or more repositories
- **THEN** each source is a complete validated `watch_paths` entry
- **AND** runtime uses the entries without precedence or conversion logic

#### Scenario: A removed AST key is supplied

- **WHEN** strict component decoding encounters any of the six removed top-level keys
- **THEN** component creation fails as invalid configuration
- **AND** the key is not ignored, defaulted, or translated

#### Scenario: No watch path is supplied

- **WHEN** a runnable AST component has no `watch_paths` entry
- **THEN** validation fails before start
- **AND** no implicit current-directory source is invented

### Requirement: AST indexing exposes only active entry points

Initial and watched AST indexing MUST use the component's registered language-aware paths. The
zero-caller `Watcher.IndexDirectory` compatibility API MUST NOT remain.

#### Scenario: Initial indexing starts

- **WHEN** the AST component starts with canonical watch paths
- **THEN** each configured language uses its registered parser
- **AND** no compatibility directory-index method is called
