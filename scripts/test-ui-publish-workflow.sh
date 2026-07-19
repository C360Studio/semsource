#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow="$project_dir/.github/workflows/ci.yml"
profile_smoke="$project_dir/scripts/ui-profile-smoke.sh"

fail() {
	echo "UI publish workflow contract test failed: $*" >&2
	exit 1
}

require_text() {
	grep -F -- "$1" "$workflow" >/dev/null || fail "missing contract text: $1"
}

ruby -e 'require "yaml"; YAML.parse_file(ARGV.fetch(0))' "$workflow" ||
	fail "workflow is not valid YAML"

require_text "ui-quality:"
require_text "publish-ui:"
require_text "ui-release-smoke:"
require_text "needs: [test, lint, ui-quality]"
require_text "needs: publish-ui"
require_text "packages: write"
require_text "packages: read"
require_text "github.event_name == 'push'"
require_text "ghcr.io/c360studio/semsource-ui"
require_text "./scripts/ui-image-metadata.sh"
require_text "platforms: linux/amd64,linux/arm64"
require_text "go-task/setup-task@v2"
require_text "npx playwright install --with-deps chromium"
require_text "npm run test:a11y"
require_text "npm run test:e2e"
require_text 'SEMSOURCE_UI_VERSION: sha-${{ github.sha }}'
require_text 'SEMSOURCE_UI_VERSION=${{ steps.image.outputs.version }}'
require_text 'SEMSOURCE_UI_COMMIT=${{ github.sha }}'
require_text 'org.opencontainers.image.version=${{ steps.image.outputs.version }}'
require_text 'org.opencontainers.image.revision=${{ github.sha }}'
require_text 'image: ${{ steps.image.outputs.repository }}'
require_text 'digest: ${{ steps.publish.outputs.digest }}'
require_text 'smoke_tag: ${{ steps.image.outputs.primary_tag }}'
require_text 'version: ${{ steps.image.outputs.version }}'
require_text 'revision: ${{ steps.image.outputs.revision }}'
require_text "./scripts/ui-release-image-verify.sh"
require_text '--output "$GITHUB_OUTPUT"'
require_text 'SEMSOURCE_UI_IMAGE: ${{ steps.verify.outputs.image_ref }}'
require_text "UI_PROFILE_PIN_EVIDENCE_FILE: release-evidence/semsource-ui-image.md"
require_text "run: task ui:smoke"
require_text '${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}'
require_text '${{ github.run_attempt }}'
require_text "actions/upload-artifact@v7"
require_text "Upload UI profile failure diagnostics"
require_text "if: failure()"
require_text 'semsource-ui-smoke-failure-${{ github.sha }}-attempt-${{ github.run_attempt }}'
require_text "path: test-results/ui-profile/**"
require_text "if-no-files-found: warn"

task_action_count=$(grep -Fc "uses: go-task/setup-task@v2" "$workflow")
[ "$task_action_count" = "2" ] ||
	fail "workflow must use exactly two maintained go-task/setup-task@v2 actions"
if grep -F "arduino/setup-task" "$workflow" >/dev/null; then
	fail "workflow must not use the obsolete arduino/setup-task action"
fi

core_publish=$(awk '
  /^  build-and-push:/ { in_job = 1 }
  /^  publish-ui:/ { in_job = 0 }
  in_job { print }
' "$workflow")
if printf '%s\n' "$core_publish" | grep -F "setup-task" >/dev/null; then
	fail "core build-and-push must not install the unused Task runner"
fi

ui_publish=$(awk '
  /^  publish-ui:/ { in_job = 1 }
  /^  ui-release-smoke:/ { in_job = 0 }
  in_job { print }
' "$workflow")
printf '%s\n' "$ui_publish" | grep -F "docker/build-push-action@v7" >/dev/null ||
	fail "publish-ui does not publish the image"
if printf '%s\n' "$ui_publish" | grep -F "task ui:smoke" >/dev/null; then
	fail "publish-ui must stop after publishing"
fi
if printf '%s\n' "$ui_publish" | grep -F "ui-release-image-verify" >/dev/null; then
	fail "publish-ui must leave verification to ui-release-smoke"
fi

release_smoke=$(awk '
  /^  ui-release-smoke:/ { in_job = 1 }
  in_job { print }
' "$workflow")
printf '%s\n' "$release_smoke" | grep -F "ui-release-image-verify" >/dev/null ||
	fail "ui-release-smoke does not verify the registry image"
printf '%s\n' "$release_smoke" | grep -F "task ui:smoke" >/dev/null ||
	fail "ui-release-smoke does not run the released profile smoke"

grep -F "config --images" "$profile_smoke" >/dev/null ||
	fail "released smoke does not inspect the rendered Compose image pin"
grep -F "running_ui_image=" "$profile_smoke" >/dev/null ||
	fail "released smoke does not inspect the running UI container image pin"
grep -F "ui-profile-pin-verify.sh" "$profile_smoke" >/dev/null ||
	fail "released smoke does not independently verify observed profile pins"
grep -F "*:latest@sha256:*" "$profile_smoke" >/dev/null ||
	fail "released smoke does not reject digest-qualified latest"

# Package write permission must be job-scoped; PR-triggered jobs inherit only
# the top-level read permission and can neither log in nor publish.
top_permissions=$(awk '
  /^permissions:/ { in_permissions = 1; next }
  in_permissions && /^[^ ]/ { exit }
  in_permissions { print }
' "$workflow")
printf '%s\n' "$top_permissions" | grep -F "contents: read" >/dev/null ||
	fail "top-level contents permission is not read-only"
if printf '%s\n' "$top_permissions" | grep -F "packages: write" >/dev/null; then
	fail "package write permission must not be granted at workflow scope"
fi

echo "UI publish workflow contract tests passed"
