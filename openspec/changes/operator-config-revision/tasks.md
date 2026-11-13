# Operator config revision tasks

## 1. Approval

- [ ] 1.1 Owner approves this proposal. Hold: OPEN-Q proposal awaits owner approval

## 2. Version

- [x] 2.1 Add `config_revision` to `config.Config` with load-time validation (non-negative).
- [x] 2.2 Derive the composition version as `<generation>.<config_revision>.0` in
  `buildSemstreamsConfig`; the generation is an integer constant, `3`.
- [x] 2.3 Fingerprint test: the composition generated from a fixed configuration matches the fingerprint
  recorded for the current generation; a change without a generation bump fails with the instruction to
  bump it.

## 3. CLI

- [x] 3.1 `semsource add` and `semsource remove` increment `config_revision` when they write the file.

## 4. Boot warning

- [x] 4.1 After the config manager starts, compare the file-generated services and components with the
  effective ones and warn with the differing entry names when they differ.
- [x] 4.2 Integration test on a real broker: same file twice is silent; an edit without a revision bump
  warns and is not applied; the same edit with a bump is applied.

## 4a. Review round 1

- [x] 4a.1 After a file-newer boot, delete retained component keys the composition lacks; integration
  cases for remove and for a path change across three boots.
- [x] 4a.2 Extend the comparison to the model registry and NATS settings.
- [x] 4a.3 Name retained-only entries separately and make the hint non-destructive.
- [x] 4a.4 `semsource init` over an existing file carries the revision forward.
- [x] 4a.5 README: the retained revision must be exceeded; environment overrides are retained too; older
  binaries refuse the field.

## 4b. Review round 2 (rulings on PR #271)

- [ ] 4b.1 A prune failure is logged at Warn and does not fail the boot; add a test for that path.
- [ ] 4b.2 Extend the prune's `// shortcut:` comment: components only, the window between the push and
  the deletion, the single-writer assumption, and the SemEngine issue once it is filed.
- [ ] 4b.3 Name components the file enables and the store holds disabled under `retained_disabled`, with
  the re-enable consequence in the hint; unit and integration cases.
- [ ] 4b.4 The plain hint stops promising that the file's values persist: it says the retained
  configuration wins until the file's revision is higher and points at the README section.
- [ ] 4b.5 README: a newer file re-enables sources removed at runtime; an interrupted boot can leave a
  dropped component behind until the next bump; a model registry or service the file no longer has is
  not removable at this SemEngine version.

## 5. Documentation

- [x] 5.1 README: `config_revision` in the configuration table and a short "changing configuration on an
  existing deployment" section.

## 6. Verification

- [x] 6.1 `task check`, the `cmd/semsource` and `config` integration tests, and `task spec:check` pass.
