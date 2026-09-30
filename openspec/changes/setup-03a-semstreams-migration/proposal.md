# SETUP 03A: pinned SemStreams consumer migration

## Why

SemEngine needs a measured SemSource consumer baseline before extraction. SemSource currently pins
SemStreams `v1.0.0-beta.161`; later upstream lifecycle, identity, and authority contracts must first be
adopted on SemStreams. SemEngine SETUP 03A may run beside its SETUP 01 and 02.

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
