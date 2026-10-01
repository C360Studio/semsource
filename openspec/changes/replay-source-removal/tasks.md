# Implementation and proof tasks

Production implementation is committed at `ce241a5c6e07f69e5450bb60885b537e89ea27a9`.
The [final result ledger](../../../docs/testing/source-removal215/final-results.json) identifies the
clean binary and later test-only follow-up. Checked tasks cover the stated implementation or proof;
they do not close the open acceptance gates. The combined process run exits 1 with two #1445 failures.

## 1. Contract and baseline

- [x] Inspect issue #215, current code, pinned ADR-094/095/102/104 and storage/query/mutation APIs.
- [x] Apply `kv-or-stream` and `orchestration-check`; record operational ownership and repair obligations.
- [x] Preserve the unchanged pre-fix missing-marker probe: 9/10, exit 1, no automatic lifecycle request.
  Preserve the separate additive pre-fix run: 7/8, exit 1, missing lifecycle journal.
- [x] Obtain architect and independent Go/graph contract approval before implementation.
- [x] Create `internal/sourceintent/contract.go` and confirm the interface with both developer owners
  before parallel implementation; approve the later retained-pass and effect-fence amendments.

## 2. Intent and desired-state protocol

- [x] Implement the no-eviction journal, typed binding/key validation and CAS generations. Prove policy
  validation, stale completion and foreign/corrupt rejection in `TestRemovalJournalRetentionAndCAS`,
  `TestJournalRejectsCorruptBindingAndVersions` and `TestJournalRejectsGenerationRollback`.
- [x] Prove real-NATS reopen, replica mismatch and memory-storage refusal in
  `TestIntegrationJournalRetentionAndReceipts` and `TestIntegrationJournalRefusesMemoryBucket`.
- [ ] Add the explicit real-NATS startup refusal case for positive global MaxMsgs after successful
  Ensure; the existing policy-validator unit case does not independently exercise that startup path.
- [x] Implement intent-before-disable-before-ack. Prove prepare, disable, ambiguous committed disable,
  manifest and pending-promotion failures in `TestRemovalIntentBeforeDesiredAck` and
  `TestRemovalCrashBoundaries`.
- [x] Repair committed partial desired writes without disabling an enabled prepared source or accepting
  unknown handles: `TestRemovalPartialWritesRepair` and `TestCoordinatorUnknownAndPreparedRetry`.
- [x] Report committed disable despite stale memory after a write error:
  `TestDisableReceiptReadsCommittedEnvelopeAfterMemoryFailure` and
  `TestLifecycleRemovalReceiptReportsDurablePartialCommit`.
- [x] Serialize desired changes and replay; reject stale generation completion and repair a committed
  changed reactivation config before next-boot binding: `TestRemovalRetirementAndReaddFence` and
  `TestBootRepairsCommittedReactivationRefreshAfterJournalFailure`, under race detection.
- [x] Prove cancellation-aware admission, synchronous lock ownership and bounded direct-call joining:
  `TestGateHonorsCancellation`, `TestRemovalProjectionCancellation`,
  `TestDirectProjectionStopJoinsCanceledCall` and `TestDirectProjectionCannotEnterAfterStopBegins`.
- [ ] Add a combined owner-shutdown test that stops the replay worker while a producer receipt remains
  in flight; standalone publisher drain and direct projection Stop tests cover the separate owners.

## 3. Retirement, scope, and projection

- [x] Record SemStreams #1444. Do not decode private applied-sequence values or treat bounded advisory
  events as a parked-work census.
- [x] Implement explicit TailProof and pending/applied_tail_unproven. Prove repeat retained passes never
  claim terminal completion in `TestCurrentRetainedPassRemainsPending` and
  `TestIntegrationTailIsCurrentRetainedOnly`; prove empty-set projection in
  `TestRemovalReplayEmptyRetainedSet`.
- [x] Capture input identity before acknowledgement; gate replay on boot retirement, current tail and
  desired scope. Prove admitted input derivation, changed consumer identity, backlog, sibling overlap,
  changed desired config and degraded graph status with `TestGraphInputsAdmittedConsumerIdentity`,
  `TestRemovalBarrierDefersUnsafeProjection`, `TestRemovalRetirementAndReaddFence` and the real tail test.
- [ ] Complete explicit broker fault cases for stream recreation, AckNone/AckAll, wrong filter/delivery
  policy and recreated consumer with zero backlog. Implemented fail-closed checks are not a substitute
  for each originally planned fault proof; terminal accepted-input proof remains blocked by #1444.
- [x] Derive exact producer selectors; prove Project/version, repo child, authority and taxonomy
  isolation plus overlap refusal in `TestRemovalExactSourceScope`, `TestScopeFamiliesAndMalformedIdentity`
  and `TestRemovalReplayScopeAndArtifactIsolation`.
