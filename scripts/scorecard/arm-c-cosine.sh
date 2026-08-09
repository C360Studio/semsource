#!/usr/bin/env bash
# Arm C of the retrieval cost comparison: embeddings without the query
# machinery. Ranks the product's OWN stored vectors by cosine against the
# query embedding — the chunking and embedding are held identical to arm B,
# so the C-vs-B delta isolates what the query/fusion layer adds over raw
# cosine similarity.
#
#   scripts/scorecard/arm-c-cosine.sh <label>
#
# Procedure (design D3 of scorecard-token-cost-arms), per question:
#   1. embed args.query via semembed /v1/embeddings, query_prefix on the
#      query side only (the pattern that reproduced live cosines exactly in
#      the default-vs-override ranking work)
#   2. cosine-rank every EMBEDDING_INDEX vector (dumped once per run via
#      armc-dump, read-only over the host-mapped NATS port); ties break by
#      entity id
#   3. fetch top-K bodies through ENTITY_STATES storage_ref -> CONTENT —
#      the graph is a dumb byte store here, consulted only AFTER ranking,
#      so the isolation claim holds
#   4. grade top-K bodies with the shared grader (rank-1 body = top node);
#      charge body bytes plus the query-embedding request/response bytes
#
# Needs the scorecard stack up with semembed published to the host
# (compose.semembed-host.yml) and NATS on its host port. Gates on the same
# three readiness signals as run.sh: scoring a still-seeding index would
# measure timing, not retrieval.
set -euo pipefail
export LC_ALL=C

label="${1:?usage: arm-c-cosine.sh <label>}"
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
port="${SEMSOURCE_HTTP_PORT:-28080}"
host="${SEMSOURCE_HOST:-127.0.0.1}"
status_url="http://${host}:${port}/source-manifest/status"
embed_url="${SEMEMBED_URL:-http://127.0.0.1:28081}/v1/embeddings"
nats_url="${SCORECARD_NATS:-nats://127.0.0.1:24222}"
model="${SCORECARD_EMBED_MODEL:-Snowflake/snowflake-arctic-embed-s}"
prefix="${SCORECARD_QUERY_PREFIX:-Represent this sentence for searching relevant passages: }"
topk="${SCORECARD_ARM_C_TOPK:-20}"
questions="${SCORECARD_QUESTIONS:-$here/questions.json}"
outdir="$here/results"
mkdir -p "$outdir"
out="$outdir/${label}.json"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required (armc-dump)" >&2; exit 1; }

. "$here/grade.sh"

# --- readiness (same gate as run.sh; see its comment for why) --------------
echo "waiting for phase=ready + index.ready + embedding.ready ..."
deadline=$(( $(date +%s) + ${SCORECARD_READY_TIMEOUT:-1800} ))
while :; do
	s=$(curl -sS --max-time 5 "$status_url" 2>/dev/null || echo '{}')
	gate=$(printf '%s' "$s" | jq -r '[(.phase == "ready"), (.index.ready == true), (.embedding.ready == true)] | all' 2>/dev/null || echo false)
	[ "$gate" = "true" ] && break
	[ "$(date +%s)" -lt "$deadline" ] || { echo "stack never became fully ready" >&2; exit 1; }
	sleep 10
done
entities=$(curl -sS --max-time 5 "$status_url" | jq -r '.total_entities')
echo "ready — total_entities=$entities"

# --- vectors: one dump per run ---------------------------------------------
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "dumping EMBEDDING_INDEX vectors ..."
(cd "$repo" && go run ./scripts/scorecard/armc-dump -nats "$nats_url" vectors) > "$tmp/vectors.tsv"
nvec=$(wc -l < "$tmp/vectors.tsv" | tr -d ' ')
echo "vectors: $nvec"
[ "$nvec" -gt 0 ] || { echo "no vectors — is the stack seeded?" >&2; exit 1; }

embed_query() { # $1 = query text; prints "reqbytes respbytes vec" or fails
	req=$(jq -nc --arg m "$model" --arg q "$prefix$1" '{model:$m, input:[$q]}')
	resp=$(curl -sS --max-time 60 -H 'Content-Type: application/json' -d "$req" "$embed_url" 2>/dev/null) || return 1
	vec=$(printf '%s' "$resp" | jq -r '.data[0].embedding | map(tostring) | join(",")' 2>/dev/null) || return 1
	[ -n "$vec" ] && [ "$vec" != "null" ] || return 1
	printf '%s %s %s\n' "$(printf '%s' "$req" | wc -c | tr -d ' ')" \
		"$(printf '%s' "$resp" | wc -c | tr -d ' ')" "$vec"
}

