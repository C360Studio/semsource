# SETUP 03A: pinned SemStreams consumer migration

## Why

SemEngine needs a measured SemSource consumer baseline before extraction. At the start of this change,
SemSource pinned SemStreams `v1.0.0-beta.161`; later lifecycle, identity and authority contracts first
needed adoption on SemStreams. SemEngine SETUP 03A runs beside its SETUP 01 and 02.

## What Changes

- Freeze a versioned known-answer corpus and record behavior on SemSource
  `34bda6406fb06fd723a040988647b204204a1583` with SemStreams `v1.0.0-beta.161` before upgrading.
- Pin SemStreams main `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, selected because
  `v1.0.0-beta.163` was absent when work began. This SHA is the shared migration/extraction baseline.
- Adapt retained composition, NATS access, lifecycle ownership, type registration, and identity contracts
  using ADRs 094–107 and pinned implementation as the compatibility ledger.
- Rerun the identical corpus/profile matrix with fixed provider builds, actual model artifacts,
  vector dimensions, and query preprocessing. Attribute every difference or reproduce a defect.
- Measure production and test dependency closures; probe GRAPH capacity (#178) and RPC collision
  (SemStreams #1143), retaining explicit blockers when required evidence is unavailable.
- Deliver a migration PR for #212 with independent Go implementation and evidence review.

**BREAKING:** Upstream composition and lifecycle contracts change. Comparison runs use fresh isolated
storage; no cross-version retained-store compatibility is asserted. Restart durability is tested
within each revision against that revision's retained store.

## Current baseline acceptance ruling

The owner ruling of 2026-10-01 directs PR #213 to merge readiness as a measured **03A baseline**.
[The additive readiness record](../../../docs/testing/setup-03a/merge-readiness.md) supersedes historical
baseline merge holds without rewriting their evidence. Independent component and graph reviewers
approve baseline-only readiness at `705ae66`; this is technical approval, not a completed GitHub merge.

The exact broker-restart relationship/content failures remain failed and classified **known-at-pin**
under SemStreams #1442 / SemEngine #15. The `Enabled:false` config workaround remains for #1443 / #17.
No SemStreams fix is expected: the frozen-pin plan sends repairs into SemEngine behind failing-first
tests. Applied-input, conditional-reconcile and unknown-outcome gaps (#1444–1446) map to SemEngine
#18–20 for 03B decisions; they are not all reproduced live defects or predetermined API commitments.

Baseline acceptance does not close #215, prove recovery/capacity behavior still failing or unproven,
or complete open follow-ups #178, #216 and #219. Known-at-pin is an evidence classification, never a
skip or converted pass. Next consumer implementation is 04A on a separate SemEngine-only integration
branch with a true no-embedder composition and `test/setup03a` as its acceptance corpus, after 03B
approval. Full semembed-backed 04C qualification remains required before mainline cutover.

## Capabilities

### New Capabilities

- `consumer-migration-baseline`: versioned fixtures, exact pins, profile crosswalk, semantic/restart
  evidence, difference attribution, and reviewer acceptance for substrate migration.

### Modified Capabilities

- `runtime-configuration`: post-boot component changes become desired state with restart required.
  The implementation must report this honestly rather than implying live activation.

## Impact

SemSource owns source ingestion/projection, configuration adapters, public tools, and this workload.
SemStreams owns substrate defects. SemEngine consumes the baseline as extraction evidence; SemSpec,
SemDragon, SemOps, and SemTeams consume preserved SemSource interfaces. Changes may touch lifecycle,
bootstrap, config, publishers, tests, module pins, and current operator documentation.

## Non-goals

- Extracting SemEngine, changing its API, or importing it into this binary.
- Mixing substrates or switching mainline before exclusive SemEngine neural qualification passes.
- Claiming physical deletion where current source behavior retains stale history (#210).
- Admitting generation/community enrichment because a configuration has a higher tier number.
- Patching SemStreams or using reference-blind storage eviction for graph lifecycle.
- Counting missing provider/corpus artifacts or unrun workloads as passed gates.
