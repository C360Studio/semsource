# Design: trusted and reproducible CI fixtures

## Status and ownership

This is the fixture contract for #214, claimed in draft PR #217. The architect approved the
exact-upstream Thanos mirror and the fixture lifecycle scope. Mainline implementation and local ARM64
integration, combined Garage, unit-race, lint, and workflow-contract checks pass, with independent Go
review approval. Remote CI and combined Garage pass, with reviewed AMD64 runtime proof.
The first migration overlay exposed pre-existing AST authority-input failures; #218 separately
corrected the test wiring in #213. The corrected overlay passes full integration and combined Garage,
with the original failure evidence retained. Independent Go review approves the final identical-tree
results and unchanged pins. These approvals close only fixture and test-setup evidence gaps.

## Verified starting point before the repair

- `internal/miniotest/miniotest.go` selects `minio/minio:RELEASE.2025-09-07T16-13-09Z`
  and starts it through testcontainers with HTTP liveness readiness.
- `internal/garagetest/garagetest.go` selects `dxflrs/garage:v2.3.0`, provisions Garage, and
  checks executable/server readiness. Garage is a separate S3 compatibility backend.
- `.github/workflows/garage-compat.yml` runs `go test -tags=integration,garage` for `storage/s3store`.
  `storage/s3store/main_test.go` composes both fixture cleanup paths for those tags.
- Repository instructions require the UI image shell-contract gate when workflows, Taskfile, or
  scripts change, even though this repair is unrelated to the product UI.

Observed image acquisition failures are documented in #214 and the migration evidence. Current code
and selected test cases are the coverage baseline; neither a successful cache hit nor a skipped case
establishes restored acquisition. No runtime product or upstream module contract needs to change.

## Approved artifact contract

Use the existing upstream release through the Thanos mirror with these exact immutable coordinates:

- Repository: `quay.io/thanos/minio`
- Release tag: `RELEASE.2025-09-07T16-13-09Z`
- Manifest digest: `sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e`

The architect verified the upstream mirror statement in
[Thanos PR #9029](https://github.com/thanos-io/thanos/pull/9029), merged September 15, and checked
live index/child-manifest body hashes for `linux/amd64` and `linux/arm64`. All 20 referenced blob
requests returned HTTP 200. This selects the exact existing upstream artifact; it does not replace
MinIO with a different S3 implementation. No source build, builder/runtime base selection, or registry
publication is in scope. A future fallback requires a new reviewed decision, never silent substitution.

The parent recorded pre-pull inspection failure followed by a successful cold pull in
`/tmp/semsource-ci214-evidence/mirror-pull.log`, and the old fixture's one-case failure in
`/tmp/semsource-ci214-evidence/before-minio.log`. These prove acquisition and the original setup
failure, not repaired-suite behavior. Preserve exact commands, resolved image identity, platform, and
results in the implementation evidence. Distinguish the multi-platform manifest digest from a local
image ID and architecture-specific child manifest. Runtime validation must record the selected
architecture; metadata availability alone does not qualify both architectures.

Preserve the Garage image and bootstrap. The combined workflow still executes both backends, and its
path filter must include `internal/miniotest/**` so a shared MinIO change selects that validation.

The provenance and current validation scope are documented in
[the fixture guide](../../../../docs/testing/ci-fixtures.md) and its compact JSON evidence record.
That guide distinguishes the archived artifact from a maintained distribution and separates verified
registry identities, passing mainline ARM64/AMD64 suites, and separately recorded corrected-overlay results.

## Fixture lifecycle contract

Each fixture run owns the resources it creates: containers, networks, temporary data, credentials,
and ports. Resource
identity must be available in diagnostics so cleanup can be restricted to that run. Setup uses finite
contexts and observable readiness through the existing health/API contracts, not fixed sleeps.
Image acquisition, startup, bootstrap, and test failure must each preserve useful logs and terminate
owned resources under a bounded cleanup context. Cleanup errors are reported rather than discarded.
A non-nil owned container handle returned alongside a startup error must still be terminated.
`RunTests`/`TestMain` must preserve test failure status and turn an otherwise-green suite red when
teardown fails. Governance tests must explicitly terminate their shared MinIO fixture. Preserve
Garage bootstrap and backend selection while making shared cleanup truthful.
No destructive global Docker cleanup or reliance on a pre-existing developer service is permitted.

Chosen-fixture unavailability is a setup failure, never a successful suite or silent skip. Existing
storage/governance assertions still determine product behavior once setup succeeds. Keep MinIO and
Garage tests distinct even when a combined job executes both.

## Validation and rollout

### Independent mainline fix

Start from main `34bda6406fb06fd723a040988647b204204a1583` with SemStreams beta.161 unchanged.
Capture the old fixture failure, add regression checks for the selected artifact/preparation and
failure/cleanup paths, then restore the full original integration and Garage lanes without exclusions.
Record source/image identities, exact commands, architecture, selected test inventory, failures, and
job URLs. Obtain independent Go review before integration. This phase proves #214 independently.

### Separate migration overlay

After the mainline patch is independently reviewable, apply it in a separate migration validation
context coordinated with #213's owner. Preserve #213's pinned SemStreams/provider inputs and fixture
case inventory. Record the overlay commits and environment separately; rerun the full original
integration and combined Garage lanes. Do not rewrite earlier failed migration evidence or claim
that unrelated migration blockers are resolved by restored fixtures.

## Risks and remaining qualification

- The mirror's immutable content, cold acquisition, and local ARM64 fixture/API behavior pass the
  unchanged selected suites. AMD64 CI and the corrected migration overlay also pass. Registry
  availability remains an external dependency; no alternate image is selected silently.
- Architecture metadata is verified for amd64/arm64. Record each actual runtime environment and
  leave unexecuted architecture claims explicit rather than silently switching platforms.
- Failure cleanup changes can accidentally hide an existing nonzero test result. Regression tests
  must cover setup failure with an owned handle, teardown failure after green tests, and teardown
  failure after red tests.
- A working fixture may expose existing product/upstream failures. Keep them visible and triage
  separately without changing dependency pins, Garage bootstrap, or assertions.
