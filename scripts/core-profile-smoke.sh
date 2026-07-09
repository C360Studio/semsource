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
sources_url="http://127.0.0.1:${SEMSOURCE_HTTP_PORT}/source-manifest/sources"
mcp_url="http://127.0.0.1:${SEMSOURCE_HTTP_PORT}/mcp-gateway/mcp"
graphql_url="http://127.0.0.1:${SEMSOURCE_HTTP_PORT}/graph-gateway/graphql"
timeout_seconds=${CORE_PROFILE_TIMEOUT_SECONDS:-300}
poll_seconds=${CORE_PROFILE_POLL_SECONDS:-2}
last_body="no response yet"
graphql_get_body=$(mktemp "${TMPDIR:-/tmp}/semsource-graphql-get.XXXXXX")
graphql_post_body=$(mktemp "${TMPDIR:-/tmp}/semsource-graphql-post.XXXXXX")
mcp_probe_body=$(mktemp "${TMPDIR:-/tmp}/semsource-mcp-probe.XXXXXX")
mcp_init_headers=$(mktemp "${TMPDIR:-/tmp}/semsource-mcp-init-headers.XXXXXX")
mcp_init_body=$(mktemp "${TMPDIR:-/tmp}/semsource-mcp-init-body.XXXXXX")
mcp_initialized_body=$(mktemp "${TMPDIR:-/tmp}/semsource-mcp-initialized-body.XXXXXX")
mcp_tools_body=$(mktemp "${TMPDIR:-/tmp}/semsource-mcp-tools-body.XXXXXX")

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

	rm -f "$graphql_get_body" "$graphql_post_body" "$mcp_probe_body" "$mcp_init_headers" "$mcp_init_body" "$mcp_initialized_body" "$mcp_tools_body"

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
ready=0
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
			ready=1
			break
		fi
	fi
	sleep "$poll_seconds"
done

if [ "$ready" -ne 1 ]; then
	echo "Timed out waiting for source-manifest/status to become ready" >&2
	echo "Last response: $last_body" >&2
	exit 1
fi

echo "Checking $sources_url"
sources_body=$(curl -fsS --max-time 5 "$sources_url") || fail "source-manifest/sources is not reachable"
if ! printf '%s' "$sources_body" | grep -Eq '"namespace":"[^"]+"' ||
	! printf '%s' "$sources_body" | grep -q '"sources":\['; then
	echo "Unexpected source-manifest/sources response: $sources_body" >&2
	exit 1
fi
for source_type in ast docs config; do
	if ! printf '%s' "$sources_body" | grep -q "\"type\":\"$source_type\""; then
		echo "source-manifest/sources missing $source_type source: $sources_body" >&2
		exit 1
	fi
done
echo "Source manifest reachable with ast/docs/config sources"

echo "Checking $graphql_url"
graphql_get_code=$(curl -sS --max-time 10 \
	-o "$graphql_get_body" \
	-w '%{http_code}' \
	"$graphql_url") || fail "graph-gateway/graphql GET route is not reachable"
if [ "$graphql_get_code" = "404" ] || [ "$graphql_get_code" -ge 500 ]; then
	echo "Unexpected graph-gateway/graphql GET status: $graphql_get_code" >&2
	echo "Body: $(cat "$graphql_get_body")" >&2
	exit 1
fi

graphql_post_code=$(curl -sS --max-time 10 \
	-o "$graphql_post_body" \
	-w '%{http_code}' \
	-X POST \
	-H 'Content-Type: application/json' \
	-d '{"query":"query CoreProfileSmoke { __typename }"}' \
	"$graphql_url") || fail "graph-gateway/graphql POST route is not reachable"
if [ "$graphql_post_code" -lt 200 ] || [ "$graphql_post_code" -ge 500 ]; then
	echo "Unexpected graph-gateway/graphql POST status: $graphql_post_code" >&2
	echo "Body: $(cat "$graphql_post_body")" >&2
	exit 1
fi
if ! grep -q '"data"' "$graphql_post_body" && ! grep -q '"errors"' "$graphql_post_body"; then
	echo "Unexpected graph-gateway/graphql POST body: $(cat "$graphql_post_body")" >&2
	exit 1
fi
echo "GraphQL gateway route reachable"

echo "Checking $mcp_url"
mcp_code=$(curl -sS --max-time 5 \
	-o "$mcp_probe_body" \
	-w '%{http_code}' \
	-X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	"$mcp_url") || fail "mcp-gateway/mcp is not reachable"
if [ "$mcp_code" != "400" ]; then
	echo "Unexpected mcp-gateway/mcp probe status: $mcp_code" >&2
	echo "Body: $(cat "$mcp_probe_body")" >&2
	exit 1
fi
if ! grep -q "POST requires a non-empty body" "$mcp_probe_body"; then
	echo "Unexpected mcp-gateway/mcp probe body: $(cat "$mcp_probe_body")" >&2
	exit 1
fi
echo "MCP gateway endpoint reachable"

echo "Checking MCP initialize + tools/list happy path"
mcp_init_code=$(curl -sS --max-time 10 \
	-D "$mcp_init_headers" \
	-o "$mcp_init_body" \
	-w '%{http_code}' \
	-X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"semsource-core-smoke","version":"0.0.1"}}}' \
	"$mcp_url") || fail "MCP initialize request failed"
if [ "$mcp_init_code" != "200" ]; then
	echo "Unexpected MCP initialize status: $mcp_init_code" >&2
	echo "Body: $(cat "$mcp_init_body")" >&2
	exit 1
fi
if ! grep -q '"serverInfo":{"name":"semsource"' "$mcp_init_body"; then
	echo "Unexpected MCP initialize body: $(cat "$mcp_init_body")" >&2
	exit 1
fi
mcp_session_id=$(grep -i '^Mcp-Session-Id:' "$mcp_init_headers" | sed -n 's/^[^:]*:[[:space:]]*//p' | tr -d '\r' | sed -n '1p')
if [ -z "$mcp_session_id" ]; then
	echo "MCP initialize response did not include Mcp-Session-Id" >&2
	echo "Headers: $(cat "$mcp_init_headers")" >&2
	exit 1
fi

mcp_initialized_code=$(curl -sS --max-time 5 \
	-o "$mcp_initialized_body" \
	-w '%{http_code}' \
	-X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-H "Mcp-Session-Id: $mcp_session_id" \
	-d '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
	"$mcp_url") || fail "MCP initialized notification failed"
if [ "$mcp_initialized_code" != "202" ]; then
	echo "Unexpected MCP initialized notification status: $mcp_initialized_code" >&2
	echo "Body: $(cat "$mcp_initialized_body")" >&2
	exit 1
fi

mcp_tools_code=$(curl -sS --max-time 10 \
	-o "$mcp_tools_body" \
	-w '%{http_code}' \
	-X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-H "Mcp-Session-Id: $mcp_session_id" \
	-d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
	"$mcp_url") || fail "MCP tools/list request failed"
if [ "$mcp_tools_code" != "200" ]; then
	echo "Unexpected MCP tools/list status: $mcp_tools_code" >&2
	echo "Body: $(cat "$mcp_tools_body")" >&2
	exit 1
fi
for tool_name in add_source code_changes code_context code_impact code_search doc_context remove_source source_status; do
	if ! grep -q "\"name\":\"$tool_name\"" "$mcp_tools_body"; then
		echo "MCP tools/list missing $tool_name: $(cat "$mcp_tools_body")" >&2
		exit 1
	fi
done
echo "MCP tools/list returned the expected SemSource tools"
exit 0
