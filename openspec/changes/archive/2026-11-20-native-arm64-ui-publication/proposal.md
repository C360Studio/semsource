# Publish the workbench image from native per-platform runners

Approved-by: Coby Leuschke (2026-11-20) sha256:3adefeb4d77b

Track: design (changes a gate file, `.github/workflows/ci.yml`, and the release-evidence contract).

## Why

`publish-ui` builds `linux/arm64` under QEMU user-mode emulation. `npm ci` in `ui/Dockerfile`
intermittently dies there with `qemu: uncaught target signal 4 (Illegal instruction)`, and npm then
hangs until the 30-minute job timeout. It hung 2 of the last 7 main runs (`44616589320`,
`44386036561`); evidence is on #219. The beta.10 tag (#237) runs this path once, so a flaky publish
blocks the tag, and a green rerun is not a fix.

**Process note.** The implementation in #321 (`5a688b0`) was written before this change at the
owner's direction in session. It has no prior approval; this proposal is the first approval record,
and the reviewer's REQUEST CHANGES on #321 stands until it is approved.

## What Changes

- New `publish-ui-build` matrix job: `amd64` on `ubuntu-latest`, `arm64` on `ubuntu-24.04-arm`
  (native, free for this public repository). Each leg builds one platform, keeps
  `provenance: mode=max` and `sbom: true`, pushes by digest only (no tag), and uploads its digest.
- `publish-ui` becomes a merge job: `scripts/ui-image-merge.sh` requires exactly one valid amd64 and
  one valid arm64 digest and only in-repository tags, creates the tagged index with
  `docker buildx imagetools create`, and reports the index digest read back from the registry.
- No QEMU on the UI publication path. The Go `build-and-push` job is unchanged.
- `publish-ui`'s outputs and `ui-release-smoke` are unchanged; release evidence still verifies the
  exact tag-plus-digest, both platforms, labels, and the running pin.
- `alert-red-main` also needs `publish-ui-build`.

## Capabilities

### Modified Capabilities

- `ui-profile`: "Workbench and headless release evidence" gains native per-platform builds, the
  both-or-nothing merge, and rerun safety.

## Impact

- `.github/workflows/ci.yml`, `scripts/ui-image-merge.sh` (new), `scripts/test-ui-image-merge.sh`
  (new), `scripts/test-ui-publish-workflow.sh`, `Taskfile.yml` (`ui:image:release:test`).
- No runtime, image content, tag scheme, or consumer change. Acceptance needs one real main
  publication: PR CI skips publish jobs by design.
