# Independent source-removal implementation review

The reviewed consumer slice is approved for inclusion in draft migration PR #213. It does not qualify
terminal all-input removal, selective freshness, publication withdrawal, unknown-effect recovery,
SemEngine extraction admission, merge, or mainline cutover.

## Independent scopes

The reviewer who implemented publisher/producer changes independently reviewed the journal,
coordinator, source-manifest integration, composition-root wiring, shared contracts and supersession.
The supersession implementer independently reviewed the publisher, fingerprint, seed-proof, source
factory and nine producer/handler changes. Neither reviewer approved their own implementation.
Both used the repository's Go component and graph/event reviewer contracts. The exact production
build reviewed and exercised by the final process suite is clean commit
`ce241a5c6e07f69e5450bb60885b537e89ea27a9`, SHA-256
`09170e24f60af0ead2b209ce58bf9a4c4446316cc77e9c5fbe6f8a2da63b9421`.

Final independent commands all pass:

```bash
go test -race -tags=integration ./internal/sourcelifecycle ./processor/source-manifest ./cmd/semsource -count=1
go test -race -tags=integration ./processor/supersession -count=1
go test -race ./internal/entitypub ./handler/video -count=1
```

The producer review also ran the full 19-package affected scope under race detection before its two
localized fixes, then re-reviewed and retested those fixes. Critical lifecycle package coverage is
81.8%; publisher coverage is 85.8%, fingerprint contract 85.3%, and seed proof 100%. Supersession safety
paths cover 88.6–100%; its whole-package coverage is 65.9% because the existing unrelated correspondence
code is not this slice's critical path.

## Findings resolved before approval

- An unreadable generated video frame reported the wrong nil error. Bound seeds now fail proof while
  unbound best-effort ingestion retains its established behavior.
- Rejected automatic live publications retained finished batch bookkeeping. Failed batches now release
  only their own allocations and retain honest failure accounting.
- Idempotent retained passes reset completed progress to zero. Successful complete passes retain the
  verified enumerated count; partial/error/canceled passes count only confirmed effects.
- A changed same-handle configuration could commit before reactivation journal CAS, preventing the
  next boot. Desired repair now advances that enabled changed-digest history before producer binding.
- A timed-out product lifecycle RPC could mutate after re-add. Synchronous single-bound local calls
  now retain gate ownership; old RPC and legacy source_removed requests refuse mutation. Exact durable
  per-entity effect fences precede remote writes and block new generations on unresolved outcomes.

Channel-controlled and crash-boundary regressions cover these findings. Terminal journal evidence
survives a resolution commit followed by a lost acknowledgement; graph readback cannot resolve an
unknown remote effect. Stop seals admission and joins direct calls before releasing client ownership.

## Test-only follow-up and final gates

Commit `16e3795` changes tests only. The fingerprint fixture avoids a retired literal that triggered
the repository's predicate migration audit. The governance integration test preserves file-delete,
recreation and retained-history assertions while replacing the now-forbidden active-producer
`source_removed` RPC with the actual journal/tail/coordinator/local-projector path. It explicitly
checks legacy refusal, enabled-boot retirement blocking, checked producer Stop, retained markers,
`applied_tail_unproven`, verified effect evidence and sticky markers under the later legacy sweep.

The governance case uses a stateful in-memory desired-config seam with real NATS and production
lifecycle owners; it does not claim ConfigManager durability or process restart. The process suite
proves those separate boundaries. Independent review approved this exact adaptation and reran
`TestIntegration_StalenessLifecycle` with integration tags and `-race`: PASS, 7.917 seconds, recorded in
`reviewer-governance-staleness.log`. The reviewed production executable was not rebuilt for these tests.

| Final gate | Outcome |
| --- | --- |
| Full unit/race | 2,705 passed; 9 named skips |
| Full integration | 2,826 passed; 9 named skips |
| Local e2e lane | 7 passed |
| Garage lane | 61 passed |
| gofmt, pinned revive/vet, agent sync, strict OpenSpec | Passed |
| Combined original/additive process qualification | Exit 1; selective reactivation fails 2 of 26 checks |

[Final results](final-results.json) retain commands, counts, skip names, initial failed gates and
corrected runs. [Process qualification](replay-qualification.md) records all six final case outcomes;
[dependency results](dependency-results.json) include the remeasured test closure. Green engineering
gates and implementation review do not close the failed positive process acceptance.

## Explicit review boundaries

- #1444: current retained markers may converge, but removal remains pending/applied_tail_unproven.
- #1445: matching reactivation proof never permits an unfenced clear; positive freshness remains open.
- #1446: unknown/internal/malformed/crash outcomes retain an effect fence and refuse generation changes.
  Only authoritative journal evidence of an already-proven terminal mutation can resolve that attempt.
- Continuing publication/withdrawal eligibility and large-number equality through the pinned graph
  decoder remain unqualified. Local JSON-number fingerprint tests are not end-to-end qualification.
- The file-local max-public-structs exception in sourceintent/contract.go is approved for the explicit
  shared value/interface contract, matching the pinned projection types precedent. No global lint
  rule or warning gate is weakened.
- AST-only dc.terms.created normalization in sibling comparisons is approved from preserved-before
  plain-restart evidence. It records indexing-time metadata separately; all other triples, lifecycle
  markers, IDs and exact content bytes remain strict. Failed original observations remain preserved.

Independent evidence is retained under `/tmp/semsource-removal215-evidence/`, particularly
`reviewer-primary-fence-race.log`, `reviewer-supersession-fence-race.log`, producer review regressions,
and the versioned [intermediate attempt ledger](implementation-attempts.json). Raw files are temporary
session evidence, not a durable artifact store. The versioned final ledgers
record their hashes and outcomes. All six owned final brokers were confirmed absent. The original
legacy probe does not assert its application wait error; only additive checked-stop cases provide
that exit evidence. This review does not turn a blocked positive assertion green.

## Final delivery evidence review

A separate implementation reviewer audited the final compact records against the raw session artifacts:
all 158 evidence hashes, binary identity, six process reports, twelve observed exits (ten graceful,
two deliberate SIGKILL), six removed broker IDs, corrected and original Go test outcomes, and all nine
dependency package/root/line sets match. The captured build record supplies commit/clean provenance;
the binary does not embed VCS revision fields. The test-only `16e3795` changes no production source.

This independent approval covers the delivery records for draft PR #213. Both selective reactivation
failures and all unresolved qualification gates remain visible. It does not approve issue closure,
merge, SemEngine admission or mainline cutover. Live publication and CI status are tracked on the PR.
