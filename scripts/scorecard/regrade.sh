#!/usr/bin/env bash
# Regrade a results file from its retained answers with the current grader and
# question set, without re-running the stack.
#
#   scripts/scorecard/regrade.sh <results.json> [questions.json]
#
# Rewrites each result's verdict and reason, the file's score, and its
# questions_version, and prints every verdict that changed. Only the first call's
# answer is retained, so a question that graded UNSTABLE live cannot be regraded
# and is refused (exit 2). Cost and latency fields are untouched: grading never
# changes what an arm read.
set -euo pipefail

file="${1:?usage: regrade.sh <results.json> [questions.json]}"
here="$(cd "$(dirname "$0")" && pwd)"
questions="${2:-$here/questions.json}"
. "$here/grade.sh"

if jq -e '[.results[] | select(.verdict == "UNSTABLE")] | length > 0' "$file" >/dev/null; then
	echo "regrade: refusing $file: an UNSTABLE verdict cannot be regraded from one retained answer" >&2
	exit 2
fi

tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT
printf '[]' > "$tmp"
n=$(jq '.results | length' "$file")
for i in $(seq 0 $((n - 1))); do
	id=$(jq -r ".results[$i].id" "$file")
	q=$(jq -c --arg id "$id" '.questions[] | select(.id == $id)' "$questions")
	[ -n "$q" ] || { echo "regrade: $id is not in $questions" >&2; exit 2; }
	answer=$(jq -r ".results[$i].answer" "$file")
	old=$(jq -r ".results[$i].verdict" "$file")
	grade_answer
	[ "$old" = "$verdict" ] || printf '%s\t%s -> %s\t%s\n' "$id" "$old" "$verdict" "$reason"
	jq --arg v "$verdict" --arg r "$reason" '. += [{verdict: $v, reason: $r}]' "$tmp" > "$tmp.2" && mv "$tmp.2" "$tmp"
done

jq --slurpfile g "$tmp" --argjson qv "$(jq '.version // 0' "$questions")" '
	.results = ([.results, $g[0]] | transpose | map(.[0] + .[1]))
	| .score = ([.results[] | select(.verdict == "correct")] | length)
	| .questions_version = $qv' "$file" > "$tmp.out"
mv "$tmp.out" "$file"
echo "$(jq -r '"\(.label): \(.score)/\(.total)"' "$file")"
