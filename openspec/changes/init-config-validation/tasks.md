# Tasks: Init and Config Validation

## 1. Config-load validation

- [ ] 1.1 Validate namespace/org (and explicit source identity overrides) at config load via the
      shared segment validator (semstreams `ValidateEntityID` probe-ID composition, D2); errors
      name field + value + alphabet; unit tests (dotted, spaced, empty, unicode, valid)
- [ ] 1.2 `semsource run` startup runs the same validation before spawning components (D4); test

## 2. Wizard git identity

- [ ] 2.1 Replace the `"."` git URL default: origin-remote slug → path-derived slug fallback,
      never `"."` (D1); unit tests for both paths (with/without remote)
- [ ] 2.2 Wizard namespace prompt validates inline with the same rules; test

## 3. Validate as pre-flight

- [ ] 3.1 `semsource validate` composes one representative entity ID per configured source and
      validates it (D3)
- [ ] 3.2 Property test: for generated configs, validate-pass ⇒ publish-gate-pass (pins the
      contract of the runtime-configuration delta)

## 4. Proof and finalize

- [ ] 4.1 E2E: `init --quick` in a fixture repo (with remote) → run → commit entities present in
      graph and status (the audit's zero-git-history default becomes a regression test)
- [ ] 4.2 Release note (previously-accepted invalid orgs now fail loudly);
      `openspec validate init-config-validation`; gates green (revive v1.15.0, gofmt, vet,
      `go test -race`)
