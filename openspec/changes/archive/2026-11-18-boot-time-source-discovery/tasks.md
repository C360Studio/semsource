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

- [x] 3.1 Discovery-owned names come from an ownership record (D2, amended 2026-11-18 after the
  #300 review): each boot writes the names it admitted per parent into the next-boot manifest
  (`discovered`), and the next boot owns exactly those. `TestReconcileDiscovery` covers a removed
  parent's names retired, a failed parent's names kept, and an unrecorded sibling name
  (`git-source-github-com-acme-app-v2`) untouched; `TestExpand_Pins` (internal/subwatch).
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

- [x] 4.1 Read-only check (`ListBranches`+`MatchBranches`, `subwatch.Pins`) against the boot set
  (`processor/source-manifest/discovery.go`; boot records it in `DiscoveryState.Admitted`).
  `TestCheckDiscovery` covers added/removed, ok, capped, awaiting_checkout, a checkout that
  arrived after an awaiting boot, and listing_failed without `restart_required` (Q2), and asserts
  the filesystem and worktree list are unchanged; `TestCheckDiscovery_Submodules` covers
  changed/removed pins. Reverting the Q2 rule fails the test.
- [x] 4.2 `discovery` block on the served status, which `/source-manifest/status` returns at the
  top level and `source_status` under `status`. `TestDiscoveryStatusBlock` (block present from
  the running check, absent with no discovering source, store write-refusing);
  `TestToolsListSchemaBudget` 7,602 of 8,192 bytes with the `source_status` description update.

## 5. Removal of dead paths

- [x] 5.1 Delete `startBranchWatchers`, `startSubmoduleWatchers`, `publishBranchComponents`,
  `removeBranchComponents`, subwatch `Run`, and the two `capability: blocked` warnings.
  `go build ./...` and a grep gate that none remain. Also deleted: subwatch's whole runtime
  `Watcher` (`New`, `Tick`, the confirm re-put) and `parseBranchPollInterval`; its tests now run on
  `Expand`. Grep gate: `TestRuntimeDiscoveryPathsStayDeleted` (cmd/semsource) fails on any Go
  file naming them; it caught the e2e test, whose blocked-warning assertion now waits for the
  first-boot `awaiting_checkout` submodule state (`TestE2E_SubmoduleIngest`, passing).

## 6. Verification

- [x] 6.1 Real-process restart test (`test/e2e`): boot, create a branch and a submodule, see
  `restart_required: true`, restart, see both ingested; then delete the branch, restart, and see
  its components gone. `TestE2E_DiscoveryAcrossRestarts` (~130s, local git fixtures, one
  retained store), added to CI `e2e-local`. With boot reconcile disabled it fails at the second
  boot (branch and submodule never ingested).
- [x] 6.2 Operator docs (README sources section, `docs/QUICKSTART.md` if it mentions branches)
  and `docs/testing/setup-03a` admission ledger updated. README gains a "Branch and submodule
  discovery" section (its own heading keeps the doc passage under the splitter's 6,000-byte hard
  max, `TestSplitPassages_NeverExceedsHardMax`); QUICKSTART's pre-expand hold becomes the boot
  contract; `compatibility.md` marks both blocked rows resolved by #216. `review.md` and
  `merge-readiness.md` are dated records superseded by that ledger and stay unchanged.
- [x] 6.3 Independent review: go-component-reviewer and graph-event-reviewer, plus the PR review
  record. Both specialists ran 2026-11-17. Fixed in this PR: false `restart_required` (submodule
  parent after its first clone; boot `listing_failed`), deletes on `awaiting_checkout`, name-based
  ownership (now the record), removed-parent children not retired, the remote clone component
  retired (now always admitted), a branch whose worktree failed pending forever (`Skipped`), and
  an overclaiming test. Filed: #303 (remote branch listing), #304 (disabled entries accumulate),
  #305 (pin move re-enables a removed submodule), #306 (expected count recount). Owner
  re-approved the amended D2 on 2026-11-18 (sha256:d4c8b4cb1297). graph-event re-review of the
  record (2026-11-18): its file-wins finding does not occur, because `pruneRetainedComponents`
  deletes the stale keys (`TestIntegrationDiscoveryAcrossFileWinsBoot`, which fails with the prune
  off); fixed a skipped branch's names retired, a partial reconcile leaving unrecorded names, the
  clone treated as a submodule parent, and the capped count including skipped branches; the Q1
  file-wins limit is #307. Owner kept the record in the manifest and re-approved the design on
  2026-11-18 (sha256:87ef642f6d61). The PR review record was requested through
  `ateam review request` when the PR was marked ready (2026-11-18).
