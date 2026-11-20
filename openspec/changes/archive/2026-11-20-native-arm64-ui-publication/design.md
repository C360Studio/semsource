# Native per-platform workbench publication — design

## Context

`publish-ui` (`.github/workflows/ci.yml`) ran one `docker/build-push-action` for
`linux/amd64,linux/arm64` on `ubuntu-latest` with `docker/setup-qemu-action`. Both `npm ci` steps in
`ui/Dockerfile` (builder and runtime stage) therefore execute under QEMU for arm64. The #219 logs show
SIGILL about 17 s into the builder `npm ci`, then no output until cancellation at 30 minutes; the
amd64 leg completed with its SBOM in the same runs. Three green runs on the same setup showed no SIGILL,
so the failure is intermittent emulation, not the dependency set.

`ui-release-smoke` consumes `publish-ui` outputs (`image`, `digest`, `smoke_tag`, `version`,
`revision`) and runs `scripts/ui-release-image-verify.sh`, which requires the tag to resolve to the
digest, an OCI index with `linux/amd64` and `linux/arm64`, and matching OCI labels on each child.

## Goals / Non-Goals

**Goals:** no emulation on the UI publication path; the same tags, labels, provenance, SBOM, and job
outputs; a tag that never moves to a partial index; a re-run of one failed leg that still publishes.

**Non-Goals:** the Go image (`build-and-push`), which never hit this; image content or the tag scheme;
build caching; making PR CI publish anything.

## Decisions

1. **Native runners, not a cross-compiling builder.** `ubuntu-24.04-arm` runs the existing Dockerfile
   unchanged. The alternative, `FROM --platform=$BUILDPLATFORM` for the builder, still runs the
   runtime stage's `npm ci --omit=dev` under QEMU, and moving that too needs an audit of platform-
   specific optional dependencies. Native runners remove the cause with no Dockerfile change.
2. **Push by digest, merge by tag.** Each leg uses
   `outputs: type=image,name=<repo>,push-by-digest=true,name-canonical=true,push=true`, so no leg
   moves a tag. Only the merge job tags, after both legs succeed (`fail-fast: true`).
3. **Merge in a tested script.** `scripts/ui-image-merge.sh` refuses unless the digest directory holds
   exactly `amd64` and `arm64`, each a lowercase `sha256` digest, and every tag is in
   `ghcr.io/c360studio/semsource-ui`. It then runs `imagetools create` and reads the index digest from
   the registry rather than computing it. `scripts/test-ui-image-merge.sh` runs it against a stub
   `docker` and asserts that each refusal publishes nothing.
4. **Keep the job name and outputs.** `publish-ui` remains the job `ui-release-smoke` needs, with the
   same five outputs, so the verifier and smoke are unchanged.
5. **Rerun-safe artifacts.** Digest artifacts are named per arch with no attempt suffix and upload
   with `overwrite: true`. Re-running only the failed leg replaces its own digest; the merge still
   finds the other leg's.
6. **Contract test.** `scripts/test-ui-publish-workflow.sh` asserts the native arm runner,
   push-by-digest, one platform per leg, provenance and SBOM kept, the merge script in `publish-ui`,
   and no `setup-qemu-action` in either UI job.

## Risks / Trade-offs

- **Attestations through the merge.** Each leg pushes an index carrying its image and attestation
  manifest; `imagetools create` from those indexes is the documented Docker multi-runner pattern and
  is expected to keep them. This is not proven locally. The verifier tolerates extra
  `unknown/unknown` entries, as today.
- **First real run is post-merge.** Publish jobs run only on pushes to `main` and tags. Acceptance is
  the first main run after merge: `publish-ui-build` (both legs), `publish-ui`, and
  `ui-release-smoke` green. That must happen before beta.10 is armed (#237).
- **Rollback.** Reverting the PR restores the QEMU job with no registry cleanup: digest-only pushes
  are untagged.
- **arm runner availability.** If `ubuntu-24.04-arm` queues or is withdrawn, the leg times out at
  20 minutes and `alert-red-main` fires; the remedy is a runner change, not reintroducing QEMU.
