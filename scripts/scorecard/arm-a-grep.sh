#!/usr/bin/env bash
# Arm A of the retrieval cost comparison: a deterministic grep-and-read
# baseline over the corpus checkout. No stack, no index, no infrastructure —
# the floor any retrieval system must beat to justify existing.
#
#   scripts/scorecard/arm-a-grep.sh <corpus-dir> <label>
#
# Procedure (design D1 of scorecard-token-cost-arms), per question:
#   1. lowercase args.query, split on non-alphanumerics, drop stopwords.txt
#      entries — the remaining terms are the search terms (no stemming, no
#      synonyms: deterministic and inspectable)
#   2. grep -ril each term; rank files by distinct-terms-matched, then total
#      matching lines, then path — deterministic on any box
#   3. read files in rank order until every coverable term has appeared in a
#      read file, or the file cap (SCORECARD_ARM_A_FILE_CAP, default 5) is
#      exhausted; charge the FULL bytes of every file opened
#   4. grade the read set with the shared grader (grade.sh) — the top-ranked
#      file plays the role of the top node for discrimination bands
#
# Blindness: retrieval consumes ONLY args.query. The expect_* fields are read
# exclusively by grade_answer, after the read set is fixed. This is the
# fairness property that makes the cross-arm comparison publishable.
#
# Whole-file charging matches how agents actually read, and its bias runs
# AGAINST the arm this baseline competes with (it inflates grep's cost);
# per-file bytes are recorded so a match-window variant can be derived from
# stored results without re-running. This arm is a floor, not a simulation:
# real agentic grep iterates and re-greps, costing more on hard questions.
#
# No repeats: the procedure is a pure function of the corpus, unlike the live
# stack whose instability the repeat logic in run.sh exists to detect.
set -euo pipefail
export LC_ALL=C

corpus="${1:?usage: arm-a-grep.sh <corpus-dir> <label>}"
label="${2:?usage: arm-a-grep.sh <corpus-dir> <label>}"
here="$(cd "$(dirname "$0")" && pwd)"
corpus="$(cd "$corpus" && pwd)"
questions="${SCORECARD_QUESTIONS:-$here/questions.json}"
cap="${SCORECARD_ARM_A_FILE_CAP:-5}"
outdir="$here/results"
mkdir -p "$outdir"
out="$outdir/${label}.json"

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
[ -f "$here/stopwords.txt" ] || { echo "stopwords.txt missing" >&2; exit 1; }

. "$here/grade.sh"

# Mirrors the ingestion corpus rules (basename-matched): never read VCS
# internals, dependency trees, or the scorecard itself (which contains the
# expected answers — reading it would break the blindness property).
grep_hits() { # $1 = term; emits "path<TAB>count" for files with >0 matches
	grep -r -i -c -I \
		--exclude-dir=.git --exclude-dir=node_modules --exclude-dir=scorecard \
		-e "$1" "$corpus" 2>/dev/null \
		| awk -F: -v OFS='\t' 'NF>=2 && $NF+0 > 0 { c=$NF; NF--; print $0, c }' \
		|| true
}

terms_for() { # $1 = query text; one term per line, first-occurrence order
	printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '\n' \
		| sed '/^$/d' | grep -vxFf "$here/stopwords.txt" | awk '!seen[$0]++' || true
}

