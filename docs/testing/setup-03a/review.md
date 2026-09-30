# SETUP 03A review and admission state

This migration is delivered in [draft PR #213](https://github.com/C360Studio/semsource/pull/213).
It stays on the frozen SemStreams revision and does not link SemEngine. Running the comparison matrix
is separate from passing it, and neither replaces
independent implementation review.

## Independent review

| Review | Disposition | Effect |
| --- | --- | --- |
| Go component/lifecycle | Scoped approval for draft PR | Approved scope below |
| Graph/event contracts | Scoped approval; replay and enumeration changes requested | Full merge sign-off withheld |
| Disabled-entry workaround for #1443 | Independently approved | Real-manager restart/re-add tests pass |
| Final comparison evidence | Independently approved, including removal supplement | Evidence approval only |
| Baseline admission | Broker recovery failures remain at both pins | SETUP 03A admission blocked |

The independent Go component reviewer approved bootstrap, factories, payload registries/contracts,
immutable authority, desired-state handling, and worker/native-consumer/publisher ownership for the
**draft PR**. Independent race and native-callback regressions passed, including settlement with an
active callback plus more than one batch of queued work. Coverage was 100% for `workerjoin`, 91.3% for
`seedsup`, and 78.3% overall for `entitypub`; its critical Stop paths covered 90.9–100%. These figures
are scoped evidence and do not claim 80% coverage across every changed package.

The independent graph reviewer approved canonical identity, URL authority bounds, summaries, binary
proofs, default scope completeness, and corrected pagination. Full migration merge and qualification
sign-off remain **withheld**. Desired source removal lacks durable `source_removed` replay; truncated
entity enumeration and swallowed mutation errors also prevent trustworthy replay. Dynamic discovery
is disabled, and the upstream recovery/collision holds remain unresolved.

The bounded removal supplement now confirms two distinct failures. Both beta.161 and the reviewed
pinned snapshot retain the document parent and passage without `source_removed`, and neither emits
an automatic lifecycle RPC during the observed window. The old path has a canceled request trigger
and incorrect project scope; missing markers are therefore a pre-existing defect, not a newly proved
regression at the pin. The pinned replay implementation is still missing and remains a merge hold.

The supplement also exposed a migration defect: removing a source declared in the original JSON
returned a desired-change/restart receipt, but the source reappeared after restart. The upstream
configuration manager overlays retained keys onto file components, so deleting a key restores the
file entry ([SemStreams #1443](https://github.com/C360Studio/semstreams/issues/1443)). SemSource now
retains a disabled component envelope and enables it on re-add. The independent Go reviewer approved
this workaround; real-manager remove/restart/re-add and retry-guard tests pass with the race detector.
The final executable removal supplement passes 9 of 10 checks: the original JSON source stays live
until restart, is absent after restart, and the same handle is enabled by a re-add receipt and becomes
live on the next restart. Only the missing parent/passage `source_removed` markers fail; the automatic
lifecycle-request capture remains empty. Final BM25 passes 61/63 with the same two broker-recovery
failures. Final neural passes 65/67 with the same failures, and its provider supplement passes 6/6.
Final capacity also completes with zero source loss/errors and application and container cleanup
exit codes of zero.
Earlier executable comparisons remain provenance only.

Closing the replay hold still requires durable removal intent, complete enumeration, surfaced and
retryable mutation failures, and a behavior test across retirement/replay. Watched-file staleness
and runtime-added-source E2E tests cover different paths and did not detect the boot-file regression.

The independent evidence reviewer approved the final executable identified in the pinned ledger and
all primary, provider-fault, removal/re-add, and capacity artifacts. The review verified corpus and
executable hashes, profile crosswalk, fixed provider identity and counters, every baseline assertion
plus six authority checks, the two #1442 failures, #1143 collision and working explicit-subject queries,
matched capacity corpus/default-stem/resources and zero exits, and the all-retained tagged test
closure (110 / 218,808 → 114 / 232,126). The final removal supplement's 9/10 outcome is included,
with its missing-marker hold preserved. Scoped implementation draft approval covers commit
`c884d11e80dfdb6398aaf65e08ffef4fb3f03610`. Neither approval admits the system or authorizes merge/cutover.

The final fault deadline returns HTTP 504 `upstream_timeout` at 5,001 ms; the baseline reaches a caller
context deadline at about five seconds. Review treats this as a timing-dependent surface at the same
deadline boundary. The common expectation is bounded failure without a false absence result, not
identical raw error strings.

The CI integration list now includes `internal/sourcespawn`, retaining the original-file
remove/restart regression test. The independent Go reviewer approved this workflow-only change and
verified test selection. It does not change the qualified executable.

These are scoped reviewer dispositions, not blanket approval of every system behavior.

## Reproduced upstream holds

- [SemStreams #1442](https://github.com/C360Studio/semstreams/issues/1442): a persistent applied-sequence
  guard suppresses valid changed-source re-ingestion after memory GRAPH is recreated. Both baseline
  profiles and reviewed pinned profiles reproduce stale call/content after broker restart.
- [SemStreams #1143](https://github.com/C360Studio/semstreams/issues/1143): wildcard `graph.ingest.>`
  transport captures exact-query RPC requests and returns PubAck. The retained explicit-subject
  composition serves actual queries; the hazard remains when configuration widens the stream.
- [SemSource #178](https://github.com/C360Studio/semsource/issues/178): the current parser guard reduces
  the OSH payload volume. A guarded transport run does not establish that the original capacity
  regression is fixed. Structural/embedding completion is a separate gate.

## Explicitly unavailable paths

Dynamic branch discovery and remote submodule discovery are blocked with operator warnings. Their
changes are not persisted automatically for a later boot. Pre-expanded supported boot sources are
the available composition. These warnings preserve honesty; they do not qualify the absent behavior.

Source physical purge, optional generation/clustering, SemConnect cases, same-broker independent
deployments, and a SemEngine-only consumer are not established by this migration's results. Different
platform stems separate identity/configuration, but shared graph subjects/storage still require broker
or account isolation for independent deployments.

## Check inventory

| Gate | Recorded result | Limit |
| --- | --- | --- |
| Full unit suite and final touched packages, race | Pass | Does not replace broker/provider qualification |
| Available integration suite | Pass | Two MinIO-dependent governance tests explicitly skipped |
| Original complete integration suite | Blocked | MinIO image retrieval failed |
| E2E desired add/remove and two restarts | Pass | Desired composition only; no removal replay proof |
| Full local E2E lane | Uncached pass, 47.509 seconds | Includes desired-state restarts and upgrade path |
| Agent consistency and OpenSpec strict validation | Pass | Includes updated migration artifacts |
| gofmt, vet, pinned revive | Pass after disabled-envelope fix | Earlier formatting failure superseded |
| Final executable matrix | BM25 61/63; neural 65/67; faults 6/6 | Same two broker-recovery failures |

The original integration gate could not fetch its MinIO image: Docker Hub denied the image and Quay
returned HTTP 401. The original beta.161 fixture independently reproduces the same pull failure, so
this is an environmental evidence gap rather than an attributed migration regression. The available
suite records its two explicit skips and does not qualify the missing MinIO-backed governance paths.
A later image-backed rerun must close that evidence gap.

The qualification harness's `stop(false)` wait error is not asserted. Semantic restart results alone
do not certify graceful exit. The separately passing E2E lane and canonical capacity probes with
`application_exit=0` support the recorded exit claim; the qualification report must retain this limit.

All retained production packages' test closure is now measured: beta.161 has 109 packages / 218,176
lines (110 / 218,808 with tags); the pin has 114 / 232,126 with or without tags. The 112-package
figure covers only direct/composed roots and their tests. The independent evidence reviewer verified
and approved this distinction and the wider measurement.

The versioned [baseline](baseline-results.json), [pinned result ledger](pinned-results.json),
[dependency measurement](dependency-results.json), and [immutable pins](pins.json) are the evidence
sources. The OpenSpec task list leaves proof/sign-off tasks open when only a failing run was completed.

## Cutover boundary

SemSource remains on SemStreams. Later qualification uses a separate branch with SemEngine as the
exclusive substrate; no binary may import both. Mainline switches only after the complete semembed
workload and promised lower profiles pass, with the required contract and independent review gates.
