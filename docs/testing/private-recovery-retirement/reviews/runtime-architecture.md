# Independent runtime architecture and graph review

Status: **approved for the frozen corrective production scope**. No blocking architecture or graph
findings. This technical approval does not claim that final process/full-repository qualification is
complete, and is not a GitHub human review. Worker B's publisher/producer approval is recorded separately
in `review-publisher.md`.

## Exact scope

Reviewed the frozen Worker A production changes on `codex/remove-private-lifecycle`, plan HEAD
`0221635b7cb42cfc6d443fd492a7941cb58b6397`, against both canonical component and graph/event reviewer
contracts and the approved `remove-private-source-recovery` design. `reviewer-runtime-files.json` records
22 current file hashes and the deleted root/private-authority files. The manifest includes relevant
unchanged prefix-query, HTTP and module files and the CI changes. No production files were edited.

The approved delta removes the journal, coordinator, epochs, receipt/proof contracts, effect fences,
replay and private root decorators. Searches found no production imports or calls into the deleted
`sourceintent`, `sourcelifecycle` or `seedproof` packages. The single remaining private bucket name is
in the root compatibility guard. Ordinary supersession work and source status are not replacement
recovery authorities: they do not own removal requests, durable command records or replay outcomes.

## Architectural checks

- `cmd/semsource/legacy_lifecycle.go` uses the pinned existing-only `GetKeyValueBucket`, followed by fresh
  `Status`. It permits only classified absence or zero retained messages. NATS v1.52.0 `Values` is the
  stream message count, including history/tombstones. No record/key/phase parser or storage mutation
  is retained. Lookup/status errors and canceled/deadline contexts refuse admission. Root invokes it
  immediately after connection and before ordinary stream/config provisioning or component admission.
- This is intentionally a whole-account bucket refusal. It does not certify terminal-looking records
  or resolve unknown effects. Old writers must be stopped, and old nonempty storage stays intact; the
  supported separate fresh storage boundary is documented. This review does not qualify concurrent old
  writers, migration of nonempty state or any operator deletion procedure.
- Desired Add/Remove stays on ConfigManager-owned component envelopes and manifest data. The new read
  adapter observes that existing authority only; request-local views do not persist their own state or
  start a repair loop. The path has no graph readiness, tail, journal or effect dependency. Full disabled
  envelopes, deterministic handles, request-driven manifest repair and explicit partial-write errors
  remain. Local channel gates serialize requests with cancellation before admission; they retain no work.
- Replies explicitly distinguish desired changes from active composition and projection. Successful
  and partial desired-change replies expose `projection_status:"unavailable"`; retired generation and
  phase fields are removed. Authenticated HTTP lifecycle requests return 410. Both source-projection
  and legacy `source_removed` RPC paths explicitly refuse with no graph lookup or mutation. MCP wording
  reflects restart activation and keeps partial committed receipts visible with their error.
- Existing graph safeguards survive: complete bounded paging, exact prefix boundary checks, duplicate
  and cyclic page refusal, truncation/error propagation, effective-authority filtering, typed exact-parent
  passage liveness, ordinary deletion/recreation and sticky retained `source_removed`. No new whole-source
  marker or positive freshness claim is admitted. The OpenSpec delta now exempts `source_removed` from
  generic absent-set clearing while retaining ordinary passage-liveness rules.
- CI replaces the deleted private lifecycle integration package with `cmd/semsource` so the real guard
  tests run. The separate exact `TestDesiredSourceLifecycleWithoutRecovery` case is added to e2e-local
  under frozen no-provider BM25 configuration; it does not select or rewrite old positive marker tests.
  Process body-byte comparisons using object-store inspection are retained-content checks, not a claim
  that a new public body retrieval API or semantic qualification has been delivered.

## Independent validation

`go test -race -count=1` passed for all five packages: `cmd/semsource`, `processor/source-manifest`,
`processor/supersession`, `internal/sourcespawn`, and `processor/mcp-gateway`.
Output: `reviewer-runtime-focused.log`, exit 0.

The following real-NATS cases independently passed with `-race -tags=integration -count=1` in
`cmd/semsource` and `processor/supersession` (output `reviewer-runtime-integration.log`, exit 0):

- `TestIntegrationLegacyLifecycleCompatibility`
- `TestRootRefusesBeforeProvisioning`
- `TestIntegrationSourceProjectionOwnsRefusalRPC`
- `TestIntegrationCanceledQueuedProjectionCannotMutate`
- `TestIntegrationStopRetainsActiveSubscriptionOnDeadline`

These prove retained storage refusal without changing the inspected values/revisions, guard ordering
before GRAPH creation, no creation on an absent-bucket probe, explicit refused RPC behavior, zero
late-request mutation after caller cancellation, and retained drain ownership on deadline. The guard
fixture was subsequently isolated by its author so each value case independently excludes tombstone
contamination; the final author run is a separate evidence file.

The package run also covers sticky marker oracles, typed parent association, truncation-before-write,
partial mutation failure, cancellation-before-admission, truthful committed desired flags and explicit
unavailability. Developer failing-first evidence in `worker-a-red.log` reproduces old response/private
ownership semantics and lost committed Add receipts. The developer's final test-only refinements and
full governance/process gates remain separately attributable, not inferred from this review.

A final narrow production delta preserves the joined cause in `KV_WRITE_FAILED` wire errors instead
of reducing them to the operation label. Validation and not-found messages are unchanged. Independently
reviewed this delta and reran desired-state/readback/cancellation/unavailability tests with `-race`: PASS
(`reviewer-runtime-manifest-final.log`, exit 0). The reviewed-file manifest includes this final delta.

`git diff --check` passed. Strict OpenSpec and the edited delta's 120-column check passed. Exact evidence:

- `reviewer-runtime-files.json`: `f26b85b894724587eadff2138c1f77019f03539463e9c72f91da27211b4f783f`
- `reviewer-runtime-focused.log`: `5cc0405c431071d56df49ef396e9b63e50d71ad80f15a26f4eeccef2cb936dc8`
- `reviewer-runtime-integration.log`: `ac89e53dca585f9c02562b90a315675ff85eaf9998c43b2d8a4203bc83cc8ffe`

- `reviewer-runtime-manifest-final.log`: `0125559af0a86c89779b1f2913007244f628eec17c14135b278b78f1ae1f0e8a`

## Limits and remaining delivery gates

The private-authority approval is withdrawn, not repaired under another name. Historical positive and
failed #215 records remain attributable to their old binary. This correction deliberately cannot pass
the unchanged source-removal-positive expectation, and does not qualify all-input completion, conditional
source reactivation, unknown-effect resolution, crash recovery, broker-restart repair, numeric graph
round trips or semembed. #18, #19 and #20 remain separate framework contract decisions, not premises
requiring a consumer subsystem. Final binary/source provenance, changed-contract process results,
unchanged baseline outcomes, independent Go review and full CI-equivalent gates are owner delivery work.
