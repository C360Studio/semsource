# Native per-platform workbench publication tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-20, sha256:3adefeb4d77b; line entered by claude at the owner's instruction).

## 2. Implementation (on #321, written before approval; see the proposal's process note)

- [x] 2.1 `publish-ui-build` matrix (native amd64 and arm64, push by digest, provenance and SBOM kept,
  rerun-safe digest artifacts) and `publish-ui` merge job; `alert-red-main` needs the build job.
- [x] 2.2 `scripts/ui-image-merge.sh` and `scripts/test-ui-image-merge.sh`, wired into
  `task ui:image:release:test`.
- [x] 2.3 `scripts/test-ui-publish-workflow.sh` asserts the new shape and no QEMU on the UI path.
- [x] 2.4 `task ui:image:release:test`, `actionlint`, and `task spec:check` pass.

## 3. Close

- [x] 3.1 Archive this change before #321 is marked ready; the re-review runs at the archived head.

Post-merge acceptance (the first main run's `publish-ui-build`, `publish-ui`, and `ui-release-smoke`
green, recorded on #219 before beta.10 is armed) is tracked on #219, not here.
