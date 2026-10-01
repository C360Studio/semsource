# SETUP 03A tasks

Current disposition: [baseline merge readiness](../../../docs/testing/setup-03a/merge-readiness.md),
owner ruling 2026-10-01. Independent component and graph reviews approve baseline-only readiness at
`705ae66`. Known-at-pin failures stay failed, and unproven behavior tasks stay open. This approval is
separate from full #215 acceptance, SETUP 03B engine admission and the 04C mainline cutover gate.

## 1. Architecture and immutable evidence inputs

- [x] 1.1 Read repository guidance and SemEngine SETUP 03A; record consumer/upstream SHAs and issue #212.
- [x] 1.2 Create proposal, design, tasks, and capability delta before implementation.
- [x] 1.3 Inventory ADRs 094–107 at selected SHA; sign off fixture/API contracts and profile crosswalk.

## 2. Additive baseline fixture before dependency migration

- [x] 2.1 Version a small Go/docs corpus with fixed expectations and explicit absent-answer cases.
- [x] 2.2 Add bounded public-interface real-process qualification for ingestion, exact/scoped queries,
  provenance, exact bodies, updates, deletion/recreation, and restarts.
- [ ] 2.3 Prove application and persistent broker restart separately, including unfinished indexing;
  state acknowledged-write durability/re-ingestion boundaries.
  Probes executed at both pins: KV/body durability passes; changed-source recovery fails (#1442).
  Execution is recorded, but the promised recovery behavior is not proven. These exact failures are
  known-at-pin under SemStreams #1442 / SemEngine #15 and do not block the accepted 03A baseline;
  the assertions remain failed and the recovery proof task remains open.
- [x] 2.4 Freeze provider image/build, actual model artifact/version, dimensions, and preprocessing.
- [x] 2.5 Run beta.161 structural/BM25 and full semembed baselines; record snapshots, commands, per-case
  outcomes, observed defects, and unavailable evidence without silently skipping gates.

## 3. Breaking dependency and API migration

- [x] 3.1 Update exact Go module pin only after baseline run evidence is retained.
- [x] 3.2 Adapt and behavior-test bootstrap, context, NATS ownership, identity and desired-versus-running
  configuration semantics. Independent component and graph reviewers approve the consumer baseline.
  Keep the #1443 Enabled:false workaround, proved by real-manager restart/re-add and retry tests;
  SemEngine #17 owns the later config contract repair. Dynamic branch/remote submodule discovery remains
  explicitly unsupported (#216), not qualified by the baseline approval.
- [x] 3.3 Rerun identical workload/profile matrix with identical providers at selected pin.
  Frozen migration results: BM25 61/63, neural 65/67, provider supplement 6/6; the same two recovery
  assertions remain failed. Its historical removal supplement is 9/10 with missing markers.
  Later #215 evidence is additive: unchanged original probe 10/10, retained retirement 16/16 and 20/20,
  rapid re-add 27/27, selective reactivation 24/26. Do not rewrite the frozen ledger or claim all pass.
- [x] 3.4 Attribute every difference to a pinned upstream contract or reproduced defect.
  Includes #1443 disabled-envelope workaround, pre-existing removal-marker failure, and deadline-surface timing.
- [x] 3.5 Reconcile shared agents/skills and run `task agents:check`.
- [ ] 3.6 Complete intended source-removal/re-add acceptance tracked by #215. Durable retained-marker
  replay, complete enumeration, error handling and effect fences are implemented and independently
  reviewed; terminal all-input completion, selective freshness and unknown-effect recovery remain open.
  SemStreams #1444/#1445/#1446 map to SemEngine #18/#19/#20 contract decisions in 03B. Safe refusal is
  not a positive freshness pass; this follow-up does not block the owner-accepted baseline.

## 4. Defect probes and dependency measurement

- [ ] 4.1 Reproduce GRAPH capacity (#178); distinguish small probes from the known OSH corpus tail.
  Guarded OSH transport was measured; the current #175 parser guard reduces historical offered volume.
  The original unguarded #178 failure has not been reproduced or established fixed. Keep #178 open;
  the owner ruling makes this a nonblocking baseline follow-up, not a completed capacity proof.
- [x] 4.2 Probe RPC stream collision (SemStreams #1143) at both pins and assess retained request paths.
- [x] 4.3 Measure production/test-inclusive closures with commands and raw non-test line counts;
  compare direct-import/registration cut and composed port-set closure.
  Production, root-only tests, and every retained production package's tests are measured separately.

## 5. Review and delivery

- [x] 5.1 Run gofmt, vet, unit/race, applicable integration, pinned revive, agent checks and strict OpenSpec.
  The original MinIO block is preserved in historical records and closed by reviewed fixture PR #217;
  #218 corrects previously unselected AST fixture authority. Later full integration passes 2,826 tests
  with 9 named skips, unit/race 2,705 with 9, local e2e 7 and Garage 61. See fixture follow-up and #215
  final results for exact commands/snapshots; these runs do not rerun the frozen neural comparison.
- [x] 5.2 Obtain independent implementation and baseline-evidence review. Component/lifecycle and
  graph/event reviewers approve baseline-only merge readiness at `705ae66`, with no remaining
  consumer-owned safety blocker in that scope. Later replay/enumeration/error and fence fixes are
  reviewed; #215's full acceptance and future engine admission remain open. Technical sign-off does
  not assert a GitHub human approval or completed merge.
- [x] 5.3 Update migration/results docs with pins, changes, provider manifest, outcomes, blockers;
  open/attach migration PR: https://github.com/C360Studio/semsource/pull/213 (draft).
- [x] 5.4 Separate the owner-accepted baseline from full behavior qualification. Keep exact known-at-pin
  failures, unqualified behaviors and open follow-ups visible. Later qualification uses a separate
  SemEngine-only branch; mainline cutover still waits for the full semembed-backed 04C workload and
  retained lower profiles. This ruling changes no test result, fixture, provider or substrate pin.


## 6. AST integration fixture authority correction (#218)

The full integration rerun exposed three AST cases whose direct constructor fixtures omitted
`Dependencies.Platform`. The same failures reproduce on pristine migration commit `75a17f7`,
independently of the MinIO fixture repair. Normal unit gates exclude these integration-tagged
files, and the manually selected CI integration lane did not include AST. Keep production
validation and every existing test assertion unchanged; provide the same explicit test authority
that the composition root supplies in production.

- [x] 6.1 Supply matching `PlatformMeta` in the two AST integration constructor fixtures; prove
  `TestIntegration_StartReturnsWhilePathsUnavailable`, `TestIntegration_StopDuringInFlightSeedWaits`,
  and `TestIntegration_SubmoduleExpansionSeedsScopedEntities` reach and pass their behavior checks.
- [x] 6.2 Add `processor/ast-source` to the existing CI integration package list, retaining
  `internal/sourcespawn` and all other selected packages; run the workflow shell contract suite.
- [x] 6.3 Run the AST integration package with `-race`, lint and OpenSpec validation, then obtain
  independent Go reviewer sign-off on this test-only compatibility correction.
  Named regression cases and the full AST integration package pass with `-race`; lint, workflow
  shell contracts, and strict OpenSpec validation pass. Independent Go review approved the scoped
  test/CI correction; production authority validation and existing assertions remain unchanged.

## 7. Owner-directed baseline merge and next-stage handoff

- [x] 7.1 Record the additive 2026-10-01 baseline ruling and both independent scoped readiness approvals
  in `docs/testing/setup-03a/merge-readiness.md`; preserve historical reviews and frozen failed ledgers.
- [x] 7.2 Record SemEngine homes: #1442 → #15, #1143 → #16, #1443 → #17; #1444 → #18, #1445 → #19 and
  #1446 → #20 are 03B contract decisions, not all reproduced live defects or promised implementations.
  Preserve Enabled:false; no SemStreams fix or automatic post-pin synchronization is expected.
- [x] 7.3 Hand off `test/setup03a` as the acceptance corpus for 04A on a wholly SemEngine integration
  branch after 03B approval. Require a true no-embedder composition; BM25 structural coverage is only
  its baseline. Keep #178, #215, #216 and #219 open without treating them as baseline merge blockers.
- [ ] 7.4 PR owner records final GitHub checks and merge execution. Stop this delivery after the baseline
  handoff; do not start 04A implementation, alter pins, or archive open acceptance work here.
