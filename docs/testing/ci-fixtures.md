# Reproducible S3 integration fixtures

[Issue #214](https://github.com/C360Studio/semsource/issues/214) is being addressed through
[draft PR #217](https://github.com/C360Studio/semsource/pull/217), independently of migration #213.
The approved fixture preserves the exact existing MinIO release through the Thanos mirror. Artifact
provenance and a cold pull are verified. Mainline full integration, combined Garage, unit-race, lint,
and workflow-contract checks pass on local ARM64. Remote CI and combined Garage pass on AMD64,
with independent implementation and runtime-evidence approval.
[Structured evidence](ci-fixtures-results.json) separates these passes from the remaining migration holds.

## Exact artifact

| Field | Pin |
| --- | --- |
| Repository | `quay.io/thanos/minio` |
| Release | `RELEASE.2025-09-07T16-13-09Z` |
| Index digest | `sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e` |
| Upstream source commit | `07c3a429bfed433e49018cb0f78a52145d4bedeb` |
| Source tag object | `01ce918d8279a20e4706b96a64396146894adee4` |

The fixture reference combines the repository, release, and index digest. The release makes the
intended version readable; the digest fixes the selected bytes. The multi-platform index is distinct
from each architecture's manifest and from a local Docker image ID.

| Platform | Child manifest digest | Verified scope |
| --- | --- | --- |
| `linux/amd64` | `a1a8bd4ac40ad7881a245bab97323e18f971e4d4cba2c2007ec1bedd21cbaba2` | CI suites pass |
| `linux/arm64` | `9966a92a734f9411e32f4f41d7d9d826fcdc0f68c4e20b70295bd4e7c11f8a2f` | Local suites pass |

Both child values are SHA-256 digests. Their response bodies matched the requested digests, and all
20 referenced layer/config HEAD requests returned HTTP 200. ARM64 local suites and AMD64 CI pass.
The AMD64 Garage job explicitly records the architecture, exact digest startup, 83 passing checks,
no failures or skips, and termination of both owned containers; independent review approves this proof.
The index also lists `linux/ppc64le`; this work has not verified its child, blobs, or runtime and makes
no qualification claim for it.

## Why this mirror preserves the fixture

The [Thanos mirror change][thanos] records maintainer `bwplotka` mirroring MinIO into the public Thanos
Quay namespace; it merged on September 15, 2026. The [IBM withdrawal advisory][ibm] identifies the
original `quay.io/minio/minio` reference with the identical index digest. Historical official MinIO
Docker Hub metadata, linked in the [provenance record](ci-fixtures-results.json), corroborates the same
index and ARM64 child digest.
Those historical records establish content correspondence; they do not establish current pull
availability from the withdrawn original repositories.

The architect separately verified the live mirror index and amd64/arm64 manifest body hashes and
blob availability on September 30, 2026. The parent then verified the image was absent locally,
pulled it successfully, and ran the ARM64 version command. It reported the pinned release, source
commit above, and Go runtime `go1.24.6`. No source build or manual preload is part of the fixture contract.

This is an archived integration fixture. It does not establish a maintained MinIO distribution or
promise indefinite registry availability. The source tag's GitHub verification is recorded; no image
signature verification was performed. Registry responses were inspected but not archived as separate
raw files. The structured record states these limits alongside the successful identity checks.

[thanos]: https://github.com/thanos-io/thanos/pull/9029
[ibm]: https://www.ibm.com/support/pages/node/7289585

## Running the retained cases

Ordinary test invocations acquire the pinned image through the
existing testcontainers path. Docker must be available; there is no required source-build or manual
image-preload step. Keep both commands and all their cases:

```bash
go test -count=1 -tags=integration ./...
go test -count=1 -tags=integration,garage -v -timeout 15m ./storage/s3store/
```

The second command deliberately runs both MinIO and Garage suites. Preserve the Garage image,
bootstrap, and backend-specific assertions. A shared MinIO change must select the Garage workflow
through its `internal/miniotest/**` path filter. Do not repair acquisition by dropping cases or adding
fixture-driven skips. Setup failure remains a failed selected suite.

## Failure and cleanup contract

The [current fixture capability](../../openspec/specs/integration-fixture-reproducibility/spec.md)
records the retained requirements. The completed fixture change is archived separately from the
active SETUP 03A migration.

Acquisition, startup, readiness, and cleanup are bounded. Readiness uses an observed health/API
signal rather than a fixed sleep. Each fixture owns its container and data; cleanup must not affect
unrelated resources or rely on a pre-existing developer service.

A failed start can still return an owned container handle. That handle must be terminated before
returning the failure. `RunTests`/`TestMain` must preserve an existing nonzero test outcome and make
an otherwise-green suite fail if teardown fails. Governance tests explicitly terminate their shared
MinIO fixture. Failure diagnostics must distinguish setup, readiness, test assertions, and cleanup.
The named lifecycle regressions and independent reviewer race checks pass for these requirements.

## Recorded checks

| Check | Result | Scope |
| --- | --- | --- |
| Existing case inventory | 29 files, 76 test functions retained | Assertion-bearing files byte-identical |
| Mainline full integration | Pass, 98.867 s | 2,533 passes including subtests; 9 existing non-fixture skips |
| Combined MinIO/Garage | Pass, 37.382 s | 83 passes; zero skips or failures |
| Full unit race | Pass, 41.81 s | Entire mainline package set |
| Final lint | Pass, 4.09 s | Earlier context-parameter ordering warnings corrected |
| UI image/workflow shell contracts | Pass | `task ui:image:release:test` |
| Independent implementation review | Approved | Mainline implementation, ARM64 local and AMD64 CI evidence |
| Implementation CI and Garage | Pass | Exact jobs and reviewed AMD64 runtime evidence below |

The only changed existing storage/governance test file in the inventory is
`storage/s3store/main_test.go`, which owns cleanup. New lifecycle regressions add coverage for partial
startup, bounded cleanup/retry, retained nonzero exits, and attempting both backend cleanup paths.
The MinIO request-contract test checks the fixed digest and bounded live-health gate; no separate
selectable-artifact validator is claimed.

The nine full-suite skips are existing cases outside fixture restoration: three optional model-registry
configurations, four document/model probes, one C measurement, and one absent legacy Svelte fixture.
There are **zero storage/governance skips**. Acquisition is not being hidden by test exclusions.

The [planning-only Actions run][planning-ci] was cancelled and is not qualification evidence.
At implementation commit `cba2279624e547d10096bcdbdf484bb2d3a04116`, [CI integration][integration-job]
and [combined Garage][garage-job] succeeded. The same CI run passed lint, unit race, local E2E, and UI
quality; Quickstart and UI development smoke workflows also pass. Publish-only jobs are intentionally
skipped. Independent review confirms the AMD64 runtime identity and complete combined Garage pass.
[PR #217 checks][implementation-ci] show subsequent commit status; these links record the tested revision.

[planning-ci]: https://github.com/C360Studio/semsource/actions/runs/36787797040
[implementation-ci]: https://github.com/C360Studio/semsource/pull/217/checks
[integration-job]: https://github.com/C360Studio/semsource/actions/runs/36789209485/job/110137833358
[garage-job]: https://github.com/C360Studio/semsource/actions/runs/36789209271/job/110137832100

## Validation boundaries

The independent fix starts from main commit `34bda6406fb06fd723a040988647b204204a1583` with
SemStreams `v1.0.0-beta.161`. Implementation commit `cba2279624e547d10096bcdbdf484bb2d3a04116`
passes CI and combined Garage. All required local gates and independent implementation review pass.
The old fixture's acquisition failure remains recorded separately. This patch restores the original
release's availability and cleanup observability without changing product storage behavior.

A separate migration overlay applies only the eight fixture/workflow/test-ownership files at commit
`8676299c0df129561132cfda4d61c69c2466106b` over frozen migration commit
`75a17f7d6297c3fa18102f3e09f8a0a4a10750bf`. Its module pin is unchanged, and the migration's
`internal/sourcespawn` CI entry is retained. Its full integration run completed with 2,612 passes,
nine existing non-fixture skips, and three AST authority-input setup failures. All 64 S3, 49 governance,
and 47 source-spawn checks passed without fixture skips. Combined MinIO/Garage passed all 83 checks
in 15.181 s. The three AST failures reproduce on pristine migration commit `75a17f7`; the fixture
patch did not cause them.

[Issue #218](https://github.com/C360Studio/semsource/issues/218) tracks the separate correction of
migration test authority inputs and AST integration CI selection in PR #213. Independently reviewed
correction `c8a74e7208c089f128b524b63fa5f88d8b9952b4` supplies the required authority to two test
constructors and retains the AST package in CI. It changes no product behavior.

Corrected overlay `ee39cc22901edce3ec09d5a7faa0100da65e008a` has an identical tree to PR #213 with
that correction and the approved fixture, commit `8d104988e4fb7924b5dffc4f54f7254eeaa8fb9a`.
Its full integration run passes in 92.717 s: 2,615 passes, nine existing non-fixture skips, and zero
failures. S3 (64), governance (49), source-spawn (47), and AST (60) checks all pass without fixture
skips. Combined MinIO/Garage passes all 83 checks in 12.917 s. The identical PR #213 tree also passes
the full unit-race suite (25.723 s) and final lint (7.688 s). Independent Go review approves the
identical-tree results, unchanged pins, and termination of both owned fixture containers. The shared
tree is `db27494ec31762f0351ed43062b104d3693453fe`. This closes the fixture and test-setup gaps only.
The first failed overlay and pristine migration reproduction remain recorded separately.

The frozen SETUP 03A results remain historical evidence. Restored fixtures do not resolve
source-removal replay, discovery, broker-generation recovery, capacity, or SemEngine cutover holds.
