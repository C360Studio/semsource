# Resolve the git token from the environment, never persist it

Approved-by: Coby Leuschke (2026-11-18) sha256:746457fa7516

Track: design (adds spec Requirements and touches the boot path).

## Why

#272: the git token is written to two places it must never be.

- **The config bucket.** `SEMSOURCE_GIT_TOKEN`, or `git_token` in `semsource.json`, is resolved into
  `config.Config.GitToken` (`config/config.go:170`). Every git source's component configuration then
  carries it as `git_token` (`cmd/semsource/run.go:627`, `:1062`, `cmd/semsource/discovery.go:80`,
  `internal/sourcespawn/components.go:193`), and SemEngine pushes component configurations to
  `semengine_config_<org>_<stem>` without redaction. Anyone who can read the bucket can read the
  token, and rotating the environment variable has no effect until `config_revision` is bumped.
- **`semsource.json`.** `semsource add` and `semsource remove` load the file through `LoadConfig`, which
  applies defaults and environment fallbacks, and marshal the whole struct back (`cli/add.go:83`,
  `cli/remove.go:55`). With the variable set, the token is written into the file, as are
  machine-specific defaults such as `workspace_dir`.

The current-truth spec already holds object-store credentials to the opposite rule
(`runtime-configuration`: "Object-store credentials are resolved from the environment, never
persisted into configuration"). The git token is the same class of secret.

## What Changes

- **Environment only (owner ruling, 2026-11-18).** The token is read from `SEMSOURCE_GIT_TOKEN` by the
  code that uses it. **BREAKING:** `git_token` is removed from `semsource.json`; a file that still
  carries it fails strict decoding with an error naming the variable.
- **No component configuration carries the token.** `git_token` is removed from the git-source
  component configuration. The git source reads the variable when it is constructed.
- **The next boot scrubs the bucket.** The composition generation moves from 4 to 5, so the file
  replaces every retained component key with a token-free one.
- **`add` and `remove` write back only what the file held plus their change**: no defaults, no
  environment values.
- **Rotation takes effect on restart.** The README stops listing the token among retained overrides.

## Non-goals

- Purging earlier revisions of a component key. SemEngine owns the bucket and keeps five revisions per
  key; an operator who ran a token on main before this change rotates it (see `design.md`).
- A secret reference scheme (file path, keychain). The environment is enough for every current
  deployment.
- #267 (`semsource add <type> --flags` drops flags), though it touches `cli/add.go`.

## Impact

- `config/`, `cli/add.go`, `cli/remove.go`, `cmd/semsource/run.go`, `cmd/semsource/discovery.go`,
  `internal/sourcespawn/`, `processor/git-source/`, `README.md`.
- Spec: `runtime-configuration` gains two Requirements.
- Operators: move `git_token` from `semsource.json` to `SEMSOURCE_GIT_TOKEN`. beta.10 is a fresh
  storage break, so tagged deployments never hold a token in the bucket.
