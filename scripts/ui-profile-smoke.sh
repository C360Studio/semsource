#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

fail() {
	echo "ui:smoke preflight failed: $*" >&2
	exit 1
}

[ -f "ui/Dockerfile" ] || fail "missing SemSource-owned ui/Dockerfile"
if [ ! -x "ui/node_modules/.bin/playwright" ]; then
	fail "Playwright is not installed in ui/; run npm --prefix ui ci"
fi

cleanup() {
	status=$?
	if [ "${KEEP_UI_PROFILE:-0}" = "1" ]; then
		echo "KEEP_UI_PROFILE=1 set; leaving SemSource UI profile running"
		exit "$status"
	fi

	echo "Tearing down SemSource UI profile"
	if ! docker compose --profile ui down -v; then
		echo "Warning: failed to tear down SemSource UI profile" >&2
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Starting SemSource UI profile"
docker compose --profile ui up -d --build --wait

echo "Running SemSource UI profile Playwright smoke"
task ui:e2e