- [x] Traverse opaque pages fully and reject cycles, malformed/truncated results or query errors before
  mutation: `TestRemovalReplayPages`, `TestRemovalReplayTraversesBeyondLegacyCap`,
  `TestRemovalReplayRejectsMalformedPages` and `TestLegacyLifecycleTruncationFailsBeforeMutation`.
- [x] Converge known retained entities idempotently and retry classified non-commit failures in
  `TestRemovalReplayIdempotentAndPartialMutation`. Preserve source facts and exact expected markers in
  both final process retirement cases. Unknown mutation replies remain fenced under section 8.
- [x] Preserve verified completed counts on repeated retained passes and avoid overstating partial work:
  `TestRepeatedRetainedPassPreservesVerifiedEntityCount` and
  `TestIncompleteRetainedPassCountsOnlyConfirmedMutations`.
- [x] Run startup and periodic repair with honest status and bounded ownership. Prove error redrive and
  cancellation in `TestRemovalReplayProofAndErrorRedrive`, `TestCoordinatorCancellationAndUnavailableStatus`
  and `TestRemovalProjectionCancellation`; final process retirement recovers without an operator resend.
- [ ] Add an explicit retained-pass late-arrival process case; repeated coordinator passes are proven,
  but the final process workload does not inject a new retained entity after its first marker pass.

## 4. Current publication receipts and reactivation

- [x] Record frozen public conditional-reconcile gap in SemStreams #1445; use no raw-subject shim.
- [x] Return conditional_reconcile_unavailable with zero clear writes for matching source_removed facts:
  `TestReactivationCurrentEntityReceipts`. Preserve the positive process freshness assertions as failures.
- [x] Inventory current-epoch sealed batches with ListSeeds and validate exact receipts/digests:
  `TestListSeedsOnlySealedCurrentEpoch`, `TestReceiptAndSeedImmutableAgreement`,
  `TestReceiptListRejectsForgedBinding` and `TestReactivationRejectsUnsealedOrIncompleteEvidence`.
- [x] Renew every enabled Reactivate history on boot, including Complete, and keep periodic inventory
  after completed batches: `TestCompletedHistoryRenewsEveryBoot` and
  `TestLiveInventorySurvivesLostWakeAndReopensFailure`.
- [x] Validate live target and same-epoch initial prerequisite independently; failed initial cannot be
  repaired by live success: `TestReactivationLiveBatchRequiresSameEpochInitialProof` and
  `TestLiveInventoryDoesNotRepairFailedInitial`.
- [x] Bind exact source generation and new epoch before admission, with no mutable global registry:
  `TestSourceFactoryBindings`, `TestPublisherBindRequiresPreAdmission` and
  `TestReactivationReceiptEpochAndManifest` reject mismatched configuration and stale proof.
- [x] Persist immutable receipts only after PubAck; retry receipt storage without republishing and retain
  drain ownership: `TestPublisherReceiptAfterAck` and `TestPublisherReceiptShutdownDeadline`.
  `TestPublisherSupersededBindingDrainsWithoutGrantingFreshness` proves obsolete proof does not wedge Stop.
- [x] Seal only successful exact seed enumeration/publication/receipt sets, including empty seeds:
  `TestPublisherSeedBoundaries`, `TestPublisherSeedRejectedSendCannotSeal`,
  `TestPublisherRejectsInvalidBatchEvidence` and `TestPublisherWatchBatchCannotCompleteFailedSeed`.
- [x] Keep source_removed sticky across filesystem, empty-Absent and passage oracles while preserving
  ordinary file lifecycle behavior: `TestSourceRemovedStickyAcrossLegacyOracles`,
  `TestPassageUsesExactParent` and the independently reviewed `TestIntegration_StalenessLifecycle`.
- [ ] Intended acceptance, blocked by #1445: clear only source_removed at the SAME authoritative revision
  used for the source-fact comparison. Preserve newer stale reasons and recompute after conflicts;
  a hidden reread/retry is not an admitted conditional mutation.
- [ ] Intended acceptance, blocked by #1445: make current same-ID parents/passages fresh while removed
  symbols/files/tail passages remain stale. Final selective process result is 24/26; completion and
  A freshness remain failed, while A-only publication proof and retained-stale B pass.
- [x] Prove checked-stop/offline-delete/reseed A-only evidence and two remove/add cycles in the final
  selective and rapid-re-add process cases. The rapid case observes zero transient old-removal markers.
- [ ] Prove the separate crash after A+B receipts but before receipt persistence/projection, offline B
  deletion, and partial-removal/re-add selective recovery; checked-stop evidence is not that crash proof.
- [ ] Specify and prove continuing publication/withdrawal eligibility, including later B recreation and
  an old B receipt followed by deletion or failed enumeration. Historical receipts and sticky revisions
  are insufficient; current ListSeeds inventory does not qualify live-B freshness.

## 5. Real process acceptance

- [x] Add a private-broker suite for original/runtime sources with durable intent, admitted old producer,
  observed process exit, new-boot retirement, exact retained markers and unchanged siblings/content.
  Final `TestRemovalReplayRetirement` passes 16/16 original and 20/20 runtime-added observations.
