#!/usr/bin/env bash
# R1 pair on fresh dogfood storage with retained restart evidence.
set -euo pipefail
O=scripts/scorecard/results/beta10-tier0
E="$O/r1-evidence"; mkdir -p "$E"
dc() { docker compose -p semb10 -f docker-compose.yml -f "$O/compose.semb10.yml" "$@"; }
c=semb10-semsource-1
ingested() { docker logs "$c" 2>&1 | grep -c -E 'Initial index complete|Doc-source initial ingest complete|Cfgfile-source initial ingest complete' || true; }
dc down -v
SEMSOURCE_CONFIG=tier0.json dc up -d nats semsource
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh beta10-tier0-dogfood-B
docker inspect "$c" --format '{{.Id}} {{.State.StartedAt}} {{join .Args " "}}' > "$E/before.txt"
SEMSOURCE_CONFIG=tier0-reversed.json dc up -d --no-deps semsource
docker inspect "$c" --format '{{.Id}} {{.State.StartedAt}} {{join .Args " "}}' > "$E/after.txt"
[ "$(cut -d' ' -f1 "$E/before.txt")" != "$(cut -d' ' -f1 "$E/after.txt")" ] || { echo "semsource was not recreated" >&2; exit 1; }
grep -q 'tier0-reversed.json' "$E/after.txt" || { echo "restarted without the reversed config" >&2; exit 1; }
# Bounded: the NEW process must log all three initial ingests before leg 2 is scored.
deadline=$(( $(date +%s) + 900 ))
until [ "$(ingested)" -ge 3 ]; do
	[ "$(date +%s)" -lt "$deadline" ] || { echo "restarted semsource never finished its initial ingests" >&2; exit 1; }
	sleep 2
done
docker logs "$c" 2>&1 | grep -E 'Starting initial code index|Initial index complete|Starting (doc|cfgfile)-source initial ingest|(Doc|Cfgfile)-source initial ingest complete' > "$E/reseed-log.jsonl"
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh beta10-tier0-dogfood-r1b
scripts/scorecard/rank-diff.sh scripts/scorecard/results/beta10-tier0-dogfood-B.json scripts/scorecard/results/beta10-tier0-dogfood-r1b.json | tee "$E/rank-diff.txt"
