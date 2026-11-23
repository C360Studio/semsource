# Reconcile group `source` on republish — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-23, sha256:c48c5a20ab85). Re-approved after the D1 amendment, which adds the bounded re-read retry on revision conflict and drops the metadata timestamp (2026-11-23, sha256:c87d3b898461). Re-approved a second time after D4 was decided by the 5.2 ruling and the known gap was added to the spec (2026-11-23, sha256:493f7884e201). Re-approved a third time after D1 was amended by the specialist review (wait for the write to apply, worker off the drain loop) (2026-11-23, sha256:8226c457d77d). Re-approved a fourth time after round-2 review reworded the birth scenario and added the envelope-time rule to D1 (2026-11-23, sha256:47a0e29f1013; all four lines entered by claude at the owner's instruction). Ruling: #336 comments 7100421719 and 7100564191.

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
- [x] 3.2 After a successful stream publish, `publishOne` reconciles group `source` before the delivery counters move (so `WaitSettled` covers it). `Desired` is the payload's `OwnedPredicates()` statements, an empty source stamped `semsource` and a zero timestamp stamped with the envelope time, which is fixed at millisecond precision. Per D1 as amended, the metadata carries no timestamp, because the client refuses a metadata timestamp that differs from a statement's own (the lifecycle owner fact carries its own). Proved by: `TestReconcileWaitsForItsStreamWriteAndMatchesItsStamping`.
- [x] 3.3 Outcomes (D1 as amended): `entity_not_found` is not an error; like a read that does not yet show the write, it is read again with a 20–500 ms backoff until the stream write applies, within the 30 s budget. A revision conflict re-reads the same way. Any other error, a full queue, or a write not applied within the budget moves `reconcile_failed_total` and leaves `Published`/`Failed`/`Lost` untouched. Measured in the repro: the stream publish is acknowledged when stored, not when applied, so a reconcile that does not wait loses the compare-and-set to its own stream write, every time. Each attempt re-reads, so no write lands over unseen state. Proved by: `TestReconcileOutcomes`, `TestReconcileWaitsForItsStreamWriteAndMatchesItsStamping`.
- [x] 3.4 Folder/repo exclusion: ast-source marks hierarchy and folder-chain payloads `SkipReconcile`, with a `// shortcut:` marker to remove in the #347 fix. Done at the call sites rather than by type in the publisher, so `entitypub` stays handler-neutral. Proved by: `TestReconcileSkipsContainersAndBodyFaults` (seed subtest).
- [x] 3.5 Fault guard: `EntityPayload.SkipReconcile` (originally `Partial`, renamed when the 5.2 ruling widened it; never serialized, copied through the snapshot) and the publisher skips reconcile for it. The AST body-offload faults are marked: a failed `Put`, an unreadable file, or a body store that failed to attach. Image/video marking moves to 4.2 with their opt-in. Proved by: `TestSkipReconcilePayloadIsNeverReconciled`, plus the `TestReconcileSkipsContainersAndBodyFaults` fault subtests, which fail with the marking removed.
- [x] 3.6 `Desired` takes only statements whose source is `semsource` or empty. Proved by: `TestReconcileWaitsForItsStreamWriteAndMatchesItsStamping` (a `supersession` lineage triple is left out).

## 4. Enable per handler