- [x] Kill after durable intent, restart against retained broker state, and repair without resend using
  explicit state barriers. Assert pending/applied_tail_unproven rather than terminal completion.
- [x] Prove re-add before first restart supersedes old work without transient old-generation mutation:
  `TestRemovalReplayReaddBeforeRestart` passes 27/27 with an authoritative KV transition watcher.
- [x] Preserve and qualify the AST-only dc.terms.created comparison rule with a pre-fix plain restart
  control and negative oracle tests. Keep strict original failure and all non-AST/fact/content checks;
  the final `TestRemovalReplaySiblingRestartControl` passes 8/8.
- [x] Execute the unchanged original probe against the reviewed binary: 10/10. Preserve its pre-fix 9/10
  failure and its legacy Stop limitation; it does not independently assert graceful application exit.
- [x] Record the complete six-case final run, exact commit/binary/module/config/workload identities,
  checked additive exits, all six exact broker cleanup results and raw temporary-evidence limitations.
- [ ] Pass the intended selective freshness process case. Keep its two final failed assertions visible;
  passing pending/no-clear observations must not substitute for positive acceptance.

## 6. Review and delivery

- [x] Obtain independent Go and graph/event approval of production contracts, retained-pass/pending-tail
  boundary, scope, context, serialization, receipt, effect and Stop ownership. Record review scope and
  critical coverage in `docs/testing/source-removal215/review.md`.
- [x] Independently review the test-only governance adaptation using real NATS, journal, tail and local
  projector; focused integration race run passes. Its desired-config seam does not claim process proof.
- [x] Pass gofmt, pinned revive/vet, unit/race (2,705 passed, 9 skips), full integration (2,826 passed,
  9 skips), local e2e (7), Garage (61), agent sync and strict OpenSpec. Preserve initial gate failures and
  named skips. Later test-only commit `16e3795` does not alter the production process binary.
- [x] Remeasure consumer and extraction dependencies, including tests, using an evidence-local modfile;
  record `dependency-results.json` without changing the frozen module pin.
- [x] Prepare versioned final results, independent review, exact pins and remaining holds for PR #213 /
  issue #215. Live publication and CI status are tracked there. Keep SemStreams substrate work and future
  SemEngine/semembed qualification separate; do not archive.

## 7. Explicit completion gates

- [ ] Obtain a supported stream-incarnation-aware applied/unresolved-input contract proving the entire
  accepted input set, including MaxDeliver, terminal rejection and retention gaps (#1444).
- [ ] Obtain a public caller-revision-fenced conditional reconcile with classified conflict/unknown
  outcomes and no hidden reread/retry (#1445); rerun positive freshness and independent graph review.
- [ ] Qualify continuing publication/withdrawal eligibility before live batches can grant freshness.
- [ ] Qualify unknown remote mutation outcome resolution (#1446). Durable refusal is safety evidence,
  not automatic recovery of an uncertain remote commit.
- [ ] Complete full semantic/semembed and eventual separate-branch SemEngine qualification before any
  mainline cutover. This BM25 follow-up and local numeric fingerprint tests do not satisfy those gates.

## 8. Generation and commit uncertainty review follow-up

- [x] Replace mutation RPC dispatch with a synchronous single-bound projector and refuse old RPC and
  legacy source_removed bypasses: `TestLocalProjectorRequiresOneSynchronousBinding`,
  `TestProjectionFactoryBindsSingleLocalOwner`, `TestSourceProjectionRPCRefusesAllEffects`,
  `TestLegacyLifecycleCannotRequestSourceRemoved` and real-NATS refusal/queued-cancellation tests.
- [x] Persist exact binding/entity/attempt fences before graph effects; classify only explicit safe
  terminal outcomes: `TestRemovalEffectFenceRequiredBeforeMutation`,
  `TestRemovalEffectFenceClassifiesCommitEvidence` and `TestRemovalEffectFenceAuthorizesExactAttempt`.
- [x] Block generation changes before config writes while an effect is unresolved; allow only safe
  existing nonoverlap boots: `TestDirectReplayRetainsUnknownEffectBeforeAdmission`,
  `TestUnresolvedEffectRefusesAddBeforeAnyConfigWrite` and
  `TestEffectBootAdmitsOnlyNonoverlappingExistingSources`.
- [x] Preserve exact terminal evidence through lost journal acknowledgements and reject stale/wrong
  attempts: `TestEffectTerminalEvidenceSurvivesProgressAndLostReply`,
  `TestEffectBeginAmbiguityAndExactResolution`, `TestEffectCannotAuthorizeStaleGenerationOrChangedDesired`
  and `TestJournalRejectsCorruptOrDiscardedEffect`. Graph readback never resolves unknown outcomes:
  `TestRemovalReadbackCannotResolveLostReplyFence`.
- [x] Track upstream outcome/commit ambiguity in SemStreams #1446 and obtain independent implementation
  approval for the amended synchronous/fence boundary. Recovery remains an open section 7 gate.