echo "[]" > "$out.tmp"
total=0; correct=0
n=$(jq '.questions | length' "$questions")
for i in $(seq 0 $((n - 1))); do
	q=$(jq ".questions[$i]" "$questions")
	id=$(jq -r '.id' <<<"$q")
	band=$(jq -r '.band' <<<"$q")
	query=$(jq -r '.args.query' <<<"$q")

	hits=$(mktemp)
	terms=$(terms_for "$query")
	nterms=0
	while IFS= read -r t; do
		[ -n "$t" ] || continue
		nterms=$((nterms + 1))
		grep_hits "$t" | awk -F'\t' -v OFS='\t' -v t="$t" '{print $1, $2, t}' >> "$hits"
	done <<<"$terms"

	# Rank: distinct terms matched desc, total matching lines desc, path asc.
	ranked=$(awk -F'\t' '{ d[$1 "\t" $3]=1; tot[$1]+=$2 }
		END { for (k in d) { split(k, a, "\t"); dis[a[1]]++ }
		      for (f in tot) printf "%d\t%d\t%s\n", dis[f], tot[f], f }' "$hits" \
		| sort -t"$(printf '\t')" -k1,1nr -k2,2nr -k3,3)

	# Coverable terms: those grep found anywhere at all. A term with zero hits
	# can never be covered, so it must not force reading to the cap.
	coverable=$(awk -F'\t' '{print $3}' "$hits" | sort -u)
	ncoverable=$(printf '%s' "$coverable" | grep -c . || true)

	# Read files in rank order until every coverable term is covered or the
	# cap is hit. Coverage comes from the hits table, not a re-scan.
	files_read=""
	nread=0
	covered=$(mktemp)
	: > "$covered"
	while IFS= read -r line; do
		[ -n "$line" ] || continue
		[ "$nread" -lt "$cap" ] || break
		[ "$(sort -u "$covered" | grep -c . || true)" -lt "$ncoverable" ] || break
		f=$(printf '%s' "$line" | cut -f3)
		files_read="$files_read$f
"
		nread=$((nread + 1))
		awk -F'\t' -v f="$f" '$1==f {print $3}' "$hits" >> "$covered"
	done <<<"$ranked"

	# The read set becomes the answer: nodes in rank order, body = file text,
	# so the shared grader applies unchanged (top-ranked file = top node).
	nodes_json="[]"
	context_bytes=0
	files_json="[]"
	while IFS= read -r f; do
		[ -n "$f" ] || continue
		fb=$(wc -c < "$f" | tr -d ' ')
		context_bytes=$((context_bytes + fb))
		rel="${f#"$corpus"/}"
		nodes_json=$(jq --argjson cur "$nodes_json" -Rs --arg p "$rel" \
			'$cur + [{path:$p, body:.}]' < "$f")
		files_json=$(jq -n --argjson cur "$files_json" --arg p "$rel" --argjson b "$fb" \
			'$cur + [{path:$p, bytes:$b}]')
	done <<<"$files_read"
	answer=$(jq -nc --argjson nodes "$nodes_json" '{nodes:$nodes}')

	grade_answer

	total=$((total + 1))
	[ "$verdict" = "correct" ] && correct=$((correct + 1))
	printf '%-5s %-14s %-10s files=%-3s ctx=%-8s %s\n' \
		"$id" "$band" "$verdict" "$nread" "$context_bytes" "$reason"

	jq --arg id "$id" --arg band "$band" --arg v "$verdict" --arg r "$reason" \
	   --argjson n "${nodes:-0}" --argjson bb "${bodybytes:-0}" \
	   --argjson tb "${topbytes:-0}" --argjson cb "$context_bytes" \
	   --argjson files "$files_json" \
	   --arg a "$(printf '%s' "$answer" | head -c 6000)" \
	   '. += [{id:$id, band:$band, tool:"grep", verdict:$v, reason:$r, nodes:$n,
	           body_bytes:$bb, top_body_bytes:$tb, context_bytes:$cb,
	           files:$files, answer:$a}]' \
	   "$out.tmp" > "$out.tmp2" && mv "$out.tmp2" "$out.tmp"
	rm -f "$hits" "$covered"
done

qver=$(jq -r '.version // 0' "$questions")
jq -n --arg label "$label" --argjson score "$correct" --argjson total "$total" \
   --argjson qver "${qver:-0}" --argjson cap "$cap" --slurpfile r "$out.tmp" \
   '{label:$label, arm:"A", questions_version:$qver, score:$score, total:$total,
     file_cap:$cap, results:$r[0]}' > "$out"
rm -f "$out.tmp"

echo
echo "=== $label (arm A, grep baseline): $correct/$total ==="
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): \([.[] | select(.verdict=="correct")] | length)/\(length)"' "$out"
echo
echo "--- context cost (whole files read; tokens ~ bytes/4, estimate) ---"
jq -r '.results | group_by(.band)[] |
       "\(.[0].band): total \([.[].context_bytes // 0] | add) B (~\(([.[].context_bytes // 0] | add) / 4 | floor) tok), mean \(([.[].context_bytes // 0] | add) / length | floor) B"' "$out"
jq -r '"all bands: total \([.results[].context_bytes // 0] | add) B (~\(([.results[].context_bytes // 0] | add) / 4 | floor) tok)"' "$out"
echo "written: $out"
