# Independent source-removal implementation review

The reviewed consumer slice is approved for inclusion in draft migration PR #213. It does not qualify
terminal all-input removal, selective freshness, publication withdrawal, unknown-effect recovery,
SemEngine extraction admission, merge, or mainline cutover.

## Independent scopes

The reviewer who implemented publisher/producer changes independently reviewed the journal,
coordinator, source-manifest integration, composition-root wiring, shared contracts and supersession.
The supersession implementer independently reviewed the publisher, fingerprint, seed-proof, source
factory and nine producer/handler changes. Neither reviewer approved their own implementation.
Both used the repository's Go component and graph/event reviewer contracts.

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
and the versioned [intermediate attempt ledger](implementation-attempts.json). Full-suite and final
process outcomes are recorded separately; this review does not turn a blocked positive assertion green.