echo "[]" > "$out.tmp"
total=0; correct=0
n=$(jq '.questions | length' "$questions")
for i in $(seq 0 $((n - 1))); do
	q=$(jq ".questions[$i]" "$questions")
	id=$(jq -r '.id' <<<"$q")
	band=$(jq -r '.band' <<<"$q")
	query=$(jq -r '.args.query' <<<"$q")

	if ! emb=$(embed_query "$query"); then
		answer='{"isError":true,"error":"embedding request failed"}'
		grade_answer
		context_bytes=0; kids=0
	else
		reqb=$(printf '%s' "$emb" | cut -d' ' -f1)
		respb=$(printf '%s' "$emb" | cut -d' ' -f2)
		qvec=$(printf '%s' "$emb" | cut -d' ' -f3)

		# Rank every stored vector; ties break by entity id so the order is
		# deterministic for a fixed index.
		awk -F'\t' -v qv="$qvec" '
			BEGIN { n = split(qv, qq, ","); qn = 0
			        for (i = 1; i <= n; i++) qn += qq[i]*qq[i]; qn = sqrt(qn) }
			{ split($2, v, ","); d = 0; vn = 0
			  for (i = 1; i <= n; i++) { d += qq[i]*v[i]; vn += v[i]*v[i] }
			  vn = sqrt(vn)
			  if (vn > 0 && qn > 0) printf "%.6f\t%s\n", d/(vn*qn), $1 }' \
			"$tmp/vectors.tsv" | sort -t"$(printf '\t')" -k1,1rn -k2,2 \
			| awk -F'\t' -v k="$topk" 'NR <= k { print $2 }' > "$tmp/top.ids"
		kids=$(wc -l < "$tmp/top.ids" | tr -d ' ')

		# Bodies, AFTER ranking, in rank order (armc-dump preserves argument
		# order). An entity with no offloaded body contributes an empty node.
		# shellcheck disable=SC2046
		(cd "$repo" && go run ./scripts/scorecard/armc-dump -nats "$nats_url" bodies $(cat "$tmp/top.ids")) \
			> "$tmp/bodies.ndjson"
		answer=$(jq -sc '{nodes: [.[] | {id: .id, body: ((.body // "") | @base64d)}]}' "$tmp/bodies.ndjson")
		grade_answer
		bodysum=$(jq -s '[.[].bytes // 0] | add // 0' "$tmp/bodies.ndjson")
		context_bytes=$((bodysum + reqb + respb))
	fi

	total=$((total + 1))
	[ "$verdict" = "correct" ] && correct=$((correct + 1))
	printf '%-5s %-14s %-10s k=%-3s ctx=%-8s %s\n' \
		"$id" "$band" "$verdict" "$kids" "$context_bytes" "$reason"

	jq --arg id "$id" --arg band "$band" --arg v "$verdict" --arg r "$reason" \
	   --argjson n "${nodes:-0}" --argjson bb "${bodybytes:-0}" \
	   --argjson tb "${topbytes:-0}" --argjson cb "$context_bytes" \
	   --arg a "$(printf '%s' "$answer" | head -c 6000)" \
	   '. += [{id:$id, band:$band, tool:"cosine", verdict:$v, reason:$r, nodes:$n,
	           body_bytes:$bb, top_body_bytes:$tb, context_bytes:$cb, answer:$a}]' \
	   "$out.tmp" > "$out.tmp2" && mv "$out.tmp2" "$out.tmp"
done

qver=$(jq -r '.version // 0' "$questions")
jq -n --arg label "$label" --argjson score "$correct" --argjson total "$total" \
   --argjson qver "${qver:-0}" --argjson entities "${entities:-0}" \
   --argjson nvec "$nvec" --argjson topk "$topk" --slurpfile r "$out.tmp" \
   '{label:$label, arm:"C", questions_version:$qver, score:$score, total:$total,
     total_entities:$entities, vector_source:"kv", vectors:$nvec, topk:$topk,
     results:$r[0]}' > "$out"
rm -f "$out.tmp"

echo
echo "=== $label (arm C, cosine over $nvec product vectors, top-$topk): $correct/$total ==="
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): \([.[] | select(.verdict=="correct")] | length)/\(length)"' "$out"
echo
echo "--- context cost (top-$topk bodies + embed round-trip; tokens ~ bytes/4, estimate) ---"
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): total \([.[].context_bytes // 0] | add) B (~\(([.[].context_bytes // 0] | add) / 4 | floor) tok), mean \(([.[].context_bytes // 0] | add) / length | floor) B"' "$out"
jq -r '"all bands: total \([.results[].context_bytes // 0] | add) B (~\(([.results[].context_bytes // 0] | add) / 4 | floor) tok)"' "$out"
echo "written: $out"
