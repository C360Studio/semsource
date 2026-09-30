# Tasks: restore CI MinIO fixtures

Mainline implementation, local ARM64 validation, AMD64 CI, and the corrected migration-overlay
evidence are independently approved. The first failed overlay remains recorded. Completion closes
fixture and AST test-setup gaps only; all semantic migration and cutover holds remain separate.

## 1. Planning and verified artifact contract

- [x] 1.1 Read AGENTS/CLAUDE/OpenSpec context and inspect current MinIO/Garage fixture/workflow wiring.
  Verify: design starting-point references match the selected mainline checkout.
- [x] 1.2 Create proposal, design, tasks, and capability delta before implementation.
  Verify: `openspec validate restore-ci-minio-fixtures --strict`.
- [x] 1.3 Obtain architect sign-off on the verified mirror/digest and lifecycle contract.
  Verify: design records Thanos upstream-mirror provenance, manifest/child hashes, amd64/arm64 metadata,
  all referenced blobs accessible, and parent cold-pull evidence. Runtime qualification remains separate.

## 2. Additive fixture repair and regression coverage

- [x] 2.1 Capture the old clean-acquisition failure and inventory every selected MinIO, Garage, and
  governance case before changing fixture wiring. Verify: `before-minio.log` plus `retained-cases.json`:
  29 existing files / 76 tests retained; assertion-bearing files byte-identical.
- [x] 2.2 Add regression checks for the fixed image/readiness request and lifecycle failure paths.
  Verify: `TestStartUsesPinnedReleaseAndLiveHealthGate`, `TestStartWithoutContainerPreservesCreationError`,
  `TestFailedStartCleansContainerWithIndependentContext`, `TestRunTestsReportsTeardownFailure`,
  `TestRunTestsAttemptsEveryCleanupAndPreservesFailures`, and
  `TestTerminateRetainsFailedCleanupAndForgetsSuccess`; focused reviewer race checks pass.
- [x] 2.3 Adopt the reviewed exact-release mirror/digest and consistent CI/local selection.
  Verify: cold-pull identity is recorded and the new regression tests pass without changing Garage bootstrap.
- [x] 2.4 Preserve isolated ownership, bounded readiness, checked cleanup, and failure logs.
  Verify: setup failure with a returned handle cleans it; green-suite teardown failure fails the suite;
  red-suite teardown failure preserves failure; governance explicitly terminates its MinIO fixture.
- [x] 2.5 Keep both S3 backends and every existing assertion; wire all affected jobs without skips.
  Verify: compare before/after selected test inventories; Garage path filters include `internal/miniotest/**`;
  workflow contract tests pass with Garage bootstrap/image unchanged.

## 3. Independent mainline validation and review

- [x] 3.1 Run the full original integration suite with beta.161 unchanged.
  Verify: `go test -count=1 -tags=integration ./...`; no fixture-driven exclusions or skips.
- [x] 3.2 Run the combined compatibility lane against real MinIO and Garage.
  Verify: `go test -count=1 -tags=integration,garage -v -timeout 15m ./storage/s3store/`.
- [x] 3.3 Run the applicable repository gates: `task lint`, `go test ./...`, affected concurrent tests
  with `-race`, and `task ui:image:release:test` for workflow/Taskfile/script changes.
  Verify: command results are recorded, including any unrelated failures rather than waived checks.
- [x] 3.4 Record exact artifact/provenance identities, architecture and clean-cache evidence, commands,
  source commit, and CI job URLs; obtain independent Go reviewer approval of the mainline fix.
  Verify: local reviewer approval and integration/Garage outcomes are recorded. Exact CI jobs at
  `cba2279` pass and are linked in the fixture guide; planning-only CI remains cancelled.

## 4. Separate migration-overlay validation and delivery

- [x] 4.1 Coordinate with #213's owner and apply the reviewed fixture patch in a separate validation
  context without changing its SemStreams/provider pins. Verify: overlay diff and exact commit/pin record.
- [x] 4.2 Rerun the full integration and combined Garage commands from 3.1/3.2 on the overlay.
  Verify: first failed overlay retained; corrected full integration passes 2,615 checks with nine
  non-fixture skips, combined Garage passes 83 without skips, and independent review approves both.
- [x] 4.3 Document the fixture contract and reproducible commands; record remaining unrelated blockers.
  Verify: clean-environment reproduction and `openspec validate restore-ci-minio-fixtures --strict`.
- [x] 4.4 Deliver the independently reviewed #214 fix and evidence to the owning draft PR; retain
  #212/#213 coordination and distinguish mainline proof from migration-overlay proof.
  Verify: linked PR/reviewer/CI evidence; no claim that fixture repair admits the entire migration.
