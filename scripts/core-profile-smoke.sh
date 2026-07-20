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

# Never attach smoke containers, networks, or volumes to an operator's default
# Compose project. A caller may choose a stable isolated name for diagnostics;
# otherwise the shell PID makes concurrent smoke invocations distinct.
export COMPOSE_PROJECT_NAME="${SEMSOURCE_SMOKE_PROJECT_NAME:-semsource-core-smoke-$$}"

# The default profile must neither resolve nor authenticate for the UI image.
# A deliberately impossible immutable ref makes accidental profile leakage fail
# before any paid or long-running stack work begins.
export SEMSOURCE_UI_IMAGE="${SEMSOURCE_UI_IMAGE:-registry.invalid/c360/semsource-ui:unavailable@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff}"
core_services=$(docker compose config --services | sort)
expected_core_services=$(printf '%s\n' nats semembed semsource | sort)
if [ "$core_services" != "$expected_core_services" ]; then
	echo "Default Compose resolved unexpected services:" >&2
	printf '%s\n' "$core_services" >&2
	exit 1
fi
if docker compose config --images | grep -Eq 'semsource-ui|registry\.invalid|caddy'; then
	echo "Default Compose unexpectedly resolved a UI or proxy image" >&2
	docker compose config --images >&2
	exit 1
fi
echo "Default Compose resolves exactly nats, semembed, and semsource; UI image and Caddy are absent"

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
export SEMSOURCE_HTTP_PORT="${SEMSOURCE_HTTP_PORT:-28080}"
export NATS_HOST_PORT="${NATS_HOST_PORT:-24222}"
export NATS_MONITOR_HOST_PORT="${NATS_MONITOR_HOST_PORT:-28222}"

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
echo "Using isolated Compose project: $COMPOSE_PROJECT_NAME"
echo "Using host ports: SEMSOURCE_HTTP_PORT=$SEMSOURCE_HTTP_PORT NATS_HOST_PORT=$NATS_HOST_PORT NATS_MONITOR_HOST_PORT=$NATS_MONITOR_HOST_PORT"
docker compose up -d --build --wait

running_services=$(docker compose ps --services --status running | sort)
if [ "$running_services" != "$expected_core_services" ]; then
	echo "Core profile running-service set is not exact:" >&2
	printf '%s\n' "$running_services" >&2
	exit 1
fi
echo "Core profile runs no UI, Node service, or Caddy proxy"

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

# Removal-integrity round-trip (source-removal-integrity): a real tools/call
# per lifecycle tool, asserting answer content — not just name listing.
mcp_call() {
	# The tool result's inner JSON rides escaped inside the SSE frame's "text"
	# field; strip the escapes so content assertions match the actual payload.
	curl -sS --max-time 10 \
		-X POST \
		-H 'Content-Type: application/json' \
		-H 'Accept: application/json, text/event-stream' \
		-H 'MCP-Protocol-Version: 2025-06-18' \
		-H "Mcp-Session-Id: $mcp_session_id" \
		-d "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
		"$mcp_url" | sed -n 's/^data: //p' | tr -d '\\'
}

# /workspace/docs (not /workspace): the config-declared docs source already
# owns /workspace, and re-adding it answers created:false — the round-trip
# needs a source this call genuinely creates.
add_reply=$(mcp_call add_source '{"type":"docs","path":"/workspace/docs","actor":"core-smoke"}')
handle=$(printf '%s' "$add_reply" | sed -n 's/.*"instance_name":"\([^"]*\)".*/\1/p' | head -1)
if [ -z "$handle" ]; then
	echo "add_source returned no instance handle: $add_reply" >&2
	exit 1
fi
echo "add_source registered $handle"

remove_reply=$(mcp_call remove_source "{\"instance_name\":\"$handle\",\"actor\":\"core-smoke\"}")
if ! printf '%s' "$remove_reply" | grep -q '"removed":true'; then
	echo "remove_source did not confirm removal: $remove_reply" >&2
	exit 1
fi

# Removal must be observable: the handle leaves source_status within 15s.
removal_seen=""
for _ in $(seq 1 15); do
	status_reply=$(mcp_call source_status '{}')
	if ! printf '%s' "$status_reply" | grep -q "$handle"; then
		removal_seen="yes"
		break
	fi
	sleep 1
done
if [ -z "$removal_seen" ]; then
	echo "removed source $handle still present in source_status: $status_reply" >&2
	exit 1
fi
echo "remove_source removal observable in source_status"

