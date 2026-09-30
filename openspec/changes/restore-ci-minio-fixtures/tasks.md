# Tasks: restore CI MinIO fixtures

Planning and the architect-approved artifact contract are complete. Implementation and repaired-suite
runtime qualification remain unexecuted; the parent recorded the old failure and a successful cold pull.

## 1. Planning and verified artifact contract

- [x] 1.1 Read AGENTS/CLAUDE/OpenSpec context and inspect current MinIO/Garage fixture/workflow wiring.
  Verify: design starting-point references match the selected mainline checkout.
- [x] 1.2 Create proposal, design, tasks, and capability delta before implementation.
  Verify: `openspec validate restore-ci-minio-fixtures --strict`.
- [x] 1.3 Obtain architect sign-off on the verified mirror/digest and lifecycle contract.
  Verify: design records Thanos upstream-mirror provenance, manifest/child hashes, amd64/arm64 metadata,
  all referenced blobs accessible, and parent cold-pull evidence. Runtime qualification remains separate.

## 2. Additive fixture repair and regression coverage

- [ ] 2.1 Capture the old clean-acquisition failure and inventory every selected MinIO, Garage, and
  governance case before changing fixture wiring. Verify: preserved setup failure and test inventory.
- [ ] 2.2 Add failing preparation/selection checks for immutable identity and rejected unavailable or
  unsupported artifacts, plus readiness timeout and cleanup failure paths.
  Verify: named Go or shell regression tests fail for the old path; record actual test names here.
- [ ] 2.3 Adopt the reviewed exact-release mirror/digest and consistent CI/local selection.
  Verify: cold-pull identity is recorded and the new regression tests pass without changing Garage bootstrap.
- [ ] 2.4 Preserve isolated ownership, bounded readiness, checked cleanup, and failure logs.
  Verify: setup failure with a returned handle cleans it; green-suite teardown failure fails the suite;
  red-suite teardown failure preserves failure; governance explicitly terminates its MinIO fixture.
- [ ] 2.5 Keep both S3 backends and every existing assertion; wire all affected jobs without skips.
  Verify: compare before/after selected test inventories; Garage path filters include `internal/miniotest/**`;
  workflow contract tests pass with Garage bootstrap/image unchanged.

## 3. Independent mainline validation and review

- [ ] 3.1 Run the full original integration suite with beta.161 unchanged.
  Verify: `go test -count=1 -tags=integration ./...`; no fixture-driven exclusions or skips.
- [ ] 3.2 Run the combined compatibility lane against real MinIO and Garage.
  Verify: `go test -count=1 -tags=integration,garage -v -timeout 15m ./storage/s3store/`.
- [ ] 3.3 Run the applicable repository gates: `task lint`, `go test ./...`, affected concurrent tests
  with `-race`, and `task ui:image:release:test` for workflow/Taskfile/script changes.
  Verify: command results are recorded, including any unrelated failures rather than waived checks.
- [ ] 3.4 Record exact artifact/provenance identities, architecture and clean-cache evidence, commands,
  source commit, and CI job URLs; obtain independent Go reviewer approval of the mainline fix.
  Verify: reviewer disposition and full integration/Garage job outcomes are linked to those inputs.

## 4. Separate migration-overlay validation and delivery

- [ ] 4.1 Coordinate with #213's owner and apply the reviewed fixture patch in a separate validation
  context without changing its SemStreams/provider pins. Verify: overlay diff and exact commit/pin record.
- [ ] 4.2 Rerun the full integration and combined Garage commands from 3.1/3.2 on the overlay.
  Verify: complete case inventory, artifact identities, architecture, and separately labelled results.
- [ ] 4.3 Document the fixture contract and reproducible commands; record remaining unrelated blockers.
  Verify: clean-environment reproduction and `openspec validate restore-ci-minio-fixtures --strict`.
- [ ] 4.4 Deliver the independently reviewed #214 fix and evidence to the owning draft PR; retain
  #212/#213 coordination and distinguish mainline proof from migration-overlay proof.
  Verify: linked PR/reviewer/CI evidence; no claim that fixture repair admits the entire migration.