- [x] 4.1a ast watch path sets the file entity's `ContainedBy` (today only `BuildHierarchy` does, `source/ast/hierarchy.go:189-204`), so a watch publish no longer drops `code.structure.belongs`. Proved by: `TestWatchPublishCarriesTheSeedGroupSet` (fails without the fix: the file has no `code.structure.belongs`).
- [x] 4.1b ast body-offload faults go through 3.5.
- [x] 4.1 ast-source opts in when a NATS client is present (`NewSourceReconciler`). Proved by: `TestIntegration_ReingestDropsRemovedPredicates` green, which now waits for the reconcile revision, 5/5 runs; and `TestIntegration_ReingestKeepsOtherSourcesStatements`, where the `supersession` lineage edge, the `lifecycle` stale marker and `entity.ontology.class` survive a republish.
- [x] 4.2 Remaining cleared handlers opt in, each with a completeness test and a gating test (spec citations on each). Their group statements no longer carry their own timestamps, so the fence applies; stored timestamps move from the handler's index time to the envelope time.
  - **url:** reconciles a page whose statements differ from those this process last sent; seed and unchanged polls skip. Proved by `TestChangedPagePublishCarriesTheCompleteGroupSet`, `TestReconcileOnlyPagesThisProcessSawChange`, `TestWatchStateMatchesIngestState`.
  - **doc:** same rule per entity; roots sharing a system skip (#349). Proved by `TestDocWatchRepublishCarriesTheCompleteGroupSet`, `TestDocReconcileOnlyKnownChanges`.
  - **cfgfile:** same rule; `config.dependency`, `config.image` and `config.project` always skip (#348), as do roots sharing a system. Proved by `TestCfgfileWatchRepublishCarriesTheCompleteGroupSet`, `TestCfgfileReconcileOnlyKnownChangesOfSingleWriterTypes`.
  - **image, audio, video:** the seed skips, a watch publish reconciles, and a store fault skips the whole event. Proved per package by `TestWatchPublishCarriesTheSeedGroupSet` and `TestReconcileOnlyWatchedChangesWithoutStoreFaults` (the video keyframe subtests need ffmpeg and skip without it).
  - **objectstore:** the seed and a polled create skip, a polled modify reconciles. Proved by `TestPollPublishCarriesTheSeedGroupSet`, `TestReconcileOnlyModifiedObjects`.
  - **ast:** roots sharing a system skip (#349). Proved by `TestRootsSharingASystemDoNotReconcile`.
  - **git: not opted in.** Two git-source instances can share a system slug (path basename or URL), so each HEAD change would delete the other's statements.

## 5. Cost on OSH

- [x] 5.1 Measured 2026-11-23 on OSH core (results are on #336, comment 7103565093). Warm restart settles in 46–49 s with reconcile off and 294–377 s with it on. Ready takes 41–44 s off and 122 s on, and with it on the phase is `degraded` because the seed times out. Entity writes are the same (28,464). 27,917 reconciles needed 27,914–27,917 retries: every first attempt loses to its own stream write. Counters added: `reconciled_total`, `reconcile_retries_total`.
- [x] 5.2 Owner ruling 2026-11-23 ([#336 comment 7104692071](https://github.com/C360Studio/semsource/issues/336#issuecomment-7104692071)): the bounded form, D4. ast-source passes `reconcile` only for watch edits and for reindexes whose recorded hash differs; the seed, new and unchanged files set `SkipReconcile`. `Partial` was renamed `SkipReconcile` for the wider meaning. Known gap: an edit made while down stays until the next edit (semengine#730 closes it). Proved by: `TestReconcileSkipsContainersAndBodyFaults` (first pass plus unchanged reindex make zero calls; a changed file reconciles on reindex and watch), and the integration repros, now on the watch path, 5/5 runs.

## 6. Records and review

- [x] 6.1 ADR-0008 note: a header line and §6 ("Retention applies to entities and versions; within one entity ID, a source's statements are a complete set"), with the single-writer assumption, source scoping, the known gap and semengine#730.
- [ ] 6.2 `openspec validate --strict` and `task spec:check` green. Lint and unit, integration and race tests green.
- [ ] 6.3 Independent review by `graph-event-reviewer` and `go-component-reviewer`, then `ateam review request 346`.
  - [x] Specialist review 2026-11-23, both on head 8357bbb. Findings fixed, and D1 amended with owner approval:
    - **Major (both reviewers): an older queued write restored a dropped predicate.** The reconcile now waits until its stream write applies (the stored state holds every desired statement), fences the replace at that revision, and skips a job once a later publish is queued. Proved by `TestOlderQueuedWriteCannotRestoreADroppedPredicate` and `TestReconcileWaitsForItsStreamWriteAndMatchesItsStamping`; both fail when the applied gate is removed.
    - **Major (go-component): a slow reconcile stalled the drain.** Reconciles now run on a single worker behind a bounded queue (4,096), with a 30 s budget per job and 5 s per request; a full queue drops the reconcile, counted. Abandoned at Stop, uncounted: `TestStopAbandonsAWaitingReconcileUncounted`.
    - **Minor:** the predicate set comes from the contract's `source` group; shutdown cancellation is not counted; the stamping test checks the envelope's `created_at`; the not-found fixture is a `*projection.MutationError`; `TestFileParentID_MatchesBuildHierarchy` covers a root file, nested files and a versioned system; `TestWatchPublishCarriesTheSeedGroupSet_PerLanguage` covers Go, Java and Python (Svelte and C not covered).
    - **Nits:** the spec scenario now names the bounded re-read. The KV-polling integration loops are unchanged; they poll observed state under a deadline.
  - [x] Independent review (`ateam review request 346`, Codex gpt-6-astra high) at fc8e23a: REQUEST CHANGES. Fixed:
    - **P1: the fence could mistake a repeated write for the stored one.** Stored A, then queued B and A again within one millisecond, so the final A's statements matched the stored A and the reconcile ran early. The envelope time is now strictly increasing per publisher, so every publish's envelope-stamped statements are unique. The fence compares timestamps to the nanosecond, and a payload with no envelope-stamped group statement is not reconciled (no enabled handler sends one). Proved by `TestReconcileFenceTellsARepeatedWriteFromTheStoredOne` with a frozen clock, which fails 3/3 without the monotonic envelope time.
    - **P2: Stop could exceed its deadline on an in-flight request.** The worker's context is cancelled at Stop, and joining the worker honours the caller's deadline. Proved by `TestStopIsNotHeldByAnInFlightReconcileRequest`, which fails 3/3 without the cancel.
    - **P2: a dropped job leaked its tracking entry.** It is now forgotten on drop. Proved by `TestQueueOverflowLeavesNoTrackingEntry`, which fails 3/3 without the cleanup.
    - **P2: gofmt.** Fixed, and `task lint` is clean.
  - [ ] Independent review round 2 at ff10704: REQUEST CHANGES. Fixed:
    - **P1: the monotonic envelope time ran ahead of the clock.** Each publish moved it 1 ms, so a fast burst stored future timestamps and a restarted publisher's writes were rejected as older by `ReplaceBySource`. The envelope time is now the wall-clock millisecond, never earlier than the previous one; only a repeat of the same entity within one millisecond moves it on, by 1 ms, so the fence stays unique per entity. Proved by `TestEnvelopeTimeBurstDoesNotOutrunTheClock` (a 1,000-entity burst, then a restart checked against the real `ReplaceBySource`), which fails without the fix; `TestReconcileFenceTellsARepeatedWriteFromTheStoredOne` still passes.
    - **P2: a file truncated between parse and body read skipped the fault guard.** A symbol whose parsed lines the read no longer has is now faulted. Proved by `TestBodyTruncatedBetweenParseAndOffloadIsFaulted`, which fails without the fix.
    - **Notes:** the birth scenario, the proposal and 3.3 now describe the budgeted re-read; stale test names in 3.2, 3.5 and 3.6 are corrected; the three uncited reconcile tests carry spec citations.
- [x] 6.4 Upstream ask filed: C360Studio/semengine#730 (per-message complete-set intent). When it ships, the local reconcile and the D4 bound come out.
