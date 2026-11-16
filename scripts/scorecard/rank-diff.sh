#!/usr/bin/env bash
# R1 rank-stability probe (semengine#10): are two runs over the same corpus
# rank-identical, not merely verdict-identical?
#
#   scripts/scorecard/rank-diff.sh <first.json> <second.json>
#
# The procedure that produces the two files (README, "R1 rank stability"):
# seed the corpus and score it, restart the substrate, seed the same corpus with
# the source order reversed, and score it again. Both runs retain every answer in
# full, so the probe is a pure function of the two result files.
#
# A question's ranking is the ordered list of what it returned: node handles
# (fusion tools), node ids (arm C), name@path, or match ids (graph_search). A
# returned node or match with none of those is refused. The first call of each question is compared — repeats are
# instability instrumentation, not a second ranking.
#
# This is an acceptance gate, so it refuses evidence it cannot rank (exit 2):
# an empty or duplicated question set, two files with different question sets,
# and any answer that is an error, not JSON, or carries none of nodes, matches,
# or misses. An empty nodes or matches array, or a code_context answer that
# reports only misses, is a legitimate no-hit ranking. Exits 1 when any
# question's ranking differs.
set -euo pipefail

a="${1:?usage: rank-diff.sh <first.json> <second.json>}"
b="${2:?usage: rank-diff.sh <first.json> <second.json>}"

ranks() { # $1 = results file; prints {id: [ranked identities]} or fails
	jq -ce '
		def str: type == "string" and length > 0;
		# A node is identified by its handle (fusion tools), its id (arm C), or
		# name@path when both are present; anything else would collapse distinct
		# entities into one identity and certify a reordering as stable.
		def nodeid($q): if type != "object" then error("\($q): node is not an object")
			elif (.handle | str) then .handle elif (.id | str) then .id
			elif (.name | str) and (.path | str) then .name + "@" + .path
			else error("\($q): node has no handle, id, or name and path") end;
		def matchid($q): if type == "object" and (.id | str) then .id
			else error("\($q): match has no id") end;
		(.results // []) as $r
		| if ($r | length) == 0 then error("no results") else . end
		| if ([$r[].id] | unique | length) != ($r | length) then error("duplicate question ids") else . end
		| [$r[] | .id as $id | {key: $id, value: (
			(.answer | try fromjson catch error("\($id): answer is not JSON")) as $ans
			| if ($ans | type) != "object" then error("\($id): answer is not an object")
			  elif $ans.isError == true then error("\($id): answer is an error: \($ans.error // "isError")")
			  elif ([$ans.nodes, $ans.matches, $ans.misses] | map(type == "array") | any | not) then error("\($id): answer has no nodes, matches, or misses")
			  else [((($ans.nodes // [])[]) | nodeid($id)), ((($ans.matches // [])[]) | matchid($id))]
			  end)}] | from_entries' "$1"
}

ra=$(ranks "$a") || { echo "rank-diff: refusing $a" >&2; exit 2; }
rb=$(ranks "$b") || { echo "rank-diff: refusing $b" >&2; exit 2; }
if [ "$(jq -cn --argjson a "$ra" --argjson b "$rb" '($a | keys) == ($b | keys)')" != true ]; then
	echo "rank-diff: refusing: the two files carry different question sets" >&2
	jq -rn --argjson a "$ra" --argjson b "$rb" \
		'"  only in first: \(($a | keys) - ($b | keys))", "  only in second: \(($b | keys) - ($a | keys))"' >&2
	exit 2
fi

report=$(jq -rn --argjson a "$ra" --argjson b "$rb" '
	[($a | keys[]) as $id | {id: $id, x: $a[$id], y: $b[$id]}
	 | if .x == .y then .s = "identical"
	   elif (.x | sort) == (.y | sort) then .s = "reordered"
	   else .s = "different" end
	 | .first = ([range(0; ([(.x | length), (.y | length)] | max))] as $r
	             | [$r[] as $i | select(.x[$i] != .y[$i]) | $i] | first // null)] as $rows
	| ($rows[] | "\(.id)\t\(.s)\(if .first == null then "" else "\tfirst divergence at rank \(.first + 1)" end)"),
	  "",
	  "identical \([$rows[] | select(.s == "identical")] | length)/\($rows | length)" +
	  "  reordered \([$rows[] | select(.s == "reordered")] | length)" +
	  "  different \([$rows[] | select(.s == "different")] | length)",
	  (if [$rows[] | select(.s != "identical")] | length > 0 then "RANK-UNSTABLE" else "RANK-STABLE" end)')
printf '%s\n' "$report"
[ "$(printf '%s\n' "$report" | tail -1)" = RANK-STABLE ]
