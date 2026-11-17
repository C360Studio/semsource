# Boot-time source discovery tasks

## 1. Approval

- [x] 1.1 Obtain owner approval of `proposal.md` and `design.md` (2026-11-17). Q1: a runtime
  removal beats rediscovery. Q2: `listing_failed` does not set `restart_required`.

## 2. Evidence before code

- [x] 2.1 Measure today's behaviour in an integration test against a local git fixture:
  - first boot of a remote multi-branch source with no checkout;
  - a branch created between boots on a retained store at an equal version.
  Record the before results in the PR (expected: boot error; branch not admitted). Measured
  2026-11-17 at `86626c0`: both as expected (results in PR #300).
- [x] 2.2 Prove the D1 seam: a `PutComponentToKV` between `startConfigManager` and
  `createServiceManager` is admitted at that boot. `TestIntegrationDiscoveryBootSeam`
  (`cmd/semsource/discovery_integration_test.go`); fails when the put moves after the build.

## 3. Boot reconcile (additive)

- [x] 3.1 Discovery-owned name derivation for branches and submodules, from a parent source.
  `TestBranchOwner` (cmd/semsource): a branch's names are owned even after the branch is gone,
  operator names are not. `TestExpand_MatchesTickAndOwnership` (internal/subwatch): Expand
  builds exactly what Tick publishes, and `OwnedBy` claims only that parent's names.
- [x] 3.2 Boot reconcile (`cmd/semsource/discovery.go`, called before `prepareComponentRegistry`
  so the fusion source scopes see discovered systems): put the missing components, delete stale
  derived names, skip runtime-disabled components (Q1), and rewrite the next-boot manifest
  (`sourcemanifest.RewriteDesiredManifest`). `TestReconcileDiscovery` covers added, stale,
  retained-disabled, failed-parent, file-generated and foreign names, plus idempotence. Each
  rule fails the test when it is reverted.
- [x] 3.3 Submodule build at boot through subwatch's build logic (`subwatch.Expand`).
  `TestIntegrationDiscoveryAcrossRestarts` (cmd/semsource, real NATS): a materialized submodule
  expands at boot, a branch created between boots is admitted on a retained store at an equal
  version, and a deleted branch leaves at the next boot. The runtime `Tick` test stays until
  task 5 removes `Run`.
- [x] 3.4 Typed discovery states (D4); boot never fails on discovery.
  `TestExpandRepoSources_DiscoveryStates` covers ok, awaiting_checkout (clone-only git source),
  listing_failed (nothing expanded) and capped. The reconcile keeps a failed parent's names
  (`TestReconcileDiscovery`).

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
