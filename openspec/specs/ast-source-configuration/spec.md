# ast-source-configuration Specification

## Purpose
The ast-source processor (`processor/ast-source`) configures every code-parsing repository
through one or more complete `watch_paths` entries (`WatchPathConfig` in `config.go`): path, org,
project, optional version, languages, and excludes. `NewComponent` (`component.go`) decodes that
JSON with `DisallowUnknownFields`, so a top-level `repo_path`, `org`, `project`, `version`,
`languages`, or `exclude_patterns` key — the six fields the legacy single-repo shape used — fails
component creation outright instead of being read through a compatibility accessor, and
`Config.Validate` refuses to start with zero watch paths. Startup, file-watch handling, and every
periodic reindex sweep walk only the configured, resolved watch paths (`ResolveWatchPaths` in
`paths.go`); no directory-index entry point outside that path remains in the component.
## Requirements
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
