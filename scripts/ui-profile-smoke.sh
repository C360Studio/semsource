#!/usr/bin/env sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

fail() {
	echo "ui:smoke preflight failed: $*" >&2
	exit 1
}

mode=${1:-released}
case "$mode" in
released | dev) ;;
*) fail "mode must be 'released' or 'dev'" ;;
esac

command -v docker >/dev/null 2>&1 || fail "docker is required"
docker compose version >/dev/null 2>&1 || fail "docker compose is required"

# Never attach smoke containers, networks, or volumes to an operator's default
# Compose project. ui-profile-e2e.sh inherits this exact name so its one-shot
# Playwright container joins this smoke stack's project-scoped c360 network.
export COMPOSE_PROJECT_NAME="${SEMSOURCE_SMOKE_PROJECT_NAME:-semsource-ui-smoke-$$}"

compose_files="-f docker-compose.yml"
if [ "$mode" = "released" ]; then
	[ -n "${SEMSOURCE_UI_IMAGE:-}" ] ||
		fail "SEMSOURCE_UI_IMAGE must name an immutable released image"
	if ! printf '%s\n' "$SEMSOURCE_UI_IMAGE" | grep -Eq '@sha256:[0-9a-f]{64}$'; then
		fail "SEMSOURCE_UI_IMAGE must end in a 64-character lowercase @sha256 digest"
	fi
	case "$SEMSOURCE_UI_IMAGE" in
	*@sha256:0000000000000000000000000000000000000000000000000000000000000000)
		fail "SEMSOURCE_UI_IMAGE cannot use the unavailable placeholder"
		;;
	esac
else
	compose_files="$compose_files -f docker-compose.ui-dev.yml"
	export SEMSOURCE_UI_VERSION="${SEMSOURCE_UI_VERSION:-dev}"
	export SEMSOURCE_UI_COMMIT="${SEMSOURCE_UI_COMMIT:-$(git rev-parse --short=12 HEAD)}"
fi

created_workspace=0
smoke_workspace=${SEMSOURCE_SMOKE_WORKSPACE:-}
if [ -z "$smoke_workspace" ]; then
	smoke_workspace=$(mktemp -d "${TMPDIR:-/tmp}/semsource-ui-smoke.XXXXXX")
	created_workspace=1
	mkdir -p "$smoke_workspace/src" "$smoke_workspace/docs" "$smoke_workspace/config"
	touch "$smoke_workspace/.semsource-ui-smoke"
	cat >"$smoke_workspace/src/workbench_fixture.go" <<'GO'
package fixture

// UiProfileFixtureAlpha is deterministic source evidence for browser search.
func UiProfileFixtureAlpha() string { return "semsource workbench alpha" }

// UiProfileFixtureBeta supplies a second keyboard-selectable search result.
func UiProfileFixtureBeta() string { return UiProfileFixtureAlpha() + " beta" }
GO
	cat >"$smoke_workspace/docs/README.md" <<'MD'
# SemSource UI Smoke

The optional workbench exposes source readiness and supplied search evidence.
MD
	cat >"$smoke_workspace/config/app.yaml" <<'YAML'
service: semsource-ui-smoke
owner: semsource
YAML
fi

case "$smoke_workspace" in
/*) ;;
*) smoke_workspace="$project_dir/$smoke_workspace" ;;
esac
[ -d "$smoke_workspace" ] || fail "SEMSOURCE_SMOKE_WORKSPACE '$smoke_workspace' does not exist"

export SEMSOURCE_TARGET="$smoke_workspace"
export SEMSOURCE_HTTP_PORT="${SEMSOURCE_HTTP_PORT:-18080}"
export NATS_HOST_PORT="${NATS_HOST_PORT:-14222}"
export NATS_MONITOR_HOST_PORT="${NATS_MONITOR_HOST_PORT:-18222}"
export C360_PORT="${C360_PORT:-13000}"
export UI_PROFILE_ARTIFACT_DIR="${UI_PROFILE_ARTIFACT_DIR:-$project_dir/test-results/ui-profile/$COMPOSE_PROJECT_NAME}"
case "$UI_PROFILE_ARTIFACT_DIR" in
/*) ;;
*) UI_PROFILE_ARTIFACT_DIR="$project_dir/$UI_PROFILE_ARTIFACT_DIR" ;;
esac
export UI_PROFILE_ARTIFACT_DIR
mkdir -p "$UI_PROFILE_ARTIFACT_DIR"

compose() {
	# compose_files contains only repository-owned constant filenames.
	# shellcheck disable=SC2086
	docker compose $compose_files "$@"
}

compose_with_test() {
	# shellcheck disable=SC2086
	docker compose $compose_files -f test/ui/docker-compose.test.yml "$@"
}

print_diagnostics() {
	echo "Recent UI profile state:" >&2
	compose ps >&2 || true
	compose logs --tail=160 semsource ui caddy >&2 || true
	echo "Playwright failure artifacts: $UI_PROFILE_ARTIFACT_DIR" >&2
	if command -v curl >/dev/null 2>&1; then
		for path in / /health /source-manifest/capabilities; do
			probe_file=$(mktemp "${TMPDIR:-/tmp}/semsource-ui-probe.XXXXXX")
			echo "HTTP probe http://127.0.0.1:${C360_PORT}${path} (first 4096 bytes):" >&2
			curl -sS -i --max-time 5 "http://127.0.0.1:${C360_PORT}${path}" >"$probe_file" 2>&1 || true
			head -c 4096 "$probe_file" >&2 || true
			printf '\n' >&2
			rm -f "$probe_file"
		done
	fi
}

cleanup() {
	status=$?
	if [ "$status" -ne 0 ]; then
		print_diagnostics
	fi
	if [ "${KEEP_UI_PROFILE:-0}" = "1" ]; then
		echo "KEEP_UI_PROFILE=1 set; leaving SemSource UI profile running"
		exit "$status"
	fi
	echo "Tearing down SemSource UI profile"
	compose_with_test --profile ui --profile ui-test down -v --remove-orphans ||
		echo "Warning: failed to tear down SemSource UI profile" >&2
	if [ "$created_workspace" = "1" ] && [ -f "$smoke_workspace/.semsource-ui-smoke" ]; then
		rm -rf "$smoke_workspace"
	fi
	exit "$status"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Starting SemSource UI profile ($mode) with deterministic fixture $SEMSOURCE_TARGET"
echo "Using isolated Compose project: $COMPOSE_PROJECT_NAME"
echo "Preserving Playwright artifacts at: $UI_PROFILE_ARTIFACT_DIR"
if [ "$mode" = "released" ]; then
	compose --profile ui up -d --wait
else
	compose --profile ui up -d --build --wait
fi

echo "Running locked containerized UI profile Playwright smoke"
./scripts/ui-profile-e2e.sh
