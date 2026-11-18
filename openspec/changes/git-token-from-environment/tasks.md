# Git token from environment tasks

## 1. Approval

- [x] 1.1 Obtain owner approval of `proposal.md` and `design.md` (2026-11-18).

## 2. Token at the point of use

- [x] 2.1 Add `config.GitToken()` and delete `config.Config.GitToken`; wrap the strict-decode error for
  `git_token` with a pointer to `SEMSOURCE_GIT_TOKEN` (D1, D2).
- [x] 2.2 Remove `git_token` from the git-source component configuration and from
  `internal/sourcespawn/components.go`; the git source reads `config.GitToken()` in `Initialize` (D3).
- [x] 2.3 Boot discovery and runtime spawning read the token through `config.GitToken()`:
  `sourcespawn.Options.GitToken` is removed and the default-branch lookup calls the accessor.
- [x] 2.4 Bump `compositionGeneration` to 5 and record the new fingerprint.
- [x] 2.5 Test: with `SEMSOURCE_GIT_TOKEN` set to a probe value, no generated component configuration
  (file-generated, discovered, or runtime-spawned) contains it. Mutation-check by restoring the field.
- [x] 2.6 Test: a file carrying `git_token` fails to load with the variable named.
- [x] 2.7 Integration test on a real broker: a retained git-source key carrying a token is token-free
  after one boot at the new generation.

## 3. CLI writes

- [x] 3.1 Add `config.WriteSources` and move `add` and `remove` onto it (D4). Implemented as a
  write helper after the existing `LoadConfig` validation rather than an edit callback, so the
  interactive prompts keep their flow; the behaviour is D4's.
- [x] 3.2 Test: with the token set and no `workspace_dir` in the file, `add` writes neither; `remove`
  on an invalid file fails and leaves it unchanged.

## 4. Docs and close

- [x] 4.1 README: drop `git_token` from the fields table and from the retained-overrides list; state
  that rotation takes effect on restart. Release-note line for beta.10 (rotate a token used on main).
- [ ] 4.2 Run the go-component reviewer on the diff.
- [ ] 4.3 Archive the change.
