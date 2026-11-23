# Reconcile group `source` on republish — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-23, sha256:c48c5a20ab85). Re-approved after the D1 amendment, which adds the bounded re-read retry on revision conflict and drops the metadata timestamp (2026-11-23, sha256:c87d3b898461; line entered by claude at the owner's instruction). Ruling: #336 comments 7100421719 and 7100564191.

## 2. Repro and completeness audit (before code)

- [x] 2.1 Repro committed and failing by design: `TestIntegration_ReingestDropsRemovedPredicates` (`internal/governance/stale_predicate_integration_test.go`) reports `code.relationship.calls` and `code.doc.comment` surviving the edit.
- [x] 2.2 Per-handler completeness audit (2026-11-23, read from code). Results are on #336. Opt-in status:
  - **Safe as-is:** git (a separate identity defect is #350), url, audio, objectstore.
  - **Safe with a fault guard (3.5):** image (`storage-ref`/`thumbnail-ref` dropped on store failure), video (keyframe `storage-ref`).
  - **ast:** needs 4.1a and 4.1b first. Folder/repo stay excluded (#347).
  - **doc:** safe for one root per project.
  - **ast and doc:** several roots sharing one `project` stay unsupported under reconcile until #349 is decided.
  - **cfgfile:** `config.dependency`, `config.image` and Gradle `project` are blocked by #348. Parent types are safe.
  - **Not writers:** vision (unwired; any future wiring must publish in the media handler's own payload or under its own source). Supersession (source `supersession`, never opts in).

## 3. Publisher reconcile (additive, off by default)

- [x] 3.1 `entitypub.WithReconcile(r)` option (`internal/entitypub/reconcile.go`); without it behaviour is unchanged. Proved by: `TestNoReconcileWithoutTheOption`.
- [x] 3.2 After a successful stream publish, `publishOne` reconciles group `source` before the delivery counters move (so `WaitSettled` covers it). `Desired` is the payload's `OwnedPredicates()` statements, an empty source stamped `semsource` and a zero timestamp stamped with the envelope time, which is fixed at millisecond precision. Per D1 as amended, the metadata carries no timestamp, because the client refuses a metadata timestamp that differs from a statement's own (the lifecycle owner fact carries its own). Proved by: `TestReconcileSendsTheGroupSetStampedLikeTheStream`.
- [x] 3.3 Outcomes: `entity_not_found` is benign. Any other error moves `reconcile_failed_total` and leaves `Published`/`Failed`/`Lost` untouched. A revision conflict (D1 as amended) is re-read and retried up to 3 times. Measured in the repro: the stream publish is acknowledged when stored, not when applied, so the first reconcile reads the pre-write state and loses the compare-and-set to its own stream write, every time. "The next pass repairs it" never converges. Each attempt re-reads, so no write lands over unseen state. Proved by: `TestReconcileOutcomes`, `TestReconcileReReadsAfterLosingToItsOwnStreamWrite`.
- [x] 3.4 Folder/repo exclusion: ast-source marks hierarchy and folder-chain payloads `SkipReconcile`, with a `// shortcut:` marker to remove in the #347 fix. Done at the call sites rather than by type in the publisher, so `entitypub` stays handler-neutral. Proved by: `TestReconcileSkipsContainersAndBodyFaults` (seed subtest).
- [x] 3.5 Fault guard: `EntityPayload.SkipReconcile` (originally `Partial`, renamed when the 5.2 ruling widened it; never serialized, copied through the snapshot) and the publisher skips reconcile for it. The AST body-offload faults are marked: a failed `Put`, an unreadable file, or a body store that failed to attach. Image/video marking moves to 4.2 with their opt-in. Proved by: `TestPartialPayloadIsNeverReconciled`, plus the `TestReconcileSkipsContainersAndBodyFaults` fault subtests, which fail with the marking removed.
- [x] 3.6 `Desired` takes only statements whose source is `semsource` or empty. Proved by: `TestReconcileSendsTheGroupSetStampedLikeTheStream` (a `supersession` lineage triple is left out).

## 4. Enable per handler

- [x] 4.1a ast watch path sets the file entity's `ContainedBy` (today only `BuildHierarchy` does, `source/ast/hierarchy.go:189-204`), so a watch publish no longer drops `code.structure.belongs`. Proved by: `TestWatchPublishCarriesTheSeedGroupSet` (fails without the fix: the file has no `code.structure.belongs`).
- [x] 4.1b ast body-offload faults go through 3.5.
- [x] 4.1 ast-source opts in when a NATS client is present (`NewSourceReconciler`). Proved by: `TestIntegration_ReingestDropsRemovedPredicates` green, which now waits for the reconcile revision, 5/5 runs; and `TestIntegration_ReingestKeepsOtherSourcesStatements`, where the `supersession` lineage edge, the `lifecycle` stale marker and `entity.ontology.class` survive a republish.
- [ ] 4.2 Each remaining handler that the audit (2.2) clears opts in, with its own completeness test. Handlers not cleared are listed on #336 with the blocking issue.

## 5. Cost on OSH

- [x] 5.1 Measured 2026-11-23 on OSH core (results are on #336, comment 7103565093). Warm restart settles in 46–49 s with reconcile off and 294–377 s with it on. Ready takes 41–44 s off and 122 s on, and with it on the phase is `degraded` because the seed times out. Entity writes are the same (28,464). 27,917 reconciles needed 27,914–27,917 retries: every first attempt loses to its own stream write. Counters added: `reconciled_total`, `reconcile_retries_total`.
- [x] 5.2 Owner ruling 2026-11-23 ([#336 comment 7104692071](https://github.com/C360Studio/semsource/issues/336#issuecomment-7104692071)): the bounded form, D4. ast-source passes `reconcile` only for watch edits and for reindexes whose recorded hash differs; the seed, new and unchanged files set `SkipReconcile`. `Partial` was renamed `SkipReconcile` for the wider meaning. Known gap: an edit made while down stays until the next edit (semengine#730 closes it). Proved by: `TestReconcileSkipsContainersAndBodyFaults` (first pass plus unchanged reindex make zero calls; a changed file reconciles on reindex and watch), and the integration repros, now on the watch path, 5/5 runs.

## 6. Records and review

- [ ] 6.1 ADR-0008 note: retain-and-relate governs entities and versions, and within one entity ID a source's statements are a complete set per publish.
- [ ] 6.2 `openspec validate --strict` and `task spec:check` green. Lint and unit, integration and race tests green.
- [ ] 6.3 Independent review by `graph-event-reviewer` and `go-component-reviewer`, then `ateam review request 346`.
- [x] 6.4 Upstream ask filed: C360Studio/semengine#730 (per-message complete-set intent). When it ships, the local reconcile and the D4 bound come out.
