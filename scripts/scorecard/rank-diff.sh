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
# (fusion tools; name+path where a node has no handle) or match ids
# (graph_search). The first call of each question is compared — repeats are
# instability instrumentation, not a second ranking.
#
# Exits 1 when any question differs, so it can gate a run.
set -euo pipefail

a="${1:?usage: rank-diff.sh <first.json> <second.json>}"
b="${2:?usage: rank-diff.sh <first.json> <second.json>}"

ranks() { # $1 = results file; prints one JSON object {id: [ranked identities]}
	jq -c '[.results[] | {key: .id, value: (
		(.answer | try fromjson catch {}) as $ans
		| if ($ans | type) != "object" or $ans.isError == true then ["<error>"]
		  else [((($ans.nodes // [])[]) | (.handle // ((.name // "") + "@" + (.path // "")))),
		        ((($ans.matches // [])[]) | (.id // ""))]
		  end)}] | from_entries' "$1"
}

ra=$(ranks "$a"); rb=$(ranks "$b")
jq -rn --argjson a "$ra" --argjson b "$rb" '
	[($a | keys[]) as $id | {id: $id, x: $a[$id], y: ($b[$id] // null)}
	 | if .y == null then .s = "absent" elif .x == .y then .s = "identical"
	   elif (.x | sort) == (.y | sort) then .s = "reordered"
	   else .s = "different" end
	 | .first = ([range(0; ([(.x | length), ((.y // []) | length)] | max))] as $r
	             | [$r[] as $i | select(.x[$i] != (.y // [])[$i]) | $i] | first // null)] as $rows
	| ($rows[] | "\(.id)\t\(.s)\(if .first == null then "" else "\tfirst divergence at rank \(.first + 1)" end)"),
	  "",
	  "identical \([$rows[] | select(.s == "identical")] | length)/\($rows | length)" +
	  "  reordered \([$rows[] | select(.s == "reordered")] | length)" +
	  "  different \([$rows[] | select(.s == "different")] | length)" +
	  "  absent \([$rows[] | select(.s == "absent")] | length)",
	  (if [$rows[] | select(.s != "identical")] | length > 0 then "RANK-UNSTABLE" else "RANK-STABLE" end)
' | tee /dev/stderr | tail -1 | grep -qx RANK-STABLE 2>/dev/null
