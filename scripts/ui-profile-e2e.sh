#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

command -v docker >/dev/null 2>&1 || {
	echo "ui:e2e preflight failed: docker is required" >&2
	exit 1
}
docker compose version >/dev/null 2>&1 || {
	echo "ui:e2e preflight failed: docker compose is required" >&2
	exit 1
}

artifact_project=${COMPOSE_PROJECT_NAME:-semsource}
export UI_PROFILE_ARTIFACT_DIR="${UI_PROFILE_ARTIFACT_DIR:-$project_dir/test-results/ui-profile/$artifact_project}"
case "$UI_PROFILE_ARTIFACT_DIR" in
/*) ;;
*) UI_PROFILE_ARTIFACT_DIR="$project_dir/$UI_PROFILE_ARTIFACT_DIR" ;;
esac
export UI_PROFILE_ARTIFACT_DIR
mkdir -p "$UI_PROFILE_ARTIFACT_DIR"
echo "Playwright artifacts: $UI_PROFILE_ARTIFACT_DIR"

# The runner image contains the exact Playwright version from ui/package-lock.json
# and its browser. --no-deps preserves the already-running released or dev stack;
# the bind-mounted output directory survives the one-shot --rm runner.
docker compose \
	-f docker-compose.yml \
	-f test/ui/docker-compose.test.yml \
	--profile ui-test \
	run --rm --build --no-deps ui-e2e
