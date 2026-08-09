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

# Schema overhead: the fixed per-session cost of registering this tool surface.
# Every registered schema sits in an agent's context for the whole session,
# used or not, so it is measured once and reported session-level — never
# amortized into per-question figures. The honest framing is "fixed cost F
# plus marginal cost per question"; folding F into the questions would hide F.
tools_json=$(curl -sS --max-time 10 -X POST \
	-H 'Content-Type: application/json' \
	-H 'Accept: application/json, text/event-stream' \
	-H 'MCP-Protocol-Version: 2025-06-18' \
	-H "Mcp-Session-Id: $session" \
	-d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
	"$mcp_url" | sed -n 's/^data: //p')
schema_bytes=$(printf '%s' "$tools_json" | wc -c | tr -d ' ')
schema_tools=$(printf '%s' "$tools_json" | jq '.result.tools | length' 2>/dev/null || echo 0)

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

# Grading lives in grade.sh, shared verbatim with the other arms
# (arm-a-grep.sh, arm-c-cosine.sh) so every arm scores with identical
# matcher semantics by construction. See grade.sh for the contract.
. "$here/grade.sh"

# --- run + grade -----------------------------------------------------------
# Grading is deterministic substring matching, deliberately. An LLM judge drifts
# between runs, and a drifting judge cannot support an A/B: a score change would
# be indistinguishable from a judge change.
#
# Each question is asked SCORECARD_REPEATS times and the verdicts compared. This
# exists because a verdict was found to depend on a question's POSITION in the
# run: the same question against an unchanged stack returned the correct passage
# when asked first and lost it entirely when asked last, transiently and
# self-healingly (results/SUMMARY-instrument-diagnosis.md). That is a live
# platform defect, not an instrument artifact.
#
# Disagreement is therefore reported as UNSTABLE and never resolved silently to
# either the passing or the failing result. A warm-up call or a retry-until-pass
# would have produced a clean number by concealing a defect a real caller hits,
# and a scorecard that protects its own score is worse than one that admits it
# cannot measure.
repeats="${SCORECARD_REPEATS:-3}"
echo "[]" > "$out.tmp"
total=0; correct=0
n=$(jq '.questions | length' "$questions")
for i in $(seq 0 $((n - 1))); do
	q=$(jq ".questions[$i]" "$questions")
	id=$(jq -r '.id' <<<"$q")
	band=$(jq -r '.band' <<<"$q")
	tool=$(jq -r '.tool' <<<"$q")
	args=$(jq -c '.args' <<<"$q")

	# Ask the question $repeats times, grading each. The FIRST call's answer and
	# metrics are what get retained — it is the one a real caller's first request
	# corresponds to, and taking the best of N would be the concealment this
	# repeat logic exists to prevent.
	seen_verdicts=""
	for rep in $(seq 1 "$repeats"); do
		answer=$(mcp_call "$tool" "$args" || echo "{}")
		grade_answer
		seen_verdicts="$seen_verdicts$verdict
"
		if [ "$rep" = 1 ]; then
			first_answer="$answer"; first_verdict="$verdict"; first_reason="$reason"
			first_nodes="$nodes"; first_bb="$bodybytes"; first_tb="$topbytes"
			# Context cost: the full decoded result the FIRST call ingested,
			# charged whether it succeeded or returned isError — an error
			# result still costs an agent its bytes. Repeat calls are
			# instability instrumentation, not agent behavior: never charged.
			first_cb=$(printf '%s' "$answer" | wc -c | tr -d ' ')
		fi
	done
	distinct=$(printf '%s' "$seen_verdicts" | sed '/^$/d' | sort -u)
	answer="$first_answer"; nodes="$first_nodes"; bodybytes="$first_bb"; topbytes="$first_tb"
	if [ "$(printf '%s\n' "$distinct" | wc -l | tr -d ' ')" -gt 1 ]; then
		# Never resolve disagreement to either side: that choice is exactly what
		# would hide the platform defect this detects.
		verdict="UNSTABLE"
		reason="verdict varied across $repeats calls: $(printf '%s' "$distinct" | tr '\n' '/' | sed 's|/$||') (first: $first_verdict — $first_reason)"
	else
		verdict="$first_verdict"; reason="$first_reason"
	fi

	total=$((total + 1))
	[ "$verdict" = "correct" ] && correct=$((correct + 1))
	printf '%-5s %-10s %-13s %-10s nodes=%-3s body=%-7s top=%-7s %s\n' \
		"$id" "$band" "$tool" "$verdict" "$nodes" "$bodybytes" "$topbytes" "$reason"

	jq --arg id "$id" --arg band "$band" --arg tool "$tool" --arg v "$verdict" \
	   --arg r "$reason" --argjson n "${nodes:-0}" --argjson bb "${bodybytes:-0}" \
	   --argjson tb "${topbytes:-0}" --argjson cb "${first_cb:-0}" \
	   --arg a "$(printf '%s' "$answer" | head -c 6000)" \
	   '. += [{id:$id, band:$band, tool:$tool, verdict:$v, reason:$r, nodes:$n, body_bytes:$bb, top_body_bytes:$tb, context_bytes:$cb, answer:$a}]' \
	   "$out.tmp" > "$out.tmp2" && mv "$out.tmp2" "$out.tmp"
