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
# Never score a stack that is still catching up: a miss caused by an incomplete
# index is not a retrieval failure, and grading one as such would poison the
# comparison — worse, scoring one side of an A/B before it settles and the other
# after would produce a difference that is pure timing.
#
# Gate on all three signals the status payload publishes, not on phase alone.
# Its own note states the contract: a miss is a genuine absence only when phase
# is "ready" AND the relevant index signal is ready — phase covers seeding,
# index.ready covers NAME_INDEX (code_context / code_impact), embedding.ready
# covers the semantic pipeline (code_search / doc_context). Waiting on a fixed
# sleep instead would be guessing at something the product reports exactly.
echo "waiting for phase=ready + index.ready + embedding.ready ..."
deadline=$(( $(date +%s) + ${SCORECARD_READY_TIMEOUT:-1800} ))
while :; do
	s=$(curl -sS --max-time 5 "$status_url" 2>/dev/null || echo '{}')
	gate=$(printf '%s' "$s" | jq -r '[(.phase == "ready"), (.index.ready == true), (.embedding.ready == true)] | all' 2>/dev/null || echo false)
	[ "$gate" = "true" ] && break
	[ "$(date +%s)" -lt "$deadline" ] || {
		echo "stack never became fully ready: $(printf '%s' "$s" | jq -c '{phase, index:.index.ready, embedding:.embedding.ready, total_entities}')" >&2
		exit 1
	}
	printf '  %s\n' "$(printf '%s' "$s" | jq -c '{phase, index:.index.ready, embedding:.embedding.ready, total_entities}' 2>/dev/null || echo waiting)"
	sleep 10
done
entities=$(curl -sS --max-time 5 "$status_url" | jq -r '.total_entities')
echo "ready — total_entities=$entities"

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