# Unknown handles are NOT_FOUND, never removed:true (honest removal).
unknown_reply=$(mcp_call remove_source '{"instance_name":"smoke-no-such-source"}')
if printf '%s' "$unknown_reply" | grep -q '"removed":true'; then
	echo "remove_source claimed success for an unknown handle: $unknown_reply" >&2
	exit 1
fi
if ! printf '%s' "$unknown_reply" | grep -q 'NOT_FOUND'; then
	echo "remove_source unknown-handle reply lacks NOT_FOUND: $unknown_reply" >&2
	exit 1
fi
echo "remove_source rejects unknown handles with NOT_FOUND"

# --- MCP answer-content coverage (ci-proof-chain D2) ---
# tools/list name-matching alone is not MCP coverage: one real, content-
# asserting tools/call per remaining tool, against the fixture workspace
# (src/main.go's `main`, docs/README.md's body).

# wait_for_status_readiness polls source-manifest/status until it contains the
# given substring (e.g. an "<facet>":{"available":true,"ready":true nesting),
# or fails after timeout_seconds. source-manifest's own phase:ready means every
# source finished its initial seed — it does NOT mean the structural graph-index
# or the semantic embedding index have caught up to that write (mcp-gateway's
# own tool descriptions document this: gate structural queries on phase ready
# AND index.ready; code_search reliability tracks embedding.ready).
wait_for_status_readiness() {
	label=$1
	pattern=$2
	deadline=$(( $(date +%s) + timeout_seconds ))
	while [ "$(date +%s)" -le "$deadline" ]; do
		body=$(curl -fsS --max-time 5 "$status_url" 2>/dev/null) || { sleep "$poll_seconds"; continue; }
		if printf '%s' "$body" | grep -q "$pattern"; then
			return 0
		fi
		sleep "$poll_seconds"
	done
	echo "Timed out waiting for $label" >&2
	echo "Last status: ${body:-no response}" >&2
	return 1
}

echo "Waiting for index.ready before checking structural queries (code_context/code_impact)"
wait_for_status_readiness 'index.ready' '"index":{"available":true,"ready":true' || exit 1

echo "Checking code_context content for the known fixture symbol"
context_body=$(mcp_call code_context '{"query":"main"}')
if ! printf '%s' "$context_body" | grep -q '"name":"main"'; then
	echo "code_context did not resolve the known fixture symbol: $context_body" >&2
	exit 1
fi
if ! printf '%s' "$context_body" | grep -q 'semsource core smoke'; then
	echo "code_context did not return the verbatim body fragment: $context_body" >&2
	exit 1
fi
echo "code_context returns the known symbol with its verbatim body"

echo "Checking code_context honest-miss for a nonexistent symbol"
miss_body=$(mcp_call code_context '{"query":"totallyNonexistentSymbolXYZ123"}')
if printf '%s' "$miss_body" | grep -q '"name":"totallyNonexistentSymbolXYZ123"'; then
	echo "code_context fabricated a node for a nonexistent symbol: $miss_body" >&2
	exit 1
fi
if ! printf '%s' "$miss_body" | grep -q '"misses"'; then
	echo "code_context did not report an honest miss for a nonexistent symbol: $miss_body" >&2
	exit 1
fi
echo "code_context reports an honest miss for a nonexistent symbol (no fabricated node)"

echo "Checking code_impact relations/impact shape"
impact_body=$(mcp_call code_impact '{"query":"main"}')
if ! printf '%s' "$impact_body" | grep -q '"impact"'; then
	echo "code_impact response is missing the impact facet: $impact_body" >&2
	exit 1
fi
echo "code_impact returns the impact facet"

echo "Checking doc_context fixture README content"
doc_body=$(mcp_call doc_context '{"query":"core smoke"}')
if ! printf '%s' "$doc_body" | grep -q 'disposable workspace'; then
	echo "doc_context did not return the fixture README content: $doc_body" >&2
	exit 1
fi
echo "doc_context returns the fixture README content"

echo "Waiting for embedding.ready before checking code_search"
wait_for_status_readiness 'embedding.ready' '"embedding":{"available":true,"ready":true' || exit 1

search_body=$(mcp_call code_search '{"query":"core smoke"}')
if ! printf '%s' "$search_body" | grep -q 'semsource core smoke'; then
	echo "code_search (embedding-gated) did not return the fixture hit: $search_body" >&2
	exit 1
fi
echo "code_search (embedding-gated) returns the fixture hit"

echo "Checking code_changes honest no-versions note"
changes_body=$(mcp_call code_changes '{"project":"core-smoke-unregistered-project","from":"1.0.0","to":"2.0.0"}')
if ! printf '%s' "$changes_body" | grep -q 'no indexed entities for project'; then
	echo "code_changes did not return the honest no-versions note: $changes_body" >&2
	exit 1
