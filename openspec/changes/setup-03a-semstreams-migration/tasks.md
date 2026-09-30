# SETUP 03A tasks

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
  Execution is recorded, but the promised recovery behavior is not proven.
- [x] 2.4 Freeze provider image/build, actual model artifact/version, dimensions, and preprocessing.
- [x] 2.5 Run beta.161 structural/BM25 and full semembed baselines; record snapshots, commands, per-case
  outcomes, observed defects, and unavailable evidence without silently skipping gates.

## 3. Breaking dependency and API migration

- [x] 3.1 Update exact Go module pin only after baseline run evidence is retained.
- [ ] 3.2 Adapt retained contracts from compile and ADR/implementation inventory; behavior-test changed
  bootstrap, context, NATS ownership, identity, and desired-versus-running configuration semantics.
  Implemented and tested adaptations remain blocked on deferred source_removed replay after desired removal.
  The #1443 disabled-entry workaround passes real-manager restart/re-add tests and independent Go review.
  Dynamic branch and remote submodule discovery are explicitly unsupported with warnings.
- [x] 3.3 Rerun identical workload/profile matrix with identical providers at selected pin.
  Final binary: BM25 61/63, neural 65/67, provider supplement 6/6; recovery remains blocked.
  Removal/re-add supplement 9/10: restart retirement/re-add pass; source_removed markers remain absent.
- [x] 3.4 Attribute every difference to a pinned upstream contract or reproduced defect.
  Includes #1443 disabled-envelope workaround, pre-existing removal-marker failure, and deadline-surface timing.
- [x] 3.5 Reconcile shared agents/skills and run `task agents:check`.

## 4. Defect probes and dependency measurement

- [ ] 4.1 Reproduce GRAPH capacity (#178); distinguish small probes from the known OSH corpus tail.
  Guarded OSH transport was measured; the current #175 parser guard reduces historical offered volume.
  The original unguarded #178 failure has not been reproduced or established fixed.
- [x] 4.2 Probe RPC stream collision (SemStreams #1143) at both pins and assess retained request paths.
- [x] 4.3 Measure production/test-inclusive closures with commands and raw non-test line counts;
  compare direct-import/registration cut and composed port-set closure.
  Production, root-only tests, and every retained production package's tests are measured separately.

## 5. Review and delivery

- [x] 5.1 Run gofmt, vet, unit tests, applicable integration/race checks, pinned revive, and OpenSpec
  validation; record unavailable gates/failures as blockers.
  Final lint/race, uncached local E2E, available integration, and agent checks pass.
  Removal-fix race/retry/restart tests and final document validation pass.
  Full MinIO-backed integration is environmentally blocked.
- [ ] 5.2 Obtain independent Go reviewer implementation and baseline-evidence sign-off.
  Component review grants scoped draft approval including #1443. Final comparison-evidence review
  is approved, including the removal supplement and its retained failure.
  Graph review approves defined scopes but requests removal replay/enumeration/error fixes; full
  implementation merge and qualification sign-off remain withheld.
- [x] 5.3 Update migration/results docs with pins, changes, provider manifest, outcomes, blockers;
  open/attach migration PR: https://github.com/C360Studio/semsource/pull/213 (draft).
- [x] 5.4 Retain holds until full matrix passes; later exclusive SemEngine branch qualification is
  separate, and mainline cutover waits for the complete semembed-backed workload.


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
