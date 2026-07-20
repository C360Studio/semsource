#!/usr/bin/env bash
# Minimal reproduction: fusion silently drops the top-ranked entity from the
# first doc_context call after a burst of diverse queries.
#
#   scripts/scorecard/repro-order-dependence.sh
#
# Deliberately standalone — it does not read questions.json or the grader, so it
# can be handed to semstreams as a reproduction without carrying SemSource's test
# harness with it. Needs a ready stack on SEMSOURCE_HTTP_PORT (default 28080).
#
# WHAT IT SHOWS
#
#   call 1 after the burst : the expected passage is ABSENT from all 20 nodes
#   calls 2..N             : the same query returns it at rank 1
#
# and, at the same instant, `graph.query.semantic` — bypassing fusion — returns
# that passage at rank 1 with an unchanged similarity. So recall is intact and the
# entity is lost downstream, silently: nothing is logged at WARN or ERROR.
#
# ELIMINATED — do not re-test these:
#   * embedding service degrading under load. Run the burst, then make
#     graph.query.semantic the FIRST call after it: the recall list is pristine,
#     identical similarities in identical order, no semembed errors.
#   * mixed statistical/neural vector population. embedder_type resolves once at
#     startup; sampled stored vectors are 384-d, 0% zeros, ~50% negative, while a
#     BM25 vector here is sparse and non-negative.
#   * unstable sorting over tied cosines. Repeated recall is byte-identical
#     including at exact ties.
set -euo pipefail

port="${SEMSOURCE_HTTP_PORT:-28080}"
host="${SEMSOURCE_HOST:-127.0.0.1}"
mcp="http://${host}:${port}/mcp-gateway/mcp"
# The query and the passage it should return. Both are corpus-specific; override
# for a different corpus.
target_query="${REPRO_QUERY:-what port does the seminstruct inference container publish}"
target_handle="${REPRO_HANDLE:-configs-tiers-README-md-0006}"
burst_n="${REPRO_BURST:-21}"
after_n="${REPRO_AFTER:-4}"

hdrs=$(mktemp)
curl -sS --max-time 10 -o /dev/null -D "$hdrs" -X POST \
	-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"repro","version":"1"}}}' \
	"$mcp"
session=$(grep -i '^Mcp-Session-Id:' "$hdrs" | sed -n 's/^[^:]*:[[:space:]]*//p' | tr -d '\r' | head -1)
[ -n "$session" ] || { echo "no MCP session id" >&2; exit 1; }
curl -sS --max-time 5 -o /dev/null -X POST \
	-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' -H "Mcp-Session-Id: $session" \
	-d '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' "$mcp"

ask() {
	curl -sS --max-time 30 -X POST \
		-H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
		-H 'MCP-Protocol-Version: 2025-06-18' -H "Mcp-Session-Id: $session" \
		-d "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"doc_context\",\"arguments\":{\"query\":$(jq -Rc . <<<"$1")}}}" \
		"$mcp" | sed -n 's/^data: //p' | jq -c '.result.content[0].text | fromjson'
}

rank_of() {
	jq -r --arg h "$target_handle" '
	  [.nodes | to_entries[] | select((.value.handle // "") | contains($h)) | .key + 1] |
	  if length == 0 then "ABSENT" else "rank \(.[0])" end'
}

# The burst must be DIVERSE — repeating one query does not reproduce it (24
# identical calls in a session were all correct). Vary the wording per call.
echo "burst: $burst_n distinct doc_context queries ..."
for i in $(seq 1 "$burst_n"); do
	ask "unrelated question number $i about configuration ports services and deployment" >/dev/null || true
done

echo "target query, $after_n consecutive calls:"
for i in $(seq 1 "$after_n"); do
	printf '  call %s: %s\n' "$i" "$(ask "$target_query" | rank_of)"
done

echo
echo "Expected: call 1 ABSENT, calls 2+ rank 1 — transient and self-healing."
echo "Cross-check recall directly (should be rank 1 throughout):"
echo "  docker run --rm --network <project>_c360 natsio/nats-box:latest \\"
echo "    nats -s nats://nats:4222 req graph.query.semantic \\"
echo "    '{\"query\":\"$target_query\",\"limit\":40,\"scope\":[\"<org>.semsource.web\",\"<org>.semsource.config\"]}' --raw"