fi
echo "code_changes returns the honest no-versions note"

# --- compose-packaging-hardening assertions ---

# D5: the compose-built image must identify its build, not report "dev".
built_version=$(docker compose exec -T semsource semsource version) ||
	fail "semsource version not runnable in the container"
if printf '%s' "$built_version" | grep -Eq '(^|[^a-zA-Z0-9-])dev([^a-zA-Z0-9-]|$)'; then
	echo "compose-built image reports an unidentifiable version: $built_version" >&2
	exit 1
fi
echo "Built image identifies itself: $built_version"

# D2: the rendered healthcheck must target the HTTP serving surface, and the
# probe must discriminate serving from not-serving (same binary present in both
# cases — the audit's 'semsource version' check could not tell them apart).
rendered=$(docker compose config)
if ! printf '%s' "$rendered" | grep -q 'source-manifest/status'; then
	echo "semsource healthcheck does not target the HTTP status endpoint" >&2
	exit 1
fi
docker compose exec -T semsource wget --quiet --tries=1 --spider \
	"http://localhost:8080/source-manifest/status" ||
	fail "healthcheck probe fails against the live serving surface"
if docker compose exec -T semsource wget --quiet --tries=1 --spider \
	"http://localhost:1/source-manifest/status" 2>/dev/null; then
	echo "healthcheck probe cannot discriminate a non-serving endpoint" >&2
	exit 1
fi
echo "Healthcheck targets the serving surface and discriminates serving from not"

# D6: no mutable :latest image without an immutable digest in the core profile.
if docker compose config --images | grep ':latest$'; then
	echo "core profile resolves a mutable :latest image without a digest pin" >&2
	exit 1
fi
echo "All core-profile images are digest-pinned or locally built"

# D3: graph state must survive NATS container recreation (named volume).
echo "Recreating the nats container to prove state durability"
docker compose rm -sf nats >/dev/null
docker compose up -d --wait nats
durable=""
durability_deadline=$(( $(date +%s) + 120 ))
while [ "$(date +%s)" -le "$durability_deadline" ]; do
	post_body=$(curl -fsS --max-time 5 "$status_url" 2>/dev/null) || { sleep 2; continue; }
	post_entities=$(printf '%s' "$post_body" | sed -n 's/.*"total_entities":\([0-9][0-9]*\).*/\1/p')
	if printf '%s' "$post_body" | grep -q '"phase":"ready"' &&
		[ -n "$post_entities" ] && [ "$post_entities" -gt 0 ]; then
		context_reply=$(mcp_call code_context '{"query":"main"}')
		if printf '%s' "$context_reply" | grep -q '"name":"main"'; then
			durable="yes"
			break
		fi
	fi
	sleep 2
done
if [ -z "$durable" ]; then
	echo "entities not queryable after nats recreation (graph state lost?)" >&2
	echo "last status: ${post_body:-none}" >&2
	echo "last code_context: ${context_reply:-none}" >&2
	exit 1
fi
echo "Graph state survived nats recreation (code_context answers post-recreate)"

# D1: the documented tier0 path must boot exactly as written (BM25, no
# semembed dependency, no crash loop). Runs LAST — it replaces the config.
echo "Rebooting semsource with SEMSOURCE_CONFIG=tiers/tier0-statistical.json"
docker compose rm -sf semsource >/dev/null
SEMSOURCE_CONFIG=tiers/tier0-statistical.json docker compose up -d --wait semsource ||
	fail "tier0 config did not reach healthy (documented path crash-loops?)"
tier0_ready=""
tier0_deadline=$(( $(date +%s) + timeout_seconds ))
while [ "$(date +%s)" -le "$tier0_deadline" ]; do
	tier0_body=$(curl -fsS --max-time 5 "$status_url" 2>/dev/null) || { sleep "$poll_seconds"; continue; }
	if printf '%s' "$tier0_body" | grep -q '"phase":"ready"'; then
		tier0_ready="yes"
		break
	fi
	if printf '%s' "$tier0_body" | grep -q '"phase":"degraded"'; then
		echo "tier0 boot degraded: $tier0_body" >&2
		exit 1
	fi
	sleep "$poll_seconds"
done
if [ -z "$tier0_ready" ]; then
	echo "tier0 config never reached ready: ${tier0_body:-no response}" >&2
	exit 1
fi
echo "tier0 documented path boots ready (BM25 mode)"
exit 0
