#!/usr/bin/env sh
# Smoke the tier-2 local development overlay (docker-compose.tier2-dev.yml).
#
# What this covers UNCONDITIONALLY (cheap, no image pulls, no inference):
#   1. the overlay resolves exactly nats + semembed + semsource + seminstruct,
#      and does NOT drag in the UI profile or Caddy;
#   2. seminstruct is immutably pinned (tag@sha256:), like every other shipped
#      image;
#   3. tiers/tier2-compose-dev.json is well-formed and wires its capabilities to
#      Compose service hostnames rather than localhost — the single most common
#      way a tier-2 config silently fails inside Compose.
#
# What is OPT-IN (SEMSOURCE_TIER2_SMOKE_FULL=1): actually starting the stack and
# polling source-manifest to ready. That path pulls a ~1GB image and runs real
# LLM inference, so it is deliberately not the default and is NOT run in CI.
# Anything it would prove is therefore NOT claimed as covered by this script.
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_dir"

fail() {
	echo "tier2:smoke:dev failed: $*" >&2
	exit 1
}

command -v docker >/dev/null 2>&1 || fail "docker is required"
docker compose version >/dev/null 2>&1 || fail "docker compose is required"

overlay="docker-compose.tier2-dev.yml"
tier2_config="configs/tiers/tier2-compose-dev.json"
[ -f "$overlay" ] || fail "$overlay not found"
[ -f "$tier2_config" ] || fail "$tier2_config not found"

# Never attach smoke resources to an operator's default Compose project.
export COMPOSE_PROJECT_NAME="${SEMSOURCE_TIER2_SMOKE_PROJECT_NAME:-semsource-tier2-smoke-$$}"

# The overlay must not resolve the UI profile. An impossible immutable ref makes
# accidental profile leakage fail loudly instead of attempting a real pull.
export SEMSOURCE_UI_IMAGE="${SEMSOURCE_UI_IMAGE:-registry.invalid/c360/semsource-ui:unavailable@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff}"

compose="docker compose -f docker-compose.yml -f $overlay"

# 1. Service set.
services=$($compose config --services | sort)
expected=$(printf '%s\n' nats semembed semsource seminstruct | sort)
if [ "$services" != "$expected" ]; then
	echo "Tier-2 overlay resolved unexpected services:" >&2
	printf '%s\n' "$services" >&2
	exit 1
fi
if $compose config --images | grep -Eq 'semsource-ui|registry\.invalid|caddy'; then
	echo "Tier-2 overlay unexpectedly resolved a UI or proxy image" >&2
	$compose config --images >&2
	exit 1
fi
echo "Tier-2 overlay resolves exactly nats, semembed, semsource, seminstruct; UI and Caddy absent"

# 2. Immutable pins, to compose-deployment's rule: a digest OR an exact version.
# A bare tag, `latest`, or a floating major like `2-alpine` all let a rebuild
# change what a dev machine runs, and are refused.
$compose config --images | python3 -c '
import re
import sys

offenders = []
for line in sys.stdin:
    image = line.strip()
    if not image:
        continue
    if "@sha256:" in image:
        continue
    # The locally built semsource image carries no registry pin by construction.
    if "semsource" in image.rsplit(":", 1)[0]:
        continue
    tag = image.rsplit(":", 1)[1] if ":" in image.rsplit("/", 1)[-1] else ""
    # An exact version needs at least major.minor — "2-alpine" floats, "2.12-alpine" does not.
    if not re.match(r"^\d+\.\d+", tag):
        offenders.append((image, tag or "<none>"))

if offenders:
    print("Tier-2 overlay resolved images that are neither digest-pinned nor exact-versioned:", file=sys.stderr)
    for image, tag in offenders:
        print(f"  - {image} (tag {tag})", file=sys.stderr)
    sys.exit(1)
print("Every registry image the tier-2 overlay resolves is digest-pinned or exact-versioned")
' || exit 1

# 3. Tier-2 config wiring. Compose service hostnames, not localhost.
python3 - "$tier2_config" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path) as fh:
    cfg = json.load(fh)

registry = cfg.get("model_registry", {})
endpoints = registry.get("endpoints", {})
capabilities = registry.get("capabilities", {})

errors = []

for name, host in (("semembed", "semembed"), ("seminstruct", "seminstruct")):
    ep = endpoints.get(name)
    if ep is None:
        errors.append(f"endpoint {name!r} is missing")
        continue
    url = ep.get("url", "")
    if "localhost" in url or "127.0.0.1" in url:
        errors.append(
            f"endpoint {name!r} url {url!r} uses a loopback host; inside Compose it must "
            f"use the {host!r} service hostname"
        )
    elif f"//{host}:" not in url:
        errors.append(f"endpoint {name!r} url {url!r} does not target the {host!r} service")

# Clustering is the whole point of tier 2; a config that silently leaves it off
# would smoke green while proving nothing about the community-summary path.
graph = cfg.get("graph", {})
for key in ("enable_clustering", "clustering_llm"):
    if graph.get(key) is not True:
        errors.append(f"graph.{key} must be true in the tier-2 dev config")

for cap in ("embedding", "community_summary", "query_classification", "answer_synthesis"):
    preferred = capabilities.get(cap, {}).get("preferred", [])
    if not preferred:
        errors.append(f"capability {cap!r} has no preferred endpoint")
        continue
    for target in preferred:
        if target not in endpoints:
            errors.append(f"capability {cap!r} prefers unknown endpoint {target!r}")

if errors:
    for err in errors:
        print(f"  - {err}", file=sys.stderr)
    sys.exit(1)

print(f"{path} wires embedding + 3 LLM capabilities to Compose service hostnames")
PY

if [ "${SEMSOURCE_TIER2_SMOKE_FULL:-0}" != "1" ]; then
	echo
	echo "Composition checks passed. Stack bring-up was NOT run."
	echo "Set SEMSOURCE_TIER2_SMOKE_FULL=1 to pull seminstruct (~1GB) and poll to ready."
	exit 0
fi

echo
echo "SEMSOURCE_TIER2_SMOKE_FULL=1 — bringing up the tier-2 stack"

cleanup() {
	$compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

export SEMSOURCE_CONFIG="${SEMSOURCE_CONFIG:-tiers/tier2-compose-dev.json}"
$compose up -d --wait || fail "tier-2 stack did not become healthy"

http_port="${SEMSOURCE_HTTP_PORT:-8080}"
deadline=$(( $(date +%s) + 300 ))
until curl -fsS "http://localhost:${http_port}/source-manifest/status" 2>/dev/null | grep -q '"phase":"ready"'; do
	[ "$(date +%s)" -lt "$deadline" ] || fail "source-manifest did not reach ready within 300s"
	sleep 5
done
echo "Tier-2 stack reached source-manifest phase=ready"
