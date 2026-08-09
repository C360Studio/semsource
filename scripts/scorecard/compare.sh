#!/usr/bin/env bash
# Join scorecard result files into the per-band × per-arm comparison table.
#
#   scripts/scorecard/compare.sh results/<a>.json results/<b>.json [...]
#
# Refuses to compare files whose questions_version differs — a score is only
# meaningful against another score taken with the same questions.json, and
# cost figures inherit the same rule (README: Comparability). Files predating
# the version field count as version 0 and only compare with each other.
#
# Recall is correct/total; ctx is total context bytes per band (tokens ~
# bytes/4, an estimate). Schema overhead prints separately per arm — it is
# session-fixed cost, deliberately never folded into per-band figures.
set -euo pipefail
export LC_ALL=C

[ $# -ge 1 ] || { echo "usage: compare.sh <results.json> [...]" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

ver=""
for f in "$@"; do
	[ -f "$f" ] || { echo "no such file: $f" >&2; exit 1; }
	v=$(jq -r '.questions_version // 0' "$f")
	if [ -z "$ver" ]; then ver="$v"
	elif [ "$v" != "$ver" ]; then
		echo "refusing: questions_version mismatch ($ver vs $v in $f) — re-run both sides on one set" >&2
		exit 1
	fi
done
echo "questions_version: $ver"
echo

# One row per band plus TOTAL; one column pair (recall, ctx bytes) per file.
hdr="band"
for f in "$@"; do
	hdr="$hdr\t$(jq -r '"\(.label) (\(.arm // "?"))"' "$f")"
done
{
	printf '%b\n' "$hdr"
	for band in $(jq -r '.results[].band' "$1" | sort -u); do
		row="$band"
		for f in "$@"; do
			row="$row\t$(jq -r --arg b "$band" \
				'[.results[] | select(.band == $b)] |
				 if length == 0 then "-" else
				   "\([.[] | select(.verdict == "correct")] | length)/\(length)  \([.[].context_bytes // 0] | add) B"
				 end' "$f")"
		done
		printf '%b\n' "$row"
	done
	row="TOTAL"
	for f in "$@"; do
		row="$row\t$(jq -r \
			'"\(.score)/\(.total)  \([.results[].context_bytes // 0] | add) B (~\(([.results[].context_bytes // 0] | add) / 4 | floor) tok)"' "$f")"
	done
	printf '%b\n' "$row"
} | column -t -s "$(printf '\t')"

echo
echo "session-fixed schema overhead (excluded from figures above):"
for f in "$@"; do
	jq -r 'if .schema_overhead then
	         "  \(.label): \(.schema_overhead.bytes) B (~\(.schema_overhead.bytes / 4 | floor) tok), \(.schema_overhead.tools) tools"
	       else
	         "  \(.label): none (no tool registration in this arm)"
	       end' "$f"
done

# Non-correct verdicts that outrank a plain miss get called out — folding the
# damaging failures into the innocuous one is exactly what the grader refuses
# to do, so the comparator must not bury them either.
flagged=0
for f in "$@"; do
	bad=$(jq -r '[.results[] | select(.verdict == "UNSTABLE" or .verdict == "MISLEADING" or .verdict == "FABRICATED")] |
	             map("  \(.id) \(.verdict): \(.reason)") | join("\n")' "$f")
	if [ -n "$bad" ]; then
		[ "$flagged" = 0 ] && { echo; echo "!!! flagged verdicts:"; flagged=1; }
		echo "$(jq -r '.label' "$f"):"
		echo "$bad"
	fi
done
exit 0
