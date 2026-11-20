# Native per-platform workbench publication tasks

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md`. Hold: OPEN-Q proposal awaits owner approval

## 2. Implementation (on #321, written before approval; see the proposal's process note)

- [ ] 2.1 `publish-ui-build` matrix (native amd64 and arm64, push by digest, provenance and SBOM kept,
  rerun-safe digest artifacts) and `publish-ui` merge job; `alert-red-main` needs the build job.
- [ ] 2.2 `scripts/ui-image-merge.sh` and `scripts/test-ui-image-merge.sh`, wired into
  `task ui:image:release:test`.
- [ ] 2.3 `scripts/test-ui-publish-workflow.sh` asserts the new shape and no QEMU on the UI path.
- [ ] 2.4 `task ui:image:release:test`, `actionlint`, and `task spec:check` pass.

## 3. Close

- [ ] 3.1 Re-review at the approved head records an approving `reviewed-by:`.
- [ ] 3.2 Archive this change before #321 is marked ready.
- [ ] 3.3 After merge: the first main run's `publish-ui-build`, `publish-ui`, and `ui-release-smoke`
  are green; record the run on #219 before beta.10 is armed.
