# SETUP 03A baseline merge readiness

Current owner ruling, **2026-10-01**: prepare [SemSource PR #213][pr] for merge as the pinned consumer
baseline. Independent component/lifecycle and graph/event reviewers approve **baseline-only merge
readiness at `705ae66`** and identify no remaining consumer-owned safety blocker in that scope.
These are technical review dispositions, not GitHub human approvals or a record that merge occurred.
The PR owner records final repository checks and merge execution separately.

This additive ruling supersedes the historical baseline merge holds in [review.md](review.md) and
[compatibility.md](compatibility.md). Those documents and the frozen JSON ledgers retain the failures,
limits and review dispositions observed at their respective snapshots. No failed assertion is changed
into a pass, excluded from the harness or silently skipped by this classification.

## Accepted baseline and unchanged pins

SemSource remains wholly on SemStreams. The selected extraction/migration SHA remains
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`. Tag beta.163 was absent when the work began;
a later release does not move the baseline. [Pins](pins.json) also retain the fixed provider image,
served model/artifacts, vector dimensions and preprocessing used in the paired neural comparisons.

Baseline acceptance means the expected workload and both comparison points are recorded, differences
are attributed, dependency closures include tests, and independently reviewed consumer adaptations
are safe at the pin. It does not mean every intended substrate behavior passes. In particular:

- `broker_restart_reingested_exact_relationship` remains failed at beta.161 and the frozen pin.
- `broker_restart_reingested_exact_content` remains failed at beta.161 and the frozen pin.

Both failures reproduce in BM25 and neural runs. The recorded mechanism is retained applied-sequence
state rejecting changed-source ingestion after memory GRAPH recreation. Memory transport loss itself
is expected; suppressing valid re-ingestion is the defect. The owner classifies these exact cases as
**known-at-pin**, tracked by [SemStreams #1442][ss1442] and [SemEngine #15][engine15]. This classification
permits the measured 03A baseline to merge; it neither proves recovery nor changes the tests' failure
status or process exit. The original failed results stay in [baseline-results.json](baseline-results.json)
and [pinned-results.json](pinned-results.json).

The `Enabled:false` desired-component envelope remains the consumer workaround for file-defined
sources reappearing after deletion. Its removal/restart/re-add behavior is independently tested.
[SemStreams #1443][ss1443] maps to [SemEngine #17][engine17]; removing the workaround is not part of this
baseline delivery.

## Findings handed to SemEngine

The [setup plan's frozen-pin fix flow][plan-flow] directs repairs into SemEngine behind failing-first
tests. No SemStreams patch or post-pin synchronization is expected for this delivery. SETUP 03B must
choose retained contracts and repair-before-port obligations before extraction.

| Finding at the pin | SemEngine home | Required disposition |
| --- | --- | --- |
| #1442: reproduced stream-generation recovery defect | [#15][engine15] | Repair-before-port and recovery proof |
| #1143: reproduced wildcard stream/RPC collision | [#16][engine16] | Reserved RPC boundary and startup refusal |
| #1443: reproduced boot config overlay defect | [#17][engine17] | Config contract; retain workaround |
| #1444: missing supported applied-input barrier | [#18][engine18] | SETUP 03B contract decision |
| #1445: missing caller-revision conditional reconcile | [#19][engine19] | SETUP 03B projection contract decision |
| #1446: commit ambiguity and unknown-outcome resolution | [#20][engine20] | SETUP 03B projection contract decision |

The final three rows are not all reproduced live defects or promises that an API will be added.
#1444 is a contract request. #1445 is a verified API/TOCTOU gap; no live race was induced. #1446 is a
verified code-contract gap; no live lost-ack failure was induced. The architect may admit a primitive
or explicitly defer it with the consumer consequence recorded. [SemEngine #8][engine8] owns those
03B decisions; accepting the baseline in [SemEngine #7][engine7] does not approve its port set or APIs.

## Later evidence and open work

The original unavailable MinIO fixture and AST constructor-fixture gaps were closed by the separately
reviewed [fixture follow-up](fixture-followup.md). The original failed/skipped runs remain historical
evidence; current full integration includes those paths.

The later [source-removal results](../source-removal215/final-results.json) prove durable desired intent,
retirement before replay, complete retained enumeration, surfaced failures, effect fences and marker
repair. The captured clean-build record attributes the production binary to
`ce241a5c6e07f69e5450bb60885b537e89ea27a9`; the executable has no embedded VCS revision/dirty fields.
Later `16e3795` changes tests only, and `705ae66` records reviewed results. Its final process command exits 1:
selective reactivation remains **24/26**, with current A-only proof present but completion and A freshness
still failing. The unchanged original removal probe now passes 10/10; the final original/runtime
retirement cases pass 16/16 and 20/20, and rapid re-add passes 27/27. These later results do not rewrite
the frozen migration supplement's 9/10 result.

[SemSource #215][source215] stays open. A retained pass remains `pending` / `applied_tail_unproven`;
matching current publication evidence cannot safely clear a marker through the pinned public API;
and unknown remote effects remain fenced. Safe refusal and retained marker repair are reviewed
consumer behavior, not proof that terminal completion, selective freshness or unknown-outcome recovery
has been implemented. Continuing publication/withdrawal eligibility also remains unqualified.

[SemSource #178][source178], [#216][source216] and [#219][source219] remain open follow-ups. The guarded
OSH transport run does not reproduce or resolve the historic unguarded capacity regression. Dynamic
branch/remote-submodule discovery remains explicitly unsupported. Remaining implementation and proof
tasks stay open where their promised behavior has not been established. Under this owner ruling these
follow-ups do not block this measured baseline; they are not waived capabilities or completed tasks.

## Next consumer work and cutover boundary

The next SemSource implementation stage is **SETUP 04A Tier 0**, after the required SETUP 03B contract
and boundary approval. It belongs on a separate integration branch built **wholly on SemEngine**;
no binary may link both substrates. [test/setup03a](../../../test/setup03a/README.md) is the acceptance
corpus to carry forward, preserving known answers and explicit failure classification.

04A must prove a true **no-embedder composition** and the admitted structural interfaces. The current
baseline's structural checks run under SemSource `tier0-statistical.json`, which composes BM25;
those checks do not establish a no-embedder runtime. SemEngine Tier 0 and SemSource's numbered tier 0
are different profiles. Unsupported natural-language verbs cannot stand in for graph-only interfaces.

The admitted 04A graph-foundation workload must **pass both**
`broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content`.
The known-at-pin baseline classification does not carry into engine qualification. Lexical retrieval
belongs to 04B and neural retrieval to 04C; passing 04A does not require or claim their full workloads.

Mainline continues to use SemStreams through the lower engine profiles. The full semembed-backed
workload and retained lower-profile guarantees remain the **SETUP 04C** cutover gate, with exact
provider/model identity and independent review. This baseline ruling does not start that integration,
authorize new SemStreams fixes, or change mainline's substrate. Delivery stops at the merge handoff.

## Review record

| Scope | Current disposition |
| --- | --- |
| Component/lifecycle, snapshot `705ae66` | Approved for baseline-only merge readiness |
| Graph/event, snapshot `705ae66` | Approved for baseline-only merge readiness |
| Remaining consumer-owned safety blockers in that scope | None identified by either independent review |
| Full #215 positive acceptance and engine admission | Open; decisions and proving workloads remain required |
| Final GitHub checks and merge execution | PR owner records separately; not asserted by this document |

[pr]: https://github.com/C360Studio/semsource/pull/213
[ss1442]: https://github.com/C360Studio/semstreams/issues/1442
[ss1443]: https://github.com/C360Studio/semstreams/issues/1443
[engine7]: https://github.com/C360Studio/semengine/issues/7
[engine8]: https://github.com/C360Studio/semengine/issues/8
[engine15]: https://github.com/C360Studio/semengine/issues/15
[engine16]: https://github.com/C360Studio/semengine/issues/16
[engine17]: https://github.com/C360Studio/semengine/issues/17
[engine18]: https://github.com/C360Studio/semengine/issues/18
[engine19]: https://github.com/C360Studio/semengine/issues/19
[engine20]: https://github.com/C360Studio/semengine/issues/20
[source178]: https://github.com/C360Studio/semsource/issues/178
[source215]: https://github.com/C360Studio/semsource/issues/215
[source216]: https://github.com/C360Studio/semsource/issues/216
[source219]: https://github.com/C360Studio/semsource/issues/219
[plan-flow]: https://github.com/C360Studio/semengine/blob/3ae51c5/docs/setup-plan.md#package-admission-heuristic
