#!/usr/bin/env bash
# scripts/scorecard/first-hit.sh <questions.json> <results.json>
#
# First-hit rank: per question, the 1-based rank of the first node whose content
# (name, body, related names; plus relation file stems for opted-in questions),
# or the first graph_search match whose label or property values do,
# holds an expected literal (expect_all and expect_any). Prints MRR and hits@1/@5/@20.
q="$1"; r="$2"
jq -n --slurpfile q "$q" --slurpfile r "$r" '
	($q[0].questions | map({key: .id, value: .}) | from_entries) as $Q
	| [ $r[0].results[] | . as $res | $Q[$res.id] as $qq
	    | (([$qq.expect_all // [], $qq.expect_any // []] | add) | map(ascii_downcase)) as $want
	    | select($want | length > 0)
	    | ($res.answer | fromjson) as $a
	    | ([ ($a.nodes // [])[] | [(.name // ""), (.body // ""),
	          ((.relations // {}) | to_entries[] | .value[]? | (.name // "")),
	          (if $qq.grade_relation_files then ((.relations // {}) | to_entries[] | .value[]? | (.path // "") | split("/") | last | sub("\\.[^.]*$"; "")) else empty end)]
	        | join("\n") | ascii_downcase ]
	    + [ ($a.matches // [])[] | [(.label // ""), ((.properties // {}) | .[] | tostring)] | join("\n") | ascii_downcase ])
	    as $nodes
	    | ([range(0; $nodes | length) as $i | select(any($want[]; . as $w | $nodes[$i] | contains($w))) | $i + 1] | first // null) as $rank
	    | {id: $res.id, rank: $rank} ]
	| {n: length,
	   mrr: (([.[] | if .rank then 1 / .rank else 0 end] | add) / length * 1000 | round / 1000),
	   at1: ([.[] | select(.rank == 1)] | length), at5: ([.[] | select(.rank != null and .rank <= 5)] | length),
	   at20: ([.[] | select(.rank != null)] | length),
	   ranks: (map("\(.id)=\(.rank // "-")") | join(" "))}'
