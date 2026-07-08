#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

fail() {
	echo "core:smoke preflight failed: $*" >&2
	exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"
docker compose version >/dev/null 2>&1 || fail "docker compose is required"

created_workspace=0
smoke_workspace=${SEMSOURCE_SMOKE_WORKSPACE:-}
if [ -z "$smoke_workspace" ]; then
	smoke_workspace=$(mktemp -d "${TMPDIR:-/tmp}/semsource-core-smoke.XXXXXX")
	created_workspace=1
	mkdir -p "$smoke_workspace/src" "$smoke_workspace/docs" "$smoke_workspace/config"
	touch "$smoke_workspace/.semsource-core-smoke"
	cat >"$smoke_workspace/src/main.go" <<'GO'
package main

import "fmt"

func main() {
	fmt.Println("semsource core smoke")
}
GO
	cat >"$smoke_workspace/docs/README.md" <<'MD'
# SemSource Core Smoke

This disposable workspace gives the core profile a tiny document to index.
MD
	cat >"$smoke_workspace/config/app.yaml" <<'YAML'
service: semsource-core-smoke
owner: semsource
YAML
fi

case "$smoke_workspace" in
/*) ;;
*) smoke_workspace="$project_dir/$smoke_workspace" ;;
esac

[ -d "$smoke_workspace" ] || fail "SEMSOURCE_SMOKE_WORKSPACE '$smoke_workspace' does not exist"

export SEMSOURCE_TARGET="$smoke_workspace"
export SEMSOURCE_HTTP_PORT="${SEMSOURCE_HTTP_PORT:-8080}"
export NATS_HOST_PORT="${NATS_HOST_PORT:-14222}"
export NATS_MONITOR_HOST_PORT="${NATS_MONITOR_HOST_PORT:-18222}"

status_url="http://127.0.0.1:${SEMSOURCE_HTTP_PORT}/source-manifest/status"
timeout_seconds=${CORE_PROFILE_TIMEOUT_SECONDS:-300}
poll_seconds=${CORE_PROFILE_POLL_SECONDS:-2}
last_body="no response yet"

print_diagnostics() {
	echo "Recent Docker Compose state:" >&2
	docker compose ps >&2 || true
	docker compose logs --tail=120 semsource >&2 || true
	docker compose logs --tail=80 semembed >&2 || true
}

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		print_diagnostics
	fi

	if [ "${KEEP_CORE_PROFILE:-0}" = "1" ]; then
		echo "KEEP_CORE_PROFILE=1 set; leaving SemSource core profile running"
		if [ "$created_workspace" = "1" ]; then
			echo "Smoke workspace retained at $smoke_workspace"
		fi
		exit "$status"
	fi

	echo "Tearing down SemSource core profile"
	if ! docker compose down -v; then
		echo "Warning: failed to tear down SemSource core profile" >&2
	fi

	if [ "$created_workspace" = "1" ] && [ -f "$smoke_workspace/.semsource-core-smoke" ]; then
		rm -rf "$smoke_workspace"
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Starting SemSource core profile with SEMSOURCE_TARGET=$SEMSOURCE_TARGET"
echo "Using host ports: SEMSOURCE_HTTP_PORT=$SEMSOURCE_HTTP_PORT NATS_HOST_PORT=$NATS_HOST_PORT NATS_MONITOR_HOST_PORT=$NATS_MONITOR_HOST_PORT"
docker compose up -d --build --wait

deadline=$(( $(date +%s) + timeout_seconds ))
echo "Polling $status_url for ready source-manifest status"
while [ "$(date +%s)" -le "$deadline" ]; do
	if body=$(curl -fsS --max-time 5 "$status_url" 2>/dev/null); then
		last_body=$body
		total_entities=$(printf '%s' "$body" | sed -n 's/.*"total_entities":\([0-9][0-9]*\).*/\1/p')
		if printf '%s' "$body" | grep -q '"phase":"degraded"'; then
			echo "source-manifest/status degraded: $body" >&2
			exit 1
		fi
		if printf '%s' "$body" | grep -Eq '"namespace":"[^"]+"' &&
			printf '%s' "$body" | grep -q '"phase":"ready"' &&
			[ -n "$total_entities" ] &&
			[ "$total_entities" -gt 0 ]; then
			echo "Core profile ready with total_entities=$total_entities"
			exit 0
		fi
	fi
	sleep "$poll_seconds"
done

echo "Timed out waiting for source-manifest/status to become ready" >&2
echo "Last response: $last_body" >&2
exit 1
