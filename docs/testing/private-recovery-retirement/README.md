# Private source recovery retirement

The owner authorized [corrective PR #223][pr] after the [architecture reassessment](audit.md).
The [approved corrective contract][design] is committed at
`0221635b7cb42cfc6d443fd492a7941cb58b6397`, based on merged #213
(`3604a9ce8d5aec717253d7a905389f7910efd25d`). Independent architectural review approved this contract.
**Implementation, runtime qualification and final implementation review are pending.** This document
states the approved target, not a claim that the running binary already implements it.

The earlier architectural approval of the private recovery design is withdrawn. Historical review,
passing tests and blocked acceptance remain attributable to their original snapshots. This correction
does not invalidate every migration change or rewrite any [frozen SETUP 03A result][baseline].

## Approved behavior

Remove the coordinator, journal, boot epochs, publication receipts/seals, effect fences and periodic
replay. There is no replacement worker, durable command lane, state machine or recovery service.
Keep the existing ConfigManager authority, complete `Enabled:false` envelopes, desired manifest/count
repair, deterministic handles and actual producer Stop/join. Desired source changes must work when
configuration storage is healthy even if the graph is degraded.

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
No positive #215 acceptance is claimed. MCP descriptions and results must express the same distinction.

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

The pin remains `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. Providers, profiles, model identity, dimensions and
preprocessing remain fixed for comparisons. Frozen fixtures, `test/setup03a` bytes and old JSON ledgers
remain unchanged. New changed-contract process checks receive separate evidence paths and identities.

| Evidence or gate | Status |
| --- | --- |
| Architecture reassessment | Recorded in [audit.md](audit.md); earlier private-authority approval withdrawn |
| Corrective contract independent review | Approved before code at `0221635` |
| Corrective production source / binary identity | Pending |
| Behavioral red/green tests and real-NATS guard proof | Pending |
| Fresh boot / desired remove and re-add / checked-exit process evidence | Pending |
| Unchanged original source-removal probe | Pending; missing-marker expectation must not be rewritten |
| Unit/race, integration, e2e, Garage, lint, agents and OpenSpec gates | Pending |
| Final independent component and graph/event implementation review | Pending |
| Final dependency and deletion-scope measurements | Pending |

The historical missing-marker probe was 9/10 before the private recovery addition; its later 10/10
result remains recorded against the old binary. A new unavailable-capability check is not a pass of
that unchanged marker assertion. Any new failure is recorded and attributed, not skipped or relabeled.
Historical #215 selective reactivation remains 24/26 with two failures.

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