done

qver=$(jq -r '.version // 0' "$questions")
jq -n --arg label "$label" --argjson score "$correct" --argjson total "$total" \
   --argjson entities "${entities:-0}" --argjson qver "${qver:-0}" \
   --argjson sb "${schema_bytes:-0}" --argjson st "${schema_tools:-0}" \
   --slurpfile r "$out.tmp" \
   '{label:$label, arm:"B", questions_version:$qver, score:$score, total:$total,
     total_entities:$entities,
     schema_overhead:{bytes:$sb, tools:$st},
     results:$r[0]}' > "$out"
rm -f "$out.tmp"

echo
echo "=== $label: $correct/$total  (repeats=$repeats) ==="
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): \([.[] | select(.verdict=="correct")] | length)/\(length)"' "$out"
# Reported next to the score, never folded into it. An UNSTABLE question has no
# defensible verdict — the same question against the same stack answered two
# ways — so counting it as either a pass or a fail would be inventing one.
uns=$(jq '[.results[] | select(.verdict=="UNSTABLE")] | length' "$out")
if [ "$uns" != "0" ]; then
	echo
	echo "!!! $uns UNSTABLE question(s) — verdict depended on the call, not on retrieval"
	jq -r '.results[] | select(.verdict=="UNSTABLE") | "    \(.id): \(.reason)"' "$out"
elif [ "$repeats" = "1" ]; then
	echo "(repeats=1 — this run CANNOT detect instability; do not quote it as evidence of stability)"
fi
echo
echo "--- evidence precision (doc bands) ---"
jq -r '[.results[] | select(.band|startswith("doc"))] |
       "median top-node body: \(( [.[].top_body_bytes] | sort | .[length/2|floor] )) bytes",
       "mean  total body:     \(( [.[].body_bytes] | add / length | floor )) bytes",
       "mean  nodes/answer:   \(( [.[].nodes] | add / length ))"' "$out"
echo
# Bytes are the measured figure; tokens are bytes/4, an estimate, and labeled
# as one. The schema overhead line is session-fixed cost, reported next to the
# per-question figures and never folded into them.
echo "--- context cost (first call per question; tokens ~ bytes/4, estimate) ---"
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): total \([.[].context_bytes // 0] | add) B (~\(([.[].context_bytes // 0] | add) / 4 | floor) tok), mean \(([.[].context_bytes // 0] | add) / length | floor) B"' "$out"
jq -r '"all bands: total \([.results[].context_bytes // 0] | add) B (~\(([.results[].context_bytes // 0] | add) / 4 | floor) tok)"' "$out"
echo "schema overhead (tools/list): ${schema_bytes:-0} B (~$(( ${schema_bytes:-0} / 4 )) tok), ${schema_tools:-0} tools — session-fixed, excluded from the figures above"
if [ "$(jq '[.results[] | select(.band=="discrimination")] | length' "$out")" -gt 0 ]; then
	echo
	echo "--- discrimination (top node answers on its own) ---"
	jq -r '[.results[] | select(.band=="discrimination")] |
	       "passed:     \([.[] | select(.verdict=="correct")] | length)/\(length)",
	       "imprecise:  \([.[] | select(.verdict=="IMPRECISE")] | length) (top node carried the confusable value too)",
	       "MISLEADING: \([.[] | select(.verdict=="MISLEADING")] | length) (top node carried the confusable INSTEAD of the answer)"' "$out"
fi
mis=$(jq '[.results[] | select(.verdict=="MISLEADING")] | length' "$out")
[ "$mis" = "0" ] || echo "!!! $mis MISLEADING result(s) — the top evidence argues for the wrong answer"
fab=$(jq '[.results[] | select(.verdict=="FABRICATED")] | length' "$out")
[ "$fab" = "0" ] || echo "!!! $fab FABRICATION(S) — this outranks every other result"
echo "written: $out"
