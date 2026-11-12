# Operator config revision tasks

## 1. Approval

- [ ] 1.1 Owner approves this proposal. Hold: OPEN-Q proposal awaits owner approval

## 2. Version

- [ ] 2.1 Add `config_revision` to `config.Config` with load-time validation (non-negative).
- [ ] 2.2 Derive the composition version as `<generation>.<config_revision>.0` in
  `buildSemstreamsConfig`; the generation is an integer constant, `3`.
- [ ] 2.3 Fingerprint test: the composition generated from a fixed configuration matches the fingerprint
  recorded for the current generation; a change without a generation bump fails with the instruction to
  bump it.

## 3. CLI

- [ ] 3.1 `semsource add` and `semsource remove` increment `config_revision` when they write the file.

## 4. Boot warning

- [ ] 4.1 After the config manager starts, compare the file-generated services and components with the
  effective ones and warn with the differing entry names when they differ.
- [ ] 4.2 Integration test on a real broker: same file twice is silent; an edit without a revision bump
  warns and is not applied; the same edit with a bump is applied.

## 5. Documentation

- [ ] 5.1 README: `config_revision` in the configuration table and a short "changing configuration on an
  existing deployment" section.

## 6. Verification

- [ ] 6.1 `task check`, the `cmd/semsource` and `config` integration tests, and `task spec:check` pass.