# Returns the tool's inner result as parseable JSON. The MCP frame carries it
# escaped inside content[0].text; decode it properly rather than stripping
# backslashes, so evidence can be measured and not merely grepped.
mcp_call() {
	curl -sS --max-time "${SCORECARD_CALL_TIMEOUT:-30}" -X POST \
		-H 'Content-Type: application/json' \
		-H 'Accept: application/json, text/event-stream' \
		-H 'MCP-Protocol-Version: 2025-06-18' \
		-H "Mcp-Session-Id: $session" \
		-d "{\"jsonrpc\":\"2.0\",\"id\":9,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" \
		"$mcp_url" | sed -n 's/^data: //p' \
		| jq -c 'if .result.content[0].text then (.result.content[0].text | fromjson) else . end' 2>/dev/null \
		|| echo '{}'
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

	answer=$(mcp_call "$tool" "$args" || echo "{}")
	lower=$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')

	# Evidence precision is the measure that actually separates whole-file
	# retrieval from passage retrieval. Fact-presence alone cannot: a whole-file
	# body trivially contains every fact in the file, so a substring matcher
	# passes whether the system found the right paragraph or just dumped the
	# document. Bytes-per-answer is the difference.
	nodes=$(printf '%s' "$answer" | jq '(.nodes // []) | length' 2>/dev/null || echo 0)
	bodybytes=$(printf '%s' "$answer" | jq '[(.nodes // [])[] | (.body // "") | length] | add // 0' 2>/dev/null || echo 0)
	topbytes=$(printf '%s' "$answer" | jq '((.nodes // [])[0].body // "") | length' 2>/dev/null || echo 0)
	topbody=$(printf '%s' "$answer" | jq -r '((.nodes // [])[0].body // "")' 2>/dev/null || echo "")
	toplower=$(printf '%s' "$topbody" | tr '[:upper:]' '[:lower:]')

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
	# Discrimination questions grade the TOP node alone, not the union of every
	# node returned. Grading the union cannot separate the two systems: an answer
	# carries up to 20 passages, so a confusable value elsewhere in the same
	# document rides along even when retrieval ranked the right passage first.
	# The claim being tested is narrower and is the one that matters to an agent:
	# the single best piece of evidence answers the question on its own.
	if [ "$verdict" = "correct" ]; then
		for want in $(jq -r '(.expect_top_all // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$toplower" | grep -qF "$w" || { verdict="miss"; reason="top node missing: $w"; break; }
		done
	fi
	# IMPRECISE is deliberately NOT folded into FABRICATED. A whole-file body that
	# carries both the answer and its confusable twin has invented nothing — it is
	# imprecise, not dishonest. Conflating them would destroy the fabrication
	# signal, which is the one result that outranks everything else here.
	if [ "$verdict" = "correct" ]; then
		for bad in $(jq -r '(.expect_top_none // [])[]' <<<"$q" | tr ' ' '\001'); do
			b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$toplower" | grep -qF "$b" &&
				{ verdict="IMPRECISE"; reason="top node also carries the confusable: $b"; break; }
		done
	fi
	# A fabrication is graded separately from a miss: they are different failures
	# and conflating them hides the one that actually matters.
	for bad in $(jq -r '(.expect_none // [])[]' <<<"$q" | tr ' ' '\001'); do
		b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
		printf '%s' "$lower" | grep -qF "$b" && { verdict="FABRICATED"; reason="asserted: $b"; break; }
	done

	total=$((total + 1))
	[ "$verdict" = "correct" ] && correct=$((correct + 1))
	printf '%-5s %-10s %-13s %-10s nodes=%-3s body=%-7s top=%-7s %s\n' \
		"$id" "$band" "$tool" "$verdict" "$nodes" "$bodybytes" "$topbytes" "$reason"

	jq --arg id "$id" --arg band "$band" --arg tool "$tool" --arg v "$verdict" \
	   --arg r "$reason" --argjson n "${nodes:-0}" --argjson bb "${bodybytes:-0}" \
	   --argjson tb "${topbytes:-0}" --arg a "$(printf '%s' "$answer" | head -c 6000)" \
	   '. += [{id:$id, band:$band, tool:$tool, verdict:$v, reason:$r, nodes:$n, body_bytes:$bb, top_body_bytes:$tb, answer:$a}]' \
	   "$out.tmp" > "$out.tmp2" && mv "$out.tmp2" "$out.tmp"
done

jq -n --arg label "$label" --argjson score "$correct" --argjson total "$total" \
   --argjson entities "${entities:-0}" --slurpfile r "$out.tmp" \
   '{label:$label, score:$score, total:$total, total_entities:$entities, results:$r[0]}' > "$out"
rm -f "$out.tmp"

echo
echo "=== $label: $correct/$total ==="
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): \([.[] | select(.verdict=="correct")] | length)/\(length)"' "$out"
echo
echo "--- evidence precision (doc bands) ---"
jq -r '[.results[] | select(.band|startswith("doc"))] |
       "median top-node body: \(( [.[].top_body_bytes] | sort | .[length/2|floor] )) bytes",
       "mean  total body:     \(( [.[].body_bytes] | add / length | floor )) bytes",
       "mean  nodes/answer:   \(( [.[].nodes] | add / length ))"' "$out"
if [ "$(jq '[.results[] | select(.band=="discrimination")] | length' "$out")" -gt 0 ]; then
	echo
	echo "--- discrimination (top node answers on its own) ---"
	jq -r '[.results[] | select(.band=="discrimination")] |
	       "passed:    \([.[] | select(.verdict=="correct")] | length)/\(length)",
	       "imprecise: \([.[] | select(.verdict=="IMPRECISE")] | length) (top node carried the confusable value too)"' "$out"
fi
fab=$(jq '[.results[] | select(.verdict=="FABRICATED")] | length' "$out")
[ "$fab" = "0" ] || echo "!!! $fab FABRICATION(S) — this outranks every other result"
echo "written: $out"
