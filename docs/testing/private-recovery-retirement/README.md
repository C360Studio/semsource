# Private source recovery retirement

The owner authorized [corrective PR #223][pr] after the [architecture reassessment](audit.md).
The [approved corrective contract][design] is committed at
`0221635b7cb42cfc6d443fd492a7941cb58b6397`, based on merged #213
(`3604a9ce8d5aec717253d7a905389f7910efd25d`). Independent architectural review approved this contract.
Implementation is committed at `5334716b588f6960ed54c8cafdc7033b01088e8e`. Independent publisher, runtime,
architecture/graph and process-test/CI logic reviews approved their separate scopes. Local process
evidence is complete, including preserved failed assertions. Hosted CI remains pending; these technical
reviews are not merge approval.

The earlier architectural approval of the private recovery design is withdrawn. Historical review,
passing tests and blocked acceptance remain attributable to their original snapshots. This correction
does not invalidate every migration change or rewrite any [frozen SETUP 03A result][baseline].

## Approved behavior

The implementation removes the coordinator, journal, boot epochs, publication receipts/seals, effect
fences and periodic replay. There is no replacement worker, durable command lane, state machine or recovery service.
It retains ConfigManager authority, complete `Enabled:false` envelopes, desired manifest/count repair,
deterministic handles and producer Stop/join. Focused tests prove desired changes with no graph client
and truthful partial commits when retained configuration is readable.

| Surface | Corrective contract |
| --- | --- |
| Successful desired Add/Remove | Desired change persisted; current composition unchanged; restart required |
| Successful/partial desired-change reply | `projection_status:"unavailable"`; partial failures retain their error |
| Old removal `generation` / `projection_phase` | No longer emitted; absence is not completion |
| `GET /sources/{id}/lifecycle` | Authenticated HTTP 410, `SOURCE_LIFECYCLE_UNAVAILABLE`; no state lookup |
| `graph.lifecycle.source` | Stateless explicit refusal; no mutation |
| Legacy lifecycle reason `source_removed` | Explicit refusal; no alternate owner or background retry |
| Ordinary file/passage staleness | Retained, including history, exact-parent rules and checked ownership |
| Existing `source_removed` markers | Remain sticky; path presence/re-add does not grant freshness |

Automatic whole-source stale projection and positive source reactivation are **unavailable at this
pin**, including after restart. A desired removal can succeed without marking retained graph entities.
No positive #215 acceptance is claimed. MCP descriptions and partial results express the same distinction.

## Existing storage boundary

Stop the previous writer before starting the corrected binary. Before stream/config provisioning or
producer admission, a bounded existing-only lookup checks `SEMSOURCE_SOURCE_LIFECYCLE` and reads its
fresh retained-message count. A classified absent bucket or count zero permits boot. Any retained
message or unreadable/timed-out result refuses startup. The check does not create or modify storage.

The refusal covers the **whole account-wide product bucket**, including other authorities,
terminal-looking records, malformed bytes, historical messages and tombstones. It intentionally
parses no key, phase or claimed outcome. It does not declare an unknown effect safe.

Keep nonempty old storage intact. The supported fresh-start boundary is separately provisioned
storage/account for a new deployment, with old writers stopped and old data preserved. Changing only
a platform stem in the same account does not bypass the check. This PR offers no deletion command,
ignore/reset flag, automatic migration or terminal-outcome resolver. Reusing nonempty state requires
separate, independently proven handling; none is provided here.

## Evidence status

[Versioned results](results.json) record command outcomes and counts separately from historical ledgers.
The pin remains `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. Providers, profiles, model identity, dimensions and
preprocessing remain fixed for comparisons. Frozen fixtures, `test/setup03a` bytes and old JSON ledgers
remain unchanged. New changed-contract process checks receive separate evidence paths and identities.

The final binary is `semsource-final`, SHA-256
`b5ffe3710d161cd3246209aff127e833832ede7ec6c3e0c4f52e612f4d48be87`, built from the clean committed
source above. Commit attribution comes from [the captured build/worktree record](build.json): the binary does not
embed `vcs.revision` or `vcs.modified`. Later documentation changes do not change those build inputs.

| Evidence or gate | Recorded result |
| --- | --- |
| Architecture reassessment | [audit.md](audit.md); private-authority approval withdrawn |
| Corrective contract review | Approved before code at `0221635` |
| Behavioral red/green and real-NATS guard | PASS; isolated retained-state cases, refusal before provisioning |
| Publisher/producer review | [Approved](reviews/publisher.md); independent race run across 16 packages |
| Runtime Go / architecture review | [Go](reviews/runtime-go.md), [graph](reviews/runtime-architecture.md) approved |
| Process-test and CI logic review | [Approved](reviews/process.md); final desired-state workload PASS |
| Final source unit/race | PASS: 2,555 test/subtest pass events, 9 existing conditional skips, 64 packages |
| Broad integration/race | PASS: 2,674 pass events, 9 existing conditional skips, 65 packages; scope below |
| Local e2e / Garage | PASS: 7 / 61 test pass events |
| Final lint / workflow contracts / agents / strict OpenSpec | PASS |
| Six new production files: critical coverage | [99/114 statements, 86.8%](critical-coverage.json); ≥80% gate met |
| Final desired-state original source | [33/33](reports/desired-original.json), PASS |
| Final desired-state runtime-added source | [37/37](reports/desired-runtime_added.json), PASS |
| Final unchanged BM25 corpus | [61/63](reports/bm25-corpus.json), exit 1; two known broker-restart failures |
| Final unchanged original removal probe | [9/10](reports/historical-removal.json), exit 1; marker failure retained |
| Protected historical inputs | [All 29 files](baseline-preservation.json) remain byte-identical |
| Remote PR CI | Pending; local gates do not imply hosted CI completion |

The final desired-state run exits 0 in 213.3 seconds. Both cases prove exact retained envelopes,
restart-only activation, explicit unavailable projection, unchanged target/sibling facts and content,
no private bucket creation, and checked graceful application exits. These are changed-contract passes,
not source-removal-positive or freshness passes.

The final BM25 failures are `broker_restart_reingested_exact_relationship` and
`broker_restart_reingested_exact_content`, unchanged known-at-pin failures tracked by
[SemStreams #1442](https://github.com/C360Studio/semstreams/issues/1442) and
[SemEngine #15](https://github.com/C360Studio/semengine/issues/15). The RPC stream-collision probe and
ordinary file/application restart checks pass. This is no-provider BM25 comparison, not full neural or
semembed requalification; neither failure is skipped or converted to a pass.

The broad integration, e2e and Garage runs preceded the final narrow error-message fix. That fix only
preserves the joined authoritative-readback cause in `KV_WRITE_FAILED`; final focused readback and
independent integration review passed afterward. Final unit/race and lint ran after it. Package entries
with no test files are distinct from the nine conditional test skips: missing embedding-endpoint fixtures,
document exploratory probes, a C cross-translation-unit measurement and an optional real Svelte fixture.

A [separate approved test-only addendum](reviews/desired-readback.md) exercises the root retained-config
reader against real NATS: exact envelopes, missing/malformed values, namespace isolation and closed reads.
No production code changed. Coverage across **all six new production files** is 99/114 statements (86.8%),
not whole-repository coverage. Unit/integration coverage alone is 79.8%; a separate instrumented binary
reran only the original-source process case (33/33, exit 0, 92.01 seconds) to cover root wiring. The
[union profile](critical-path.cover) and [measurement](critical-coverage.json) retain that distinction.
The primary `b5ffe371…` binary and its 70-check desired-state qualification are unchanged.

The [upstream dependency closure](dependency-results.json) is unchanged in both measured test-tag sets:
consumer production 100 packages / 214,818 non-test lines; narrow direct imports 48 / 81,687;
proposed port roots 65 / 126,926;
root test dependencies 112 / 229,805; all retained-package tests 114 / 232,126. These count unique upstream
source directories and raw non-test Go lines, including inactive build-tag files. Removing SemSource's
private authority does not shrink the pinned SemStreams closure. [Production measurements](implementation-size.json)
show 254 → 233 non-test Go files and 53,233 → 49,807 physical lines: **3,426 net lines removed**.
This is local product-code reduction, not a reduction of upstream retained packages.

The final unchanged removal probe is 9/10, exit 1. Its only failure is
`removed_parent_and_passage_retained_with_source_removed`: retained parent/passage markers are absent,
as the corrective contract declares automatic projection unavailable. The original assertion stays
failed; this is an intentional changed-contract limitation, not a passing positive removal test.

The historical missing-marker probe was 9/10 before the private recovery addition; its later 10/10
result remains recorded against the old binary. A new unavailable-capability check is not a pass of
that unchanged marker assertion. Any new failure is recorded and attributed, not skipped or relabeled.
Historical #215 selective reactivation remains 24/26 with two failures. The unchanged legacy removal
harness does not assert clean process exit; it cannot supply checked-exit proof. The separate corrective
workload checks graceful application replacement against the same broker. It does not inject crashes,
ambiguous commits or broker restarts, or qualify authentication through its trusted-network process
fixture. Retained document bytes are compared through the existing content object store, not a new
public body-retrieval API.

Earlier process failures are preserved in [attempts.json](attempts.json): the old-binary negative control
was 7/12, and preliminary new-harness runs were 30/32 and 34/36. Only the new test
was corrected to decode the pinned legacy RPC error envelope; independent review then strengthened
HTTP error and exact-envelope assertions. No frozen test or earlier outcome was rewritten. Versioned
reports and hashes identify the evidence; full raw logs under `/tmp/semsource-lifecycle-correction/`
remain temporary session artifacts, not a permanent log store.

## Framework decisions and next work

[SemEngine #18][e18] must first decide the required durable-done contract; a universal accepted-input
census is not assumed. [#19][e19] remains the narrow caller-observed conditional-CAS question.
[#20][e20] separates repair of existing `CommitUnknown` classification from any new terminal lookup or
fencing authority. These requests do not collectively justify a consumer recovery subsystem.

No SemStreams repair or pin move is part of this correction. #215 stays open/deferred for its positive
projection/freshness contract. Future 04A remains gated by approved 03B contracts and an admitted engine
SHA, on a separate branch wholly using SemEngine. Full semembed-backed 04C still gates mainline cutover.

[pr]: https://github.com/C360Studio/semsource/pull/223
[design]: ../../../openspec/changes/remove-private-source-recovery/design.md
[baseline]: ../setup-03a/merge-readiness.md
[e18]: https://github.com/C360Studio/semengine/issues/18
[e19]: https://github.com/C360Studio/semengine/issues/19
[e20]: https://github.com/C360Studio/semengine/issues/20
