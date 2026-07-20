#!/usr/bin/env bash
# Run the retrieval scorecard against a live SemSource stack.
#
#   scripts/scorecard/run.sh <label>
#
# Writes scripts/scorecard/results/<label>.json and prints a per-band summary.
# The stack must already be up and report phase "ready"; this script does not
# provision anything, so the same harness can be pointed at two different builds.
#
# See README.md in this directory for the A/B procedure and the comparability
# rules — in particular, a score is only meaningful against another score taken
# with the SAME questions.json.
set -euo pipefail

label="${1:?usage: run.sh <label>   (e.g. run.sh pre-chunking)}"
here="$(cd "$(dirname "$0")" && pwd)"
port="${SEMSOURCE_HTTP_PORT:-28080}"
host="${SEMSOURCE_HOST:-127.0.0.1}"
mcp_url="http://${host}:${port}/mcp-gateway/mcp"
status_url="http://${host}:${port}/source-manifest/status"
questions="${SCORECARD_QUESTIONS:-$here/questions.json}"
outdir="$here/results"
mkdir -p "$outdir"
out="$outdir/${label}.json"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

# --- readiness -------------------------------------------------------------
# Never score a stack that is still seeding: a miss caused by an incomplete
# index is not a retrieval failure, and grading one as if it were would poison
# the comparison.
echo "waiting for ${status_url} to report ready..."
deadline=$(( $(date +%s) + ${SCORECARD_READY_TIMEOUT:-900} ))
until curl -sS --max-time 5 "$status_url" 2>/dev/null | grep -q '"phase":"ready"'; do
	[ "$(date +%s)" -lt "$deadline" ] || { echo "stack never reported ready" >&2; exit 1; }
	sleep 5
done
echo "stack is ready"

# Readiness means seeded, not embedded. Embeddings land asynchronously after
# ingest, and scoring semantic retrieval before they exist measures nothing.
sleep "${SCORECARD_EMBED_SETTLE:-60}"

# --- MCP session -----------------------------------------------------------
hdrs=$(mktemp); body=$(mktemp)
curl -sS --max-time 10 -o "$body" -D "$hdrs" -X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"scorecard","version":"1"}}}' \
	"$mcp_url" >/dev/null
session=$(grep -i '^Mcp-Session-Id:' "$hdrs" | sed -n 's/^[^:]*:[[:space:]]*//p' | tr -d '\r' | head -1)
[ -n "$session" ] || { echo "no MCP session id; body: $(cat "$body")" >&2; exit 1; }

curl -sS --max-time 5 -o /dev/null -X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-H "Mcp-Session-Id: $session" \
	-d '{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
	"$mcp_url"

mcp_call() {
	curl -sS --max-time "${SCORECARD_CALL_TIMEOUT:-30}" -X POST \
		-H 'Content-Type: application/json' \
		-H 'Accept: application/json, text/event-stream' \
		-H 'MCP-Protocol-Version: 2025-06-18' \
		-H "Mcp-Session-Id: $session" \
		-d "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
		"$mcp_url" | sed -n 's/^data: //p' | tr -d '\\'
}

# --- run + grade -----------------------------------------------------------
# Grading is deterministic substring matching, deliberately. An LLM judge drifts
# between runs, and a drifting judge cannot support an A/B: a score change would
# be indistinguishable from a judge change.
echo "[]" > "$out.tmp"
total=0; correct=0
n=$(jq '.questions | length' "$questions")
for i in $(seq 0 $((n - 1))); do
	q=$(jq ".questions[$i]" "$questions")
	id=$(jq -r '.id' <<<"$q")
	band=$(jq -r '.band' <<<"$q")
	tool=$(jq -r '.tool' <<<"$q")
	args=$(jq -c '.args' <<<"$q")

	answer=$(mcp_call "$tool" "$args" || echo "")
	lower=$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')

	verdict="correct"; reason=""
	# isError is a failed call, never a graded answer.
	if printf '%s' "$answer" | grep -q '"isError":true'; then
		verdict="error"; reason="tool returned isError"
	fi
	if [ "$verdict" = "correct" ]; then
		for want in $(jq -r '(.expect_all // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$lower" | grep -qF "$w" || { verdict="miss"; reason="missing required: $w"; break; }
		done
	fi
	if [ "$verdict" = "correct" ] && [ "$(jq '(.expect_any // []) | length' <<<"$q")" -gt 0 ]; then
		hit=0
		for want in $(jq -r '(.expect_any // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$lower" | grep -qF "$w" && { hit=1; break; }
		done
		[ "$hit" = 1 ] || { verdict="miss"; reason="none of expect_any present"; }
	fi
	# A fabrication is graded separately from a miss: they are different failures
	# and conflating them hides the one that actually matters.
	for bad in $(jq -r '(.expect_none // [])[]' <<<"$q" | tr ' ' '\001'); do
		b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
		printf '%s' "$lower" | grep -qF "$b" && { verdict="FABRICATED"; reason="asserted: $b"; break; }
	done

	total=$((total + 1))
	[ "$verdict" = "correct" ] && correct=$((correct + 1))
	printf '%-5s %-10s %-13s %s %s\n' "$id" "$band" "$tool" "$verdict" "$reason"

	jq --arg id "$id" --arg band "$band" --arg tool "$tool" --arg v "$verdict" \
	   --arg r "$reason" --arg a "$(printf '%s' "$answer" | head -c 4000)" \
	   '. += [{id:$id, band:$band, tool:$tool, verdict:$v, reason:$r, answer:$a}]' \
	   "$out.tmp" > "$out.tmp2" && mv "$out.tmp2" "$out.tmp"
done

jq -n --arg label "$label" --argjson score "$correct" --argjson total "$total" \
   --slurpfile r "$out.tmp" \
   '{label:$label, score:$score, total:$total, results:$r[0]}' > "$out"
rm -f "$out.tmp"

echo
echo "=== $label: $correct/$total ==="
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): \([.[] | select(.verdict=="correct")] | length)/\(length)"' "$out"
fab=$(jq '[.results[] | select(.verdict=="FABRICATED")] | length' "$out")
[ "$fab" = "0" ] || echo "!!! $fab FABRICATION(S) — this outranks every other result"
echo "written: $out"
