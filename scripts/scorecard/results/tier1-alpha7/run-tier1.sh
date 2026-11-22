#!/usr/bin/env bash
# #234 5.3: one tier 1 leg on fresh NATS storage, then a same-config restart
# that counts the provider work a restart repeats (semengine#722).
#   run-tier1.sh <dogfood|osh> <corpus-dir> <questions-file>
# semembed is never recreated between legs, so one provider instance serves
# every run (design decision 5).
set -euo pipefail
corpus=$1 tree=$2 questions=$3
O=scripts/scorecard/results/tier1-alpha7
E="$O/evidence-$corpus"; mkdir -p "$E"
dc() { docker compose -p semsource234 -f docker-compose.yml -f scripts/scorecard/compose.semembed-host.yml \
	-f "$O/compose.tier1.yml" --profile semantic "$@"; }
export TIER1_CONFIGS="$PWD/$O" TIER1_CORPUS="$tree" SEMSOURCE_CONFIG=tier1.json SEMEMBED_HOST_PORT=27081
metric() { curl -s localhost:27081/metrics | awk -v m="$1" '$1 == m {print $2}'; }
status() { curl -s localhost:27080/source-manifest/status; }
settled() { status | jq -e '(.phase == "ready") and (.index.ready == true) and (.embedding.ready == true)' >/dev/null 2>&1; }
graph() { curl -s 'localhost:27822/jsz?streams=true' | jq -c '[.account_details[]?.stream_detail[]? | select(.name=="GRAPH") | {messages:.state.messages, bytes:.state.bytes}][0]'; }
# wait_settled <label>: poll concrete state every 30 s; abort when nothing
# moves (entities, embedding revision, provider requests) for 10 minutes.
wait_settled() {
	local label=$1 last="" still=0 t0; t0=$(date +%s)
	until settled; do
		local s now; s=$(status | jq -c '{phase, total_entities, index:.index.indexed_revision, emb:.embedding.indexed_revision, emb_target:.embedding.target_revision, emb_state:.embedding.state}' 2>/dev/null || echo '{}')
		now="$s $(metric semembed_requests_total)"
		echo "$(date -u +%H:%M:%S) $label $now graph=$(graph)" | tee -a "$E/poll.log"
		if [ "$now" = "$last" ]; then still=$((still + 30)); else still=0; last=$now; fi
		[ "$still" -lt 600 ] || { echo "WEDGED: no progress for 10 minutes at $label" | tee -a "$E/poll.log"; exit 3; }
		sleep 30
	done
	echo "$label settled in $(( $(date +%s) - t0 )) s" | tee -a "$E/poll.log"
}
dc rm -sfv semsource nats >/dev/null 2>&1 || true
docker volume rm semsource234_nats-data >/dev/null 2>&1 || true
dc up -d semembed
r0=$(metric semembed_requests_total); k0=$(metric semembed_tokens_processed_total)
dc up -d --build nats semsource
wait_settled seed
r1=$(metric semembed_requests_total); k1=$(metric semembed_tokens_processed_total)
echo "seed provider requests=$((r1 - r0)) tokens=$((k1 - k0))" | tee -a "$E/poll.log"
status > "$E/status-seeded.json"; graph > "$E/graph-stream-seeded.json"
SEMSOURCE_HTTP_PORT=27080 SCORECARD_QUESTIONS="$questions" scripts/scorecard/run.sh "tier1-alpha7-$corpus-B"
# Same config, same storage: what does a restart embed again?
r2=$(metric semembed_requests_total); k2=$(metric semembed_tokens_processed_total)
dc restart semsource
until curl -sf localhost:27080/source-manifest/status >/dev/null; do sleep 2; done
wait_settled restart
r3=$(metric semembed_requests_total); k3=$(metric semembed_tokens_processed_total)
echo "restart provider requests=$((r3 - r2)) tokens=$((k3 - k2))" | tee -a "$E/poll.log"
docker logs semsource234-semsource-1 > "$E/semsource.log" 2>&1 || true
