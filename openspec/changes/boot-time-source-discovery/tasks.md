# Boot-time source discovery tasks

## 1. Approval

- [ ] 1.1 Obtain owner approval of `proposal.md` and `design.md`, including Q1 (runtime removal
  beats rediscovery).
  Hold: OPEN-Q proposal awaits owner approval

## 2. Evidence before code

- [ ] 2.1 Measure today's behaviour in an integration test against a local git fixture:
  - first boot of a remote multi-branch source with no checkout;
  - a branch created between boots on a retained store at an equal version.
  Record the before results in the PR (expected: boot error; branch not admitted).
- [ ] 2.2 Prove the D1 seam: a `PutComponentToKV` between `startConfigManager` and
  `createServiceManager` is admitted at that boot. Integration test in `cmd/semsource`.

## 3. Boot reconcile (additive)

- [ ] 3.1 Discovery-owned name derivation for branches and submodules, from a parent source.
  Unit test: names match `sourcespawn` and subwatch output, and an operator component is never
  derived.
- [ ] 3.2 Boot reconcile: put the missing components, delete the stale derived names, and skip
  any component disabled by a runtime removal (D3). Unit tests with a fake store for added,
  removed, parent-removed, retained-disabled and foreign-name cases.
- [ ] 3.3 Submodule build at boot through subwatch's build logic. Integration test:
  `processor/ast-source/submodule_expansion_integration_test.go` drives a boot instead of `Run`.
- [ ] 3.4 Typed discovery states (D4); boot never fails on discovery. Unit tests per state;
  `listing_failed` keeps the admitted children.

## 4. Runtime check and status

- [ ] 4.1 Read-only check (`ListBranches`+`MatchBranches`, `ListSubmodules`) against the boot set.
  Unit test asserts no worktree is created and no store write happens.
- [ ] 4.2 `discovery` block on `/source-manifest/status` and `source_status`. Payload test and
  MCP schema-budget check (`advertised-surface-coverage`).

## 5. Removal of dead paths

- [ ] 5.1 Delete `startBranchWatchers`, `startSubmoduleWatchers`, `publishBranchComponents`,
  `removeBranchComponents`, subwatch `Run`, and the two `capability: blocked` warnings.
  `go build ./...` and a grep gate that none remain.

## 6. Verification

- [ ] 6.1 Real-process restart test (`test/e2e`): boot, create a branch and a submodule, see
  `restart_required: true`, restart, see both ingested; then delete the branch, restart, and see
  its components gone.
- [ ] 6.2 Operator docs (README sources section, `docs/QUICKSTART.md` if it mentions branches)
  and `docs/testing/setup-03a` admission ledger updated.
- [ ] 6.3 Independent review: go-component-reviewer and graph-event-reviewer, plus the PR review
  record.
