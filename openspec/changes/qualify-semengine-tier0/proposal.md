# Qualify SemSource on SemEngine Tier 0

## Why

The SETUP 03A SemStreams baseline merged in #213 as
`3604a9ce8d5aec717253d7a905389f7910efd25d`. SemSource's next substrate work is the 04A graph-foundation
consumer qualification tracked by #221 and SemEngine #9. SemEngine 03B contracts and port admission
are still in progress in SemEngine #8 / PR #21; current engine main supplies a harness, not a usable
production graph engine. A consumer plan can record the work and exact prerequisites without
inventing public APIs or starting extraction.

## What Changes

- Claim the consumer-side qualification plan on a separate branch based on the merged baseline.
- Inventory the existing composition and the structural slice of `test/setup03a`; record the changes
  needed for a true no-embedder composition only after the relevant engine contracts are admitted.
- Define the evidence and stop conditions for the eventual SemEngine-only qualification binary.
- Preserve the frozen SemStreams reference, failed expectations, provider controls and baseline results.

This initial planning commit changes no runtime, module pin, test expectation or fixture. The branch
still contains the reference SemStreams module because no engine contract/source commit is admitted
for consumption. It is not a Tier 0 build or an engine qualification result.

## Capabilities

### New Capabilities

- `tier0-consumer-qualification`: contract-gated, exclusive SemEngine consumer composition and exact
  graph-foundation corpus evidence, separate from lexical/neural admission and mainline cutover.

## Impact

SemSource owns its composition, source adapters, public consumer interfaces and known-answer workload.
SemEngine owns the substrate, public contracts and extraction admission. SemSpec, SemDragon, SemOps
and other consumers keep the existing SemSource mainline interfaces while this separate branch is
prepared. Implementation scope remains subject to approved 03B contracts and independent review.

## Non-goals

- Implementing or editing SemEngine's claimed 03B design, extracting its packages, or patching SemStreams.
- Guessing missing engine APIs, changing the reference pin, or importing both substrates in one binary.
- Beginning code migration before 03B approval and a usable admitted Tier 0 source commit exist.
- Treating known-at-pin failures as passes or carrying their classification into 04A acceptance.
- Qualifying BM25/neural/generation capabilities at Tier 0 or switching mainline before full semembed/04C.
