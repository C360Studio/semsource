# Independent publisher and producer review

Status: **approved for the frozen Worker B scope**. No blocking Go or architecture findings.
This is a scoped technical review, not GitHub human approval or final runtime acceptance.

## Scope and basis

Reviewed against `.agents/contracts/go-component-reviewer.md` and
`.agents/contracts/graph-event-reviewer.md` on `codex/remove-private-lifecycle` at plan commit
`0221635b7cb42cfc6d443fd492a7941cb58b6397` plus the frozen Worker B working-tree changes.
The exact reviewed current files are hashed in `reviewer-publisher-files.json` beside this report.
Other workers' changing root, manifest and supersession code is outside this approval.

The nine producer components and scoped handlers match the pre-private-recovery donor `fa5c250`.
Receipt, seed-proof and binding machinery has been removed. The scoped production tree contains no
`sourceintent`, `sourcelifecycle` or `seedproof` references. No replacement recovery authority, durable
proof ledger or freshness certification was introduced.

The remaining helper at `internal/entitypub/payload_ownership.go:13` owns the admitted JSON wire value
before enqueue. It has no durable state. Bypassing `EntityPayload.UnmarshalJSON` and decoding with
`UseNumber` preserves interface-valued numeric wire values rather than passing them through float64.
Storage references, indexing profile and triple metadata survive the snapshot. This is publisher-wire
ownership validation, not end-to-end graph numeric precision qualification.

`Publisher.Send` rejects invalid encodings before admission and reports them through dropped/lost
accounting. Transport retry bytes and deterministic message IDs, accepted-batch accounting, bounded
backpressure, cancellation and checked Stop settlement remain intact. The existing explicit handshake
shutdown test covers an active batch plus a queued tail. Producer cancellation/join behavior and
required body-store startup ordering survive restoration. Ordinary source error/status policies are
retained; receipt-specific health and loss categories are removed.

## Validation

Independently ran `go test -race -count=1` across `internal/entitypub`, six affected handler packages,
and all nine source processor packages: **16 packages PASS**, exit 0. Exact output is
`reviewer-publisher-race.log`. Also independently checked the scoped diff with `git diff --check`.

Read the failing-first `worker-b-red.log`: it reproduces post-Send caller mutation changing published
bytes and admission of nil/unserializable payloads before the helper. The new behavioral tests own and
mutate nested numbers, timestamps, storage and metadata after Send and verify the original wire bytes;
invalid admission is checked against pending/published/dropped accounting.

Read the developer's final `worker-b-final-checks.log`: scoped `go vet`, revive v1.15.0 with warnings
clean, and a final publisher race rerun all exit 0. These lint/vet runs were inspected, not independently
rerun by this reviewer.

Evidence SHA-256:

- `reviewer-publisher-files.json`: `a47abb5d7984ee38be88882ae3b4daf3f9e7362b97874fa47e52f9fcb39a080c`
- `reviewer-publisher-race.log`: `04374509a6dab110f3e13a76496c1896ad5cf7d4130b3ba018c7c16a07517be5`

## Remaining review boundary

Whole-runtime architecture, startup legacy-state refusal, desired-only API behavior and supersession
semantics require the separate Worker A review and final process evidence. This approval does not
claim source-removal projection, positive reactivation, all-input completeness or semembed qualification.
No production files were edited during this review.
