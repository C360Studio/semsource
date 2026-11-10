# Qualify SemSource on SemEngine Tier 0

Approved-by: Coby Leuschke (2026-11-10) sha256:360cb9ee7418

## Why

The SETUP 03A SemStreams baseline merged in #213 as
`3604a9ce8d5aec717253d7a905389f7910efd25d`. SemSource's next substrate work is the 04A graph-foundation
consumer qualification tracked by #221 and SemEngine #9. SemEngine 03B contracts and port admission
were approved in SemEngine #8 / merged PR #21 (`9286055e2c9a18426a73da38a60766845ab9e48e`), and the
pre-release consumer pin `v0.1.0-alpha.3` was tagged at `057bc8bfe2f737000a354fd1a38aa60d65cbf9f1`
(2026-11-10) after change 5 (#550, #558, #565, #577) and the composed smoke test (#557 / PR #560)
merged. A consumer plan records the work, the consumer pin now available for branch integration, and
the remaining Slice 04A prerequisites without treating a pre-release pin as Tier 0 qualification.

## What Changes

- Claim the consumer-side qualification plan on a separate branch based on the merged baseline.
- Inventory the existing composition and the structural slice of `test/setup03a`; record the changes
  needed for a true no-embedder composition only after the relevant engine contracts are admitted.
- Define the evidence and stop conditions for the eventual SemEngine-only qualification binary.
- Preserve the frozen SemStreams reference, failed expectations, provider controls and baseline results.

This planning commit changes no runtime, module pin, test expectation or fixture. `v0.1.0-alpha.3`
supplies the pre-release consumer pin for branch integration (#221, #231, #232, #253, #256, #257);
full 04A Tier 0 qualification still awaits the remaining Slice 04A deliverables under SemEngine #9.

## Capabilities

### New Capabilities

- `tier0-consumer-qualification`: contract-gated, exclusive SemEngine consumer composition and exact
  graph-foundation corpus evidence, separate from lexical/neural admission and mainline cutover.

## Impact

SemSource owns its composition, source adapters, public consumer interfaces and known-answer workload.
SemEngine owns the substrate, public contracts and extraction admission. SemTeams, SemDragon
and other consumers keep the existing SemSource mainline interfaces while this separate branch is
prepared. Implementation scope remains subject to approved 03B contracts and independent review.

## Non-goals

- Implementing or editing SemEngine's claimed 03B design, extracting its packages, or patching SemStreams.
- Guessing missing engine APIs, changing the reference pin, or importing both substrates in one binary.
- Beginning code migration before 03B approval and a usable admitted Tier 0 source commit exist.
- Treating known-at-pin failures as passes or carrying their classification into 04A acceptance.
- Qualifying BM25/neural/generation capabilities at Tier 0 or switching mainline before full semembed/04C.
