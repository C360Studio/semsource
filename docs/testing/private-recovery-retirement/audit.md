# Architecture reassessment and withdrawal

The 2026-10-01 audit inspected SemSource `533b62a74eae2ec93776cc316f69d9b55bf91cea`, whose production
code matches merged #213 (`3604a9ce`) and the earlier `ce241a5` implementation. The independent Go,
evidence/history and architect reviews withdrew architectural approval of the private recovery design.
The owner authorized removal under [PR #223][pr], without a replacement recovery system. The approved
[corrective contract][design] is separate from [pending implementation evidence](README.md#evidence-status).

## What the earlier review missed

[SemStreams #1147][ss1147] requires settlement first, then stable identity/reconciliation, then registered
storage references. Additional persistent authority needs a named failure proving those insufficient.
Earlier reviews tested whether the implementation obeyed the selected journal/fence contract. They did
not establish whether that contract was necessary. The original missing-marker reproduction proved a
product behavior gap; it did not prove the need for another state machine and per-publication ledger.

The storage choice in `replay-source-removal/design.md:54–63` relied on finite ordinary stream age and
an indefinitely delayed restart. It omitted comparison with already-retained disabled configuration,
and the pin's explicit owner/reason archival stream declaration. That omission does not prove a new
command stream is appropriate; the corrective contract introduces none.

The foundation was available: [#759][ss759] and [#1146][ss1146] are closed in the captured tracker state;
`natsclient/delivery_settlement.go` exists at the frozen pin. Graph-ingest has its own apply/guard/ACK
policy and is not thereby qualified for every stronger guarantee. The open umbrella epic was not proof
that settlement was absent. SemEngine's setup plan and issue #8 already referenced #1147; the accurate
finding is a missing source-removal-specific comparison, not an absence of references everywhere.

The withdrawn core was 2,626 physical non-test Go lines: `internal/sourcelifecycle` 1,739,
`internal/sourceintent` 548, and `internal/entitypub/receipt.go` 339. Complete #215 contract/implementation
changes added 4,168 and deleted 280 lines across 55 Go files (`2fdf5f8` → `ce241a5`). Counts include
comments/blanks, exclude tests/fixtures and do not imply every line was inappropriate. Ordinary source
staleness, publisher retry/drain and the desired `Enabled:false` workaround existed before this core.

## Evidence and its limits

- Original missing marker: the 9/10 process probe proves an external behavior gap, not an architecture comparison.
- Historical retained-marker repair: process retirement passes prove useful behavior of the selected design,
  not the necessity of its authority.
- Crash after fence before dispatch: an independent fake persistence/replacement test observes zero graph calls,
  then a new coordinator still blocks Add and unrelated Remove.
- Graph failure before desired disable: injected unavailable/degraded tail causes zero healthy config writes.
- Recurring work/state growth: code and a focused two-pass test establish structural cost, not a scale benchmark.
- Settlement/replay/existing-state alternatives: no inspected named proof defeats them; the burden remained unmet.

The independent overlay ran under `-race` and passed in 1.334 seconds:
`TestAuditCrashBeforeDispatchBlocksDisjointAdmissionAfterReplacement` and
`TestAuditGraphFailurePreventsDesiredDisable`. These are deterministic in-memory seam experiments,
not real broker crashes or backend lost-PubAck injections. Existing focused lifecycle tests passed
in 1.345 seconds and the actual Add-guard test in 1.524 seconds. They corroborate the implementation's
admission behavior; they are not corrective implementation results.

Old process facts remain intact: original probe 10/10, sibling restart 8/8, original retirement 16/16,
runtime retirement 20/20, rapid re-add 27/27 and selective reactivation 24/26. The combined run exited 1.
Audit re-reading confirmed all six raw reports matched the versioned hashes. The two failed freshness
assertions stay failed. [Historical final results][results] retain their exact source/binary attribution.

## Keep product semantics; remove unsupported authority

Desired configuration, deterministic identity, retained content, ordinary file/passage staleness,
truthful partial errors and owned producer shutdown remain product responsibilities. The corrective
contract deletes private execution/completion authority and defers source-removal projection and
positive reactivation. It does not claim the older best-effort RPC was sufficient or restore it.

Existing unknown records cannot simply be deleted or treated as no-commit. The narrow startup check
refuses any nonempty old bucket without interpreting outcomes. Old writers must stop; old bytes remain
preserved. A separately provisioned fresh deployment is not an automatic migration or proof that an
old remote effect resolved. There is no compatibility recovery worker or override flag.

## Separate framework questions

- [SemEngine #18][e18]: first decide durable done and whether an all-accepted-input obligation is needed.
  Missing census evidence does not automatically justify another consumer ledger.
- [#19][e19]: the pinned public reconcile helper re-reads instead of honoring a caller-observed revision.
  The narrow conditional-CAS gap remains valid; no live race was induced in its filing.
- [#20][e20]: `CommitUnknown` already exists. The narrower code-contract finding is that generic classified
  backend errors can become definite non-commit. A classification repair and new terminal-resolution or
  generation-fence authority are distinct decisions; no live lost-ack failure was induced in the filing.

The broker-generation (#15), RPC boundary (#16) and desired-config (#17) findings remain independent.
This reassessment does not erase the original known-at-pin failures or authorize substrate changes.

## Audit artifact identity

Raw reports/logs remain temporary session artifacts under `/tmp/semsource-lifecycle-audit/`, not a
permanent artifact store. These recorded SHA-256 values identify the inputs to this compact record:

| Artifact | SHA-256 |
| --- | --- |
| `architect-review.md` | `198633aaace4da5d40bc0920a66434c7f084db385ed7a89db6f62e66ef00d286` |
| `independent-go-review.md` | `4c29b1d8bee6a335997bcd67ba184d5f9e49ea3473cb46aba92b76ea25717d0c` |
| `evidence-review.md` | `94c9e4b8d2160c3ce0983c933ff7fd4a7e9015d6982f8f52252816e05db606e9` |
| `overlay-results.log` | `0228c39c743f1cec3d21c95e1977e82a3ce73e613c0a1759e899fb48541d6a0d` |
| `existing-focused-results.log` | `adfc1b0511a2bb14313ea7a3806cd469483b9fc891102f773a4ea8395a82041d` |

[pr]: https://github.com/C360Studio/semsource/pull/223
[design]: ../../../openspec/changes/remove-private-source-recovery/design.md
[results]: ../source-removal215/final-results.json
[ss1147]: https://github.com/C360Studio/semstreams/issues/1147
[ss759]: https://github.com/C360Studio/semstreams/issues/759
[ss1146]: https://github.com/C360Studio/semstreams/issues/1146
[e18]: https://github.com/C360Studio/semengine/issues/18
[e19]: https://github.com/C360Studio/semengine/issues/19
[e20]: https://github.com/C360Studio/semengine/issues/20
