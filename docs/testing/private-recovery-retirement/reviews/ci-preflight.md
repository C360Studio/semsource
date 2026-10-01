# Independent CI image preflight review

Disposition: **APPROVED**, no remaining finding in the narrow workflow correction.
Reviewer `/root/removal_publisher` read the change and evidence without editing repository files.
Hosted verification of the corrected workflow remains pending; this is not human GitHub approval.

## Reviewed change

The only executable change adds `docker pull "$SETUP03A_NATS_IMAGE"` at the start of the existing
desired source lifecycle qualification step, before its build and test commands. Its explanatory
comment records the frozen harness's expectation that `docker run` output contain only the ID.
The image remains exactly
`nats:2.14.4-alpine@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`.

Reviewed `.github/workflows/ci.yml` SHA256:
`d200c37e6a27aeb8022349489298918161f0e0846ee9beee9ef79c9f6e32b51a`.
`git diff --check` passes. There is no Go, module-pin, test-helper, or `test/setup03a` diff in this fix.
No test selection, assertions, timeouts, cleanup policy, or pass/fail classification is weakened.
The preflight fails the existing shell step if the pinned image cannot be pulled.

## Evidence and diagnosis

Downloaded metadata identifies run `36854140403`, commit
`d90f0432253dab36062608583802edb4246e3129`, and a failed desired-lifecycle step in `e2e-local`.
The preceding local E2E step passed. The failed step's log and downloaded artifact show:

- Original-source case: zero observations, no application logs or generated application config,
  and failure approximately 2.85 seconds after starting the case.
- Its recorded `nats_container` is the complete Docker cold-image pull output followed by the
  valid container ID `4c4c8b1c9a8e02272805dbec5a0a9cb381e595c638263da07ed52e8beaab7737`.
- Its attempted broker log retrieval reports Docker `page not found`. Cleanup also used the
  corrupted identifier, so this attempt must not claim successful original-container cleanup.
- The subsequent runtime-added case on the same runner uses a plain container ID and passes all
  37 observations, including checked application exits, with the image then cached.

The unchanged harness explains the failure directly: `qualification_test.go:294` returns
`CombinedOutput`; `broker` assigns all successful detached-run output to `h.container` at line 312.
The next port lookup receives pull progress plus the ID as one invalid container identifier.
This is a harness setup failure before source-workload execution, not evidence of a SemSource
lifecycle behavior failure. The same combined-output problem affects its subsequent cleanup.

Pulling the exact image separately keeps cold-image progress out of the detached-run output and
addresses this observed failure without editing the frozen harness. It is a focused environmental
precondition, not a general claim that the harness parses arbitrary Docker warnings safely.

## Preservation and qualification limits

The first failure stays failed. Its metadata, failed log, and artifact remain under
`/tmp/semsource-lifecycle-correction/ci-36854140403*` and
`/tmp/semsource-lifecycle-correction/ci-failure-artifact/`.
The original case cannot be counted as qualified from that run. A subsequent hosted run must
exercise both cases and record its own outcome; the cached-image case supports the diagnosis but
does not substitute for that verification. This reviewer did not trigger a rerun or pull images.
