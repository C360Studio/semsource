# SETUP 03A fixture and integration follow-up

This is later evidence for [migration PR #213](https://github.com/C360Studio/semsource/pull/213).
It preserves the frozen [baseline](baseline-results.json), [pinned ledger](pinned-results.json),
[pins](pins.json), and their original outcomes. The original MinIO acquisition block was real; the
following separate patch and runs close that fixture evidence gap without rewriting those records.

## Independently repaired fixture

[Issue #214](https://github.com/C360Studio/semsource/issues/214) is addressed independently on the
beta.161 mainline base in [PR #217](https://github.com/C360Studio/semsource/pull/217), implementation
commit `cba2279624e547d10096bcdbdf484bb2d3a04116`. It restores the original MinIO release through
`quay.io/thanos/minio`, release `RELEASE.2025-09-07T16-13-09Z`, index digest
`sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`.
The exact upstream mirror provenance and architecture manifests are recorded in PR #217's fixture
guide and compact evidence file. This is an archived test fixture, not a maintained distribution.
No manual preload or source build is required.

The patch retains all 29 existing storage/governance test files and 76 test functions; assertion-bearing
files remain byte-identical. It adds bounded cleanup for partial startup, truthful teardown failure
outcomes, explicit governance MinIO cleanup, and Garage workflow selection for shared fixture changes.
Garage remains `dxflrs/garage:v2.3.0`, with its existing bootstrap and backend assertions.

Mainline full integration passes (2,533 pass events including subtests, nine existing non-fixture skips),
combined MinIO/Garage passes (83 checks, no skips), and unit-race, lint, and workflow contracts pass.
Independent Go review approves the implementation and local ARM64 runtime evidence. The
[AMD64 integration job][integration-job] and [combined Garage job][garage-job] also pass. Independent
review verifies explicit AMD64 architecture, exact pinned image startup, 83 passing checks, no skips or
failures, and termination of both owned containers. Other mainline PR workflows pass; publish-only
jobs are intentionally skipped.

[integration-job]: https://github.com/C360Studio/semsource/actions/runs/36789209485/job/110137833358
[garage-job]: https://github.com/C360Studio/semsource/actions/runs/36789209271/job/110137832100

## First overlay and attributed test defect

The first overlay, `8676299c0df129561132cfda4d61c69c2466106b`, applied only the eight fixture/workflow
and test-ownership files to frozen migration commit `75a17f7d6297c3fa18102f3e09f8a0a4a10750bf`.
Its full integration run completed with 2,612 passes, nine existing non-fixture skips, and three AST
setup failures. All 64 S3, 49 governance, and 47 source-spawn checks passed without fixture skips;
combined MinIO/Garage passed all 83 checks in 15.181 seconds.

The AST failures were missing authority inputs before test assertions. All three reproduced on
pristine `75a17f7`; the fixture patch did not introduce them. [Issue #218][ast-issue] tracks the
separate correction: supply the required platform to two AST test constructors and include the AST
integration package in CI. Independent Go review approved this test/workflow-only correction. The
original failed overlay and pristine reproductions remain retained, not relabelled as passing.

[ast-issue]: https://github.com/C360Studio/semsource/issues/218

## Corrected overlay results

The authority test correction is commit `c8a74e7208c089f128b524b63fa5f88d8b9952b4`. With the approved
fixture applied, the migration tree is commit `8d104988e4fb7924b5dffc4f54f7254eeaa8fb9a`. Corrected
validation overlay `ee39cc22901edce3ec09d5a7faa0100da65e008a` has an identical tree (verified by Git
diff), tree `db27494ec31762f0351ed43062b104d3693453fe`. SemStreams/provider pins and the
source-spawn regression CI entry remain unchanged.

| Gate | Result | Recorded scope |
| --- | --- | --- |
| Full integration | Pass, 92.717 s | 2,615 pass events; 9 existing non-fixture skips; 0 failures |
| Fixture/corrected packages | All pass | S3 64; governance 49; source-spawn 47; AST 60; no fixture skips |
| Combined MinIO/Garage | Pass, 12.917 s | 83 checks; no skips or failures |
| Full unit race, identical migration tree | Pass, 25.723 s | Uncached full package set |
| Final lint, identical migration tree | Pass, 7.688 s | Repository lint gate |

The nine skips are the same optional model-registry/document probes, C measurement, and absent legacy
Svelte fixture recorded on mainline. Neither S3 nor governance is excluded. Independent Go review
approves the final identical-tree results, unchanged pins, and termination of both owned fixture
containers. This approval closes the fixture and test-setup gaps; it is not blanket migration approval.

Raw commands, outcomes, retained failure runs, and exact tree records are under
`/tmp/semsource-ci214-evidence`: `migration-overlay.json`, `migration-correction.json`,
`migration-integration*`, `migration-original-ast*`, `migration-corrected-integration*`,
`migration-corrected-garage*`, and `migration-final-*`. PR #217's compact JSON records the selected
results and links to completed CI jobs; these local raw files are supporting session artifacts.

## What this evidence closes

These runs close the unavailable MinIO-backed integration/governance path and correct the previously
unselected AST test setup. They do not rerun or change the frozen known-answer, semembed, removal,
broker-recovery, or capacity comparisons. They do not resolve durable `source_removed` replay,
dynamic discovery, upstream broker-generation recovery (#1442), wildcard RPC collision (#1143), or
the guarded capacity limitation (#178). [The migration review](review.md) retains those holds.

SemSource continues to use only the pinned SemStreams substrate. Full migration merge/admission and
SemEngine cutover remain withheld; later SemEngine qualification requires its separate exclusive
branch and complete semembed-backed workload.
