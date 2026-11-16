#!/usr/bin/env bash
# Shared grader for every scorecard arm. Sourced, not executed.
#
# Extracted verbatim from run.sh so arm scripts (arm-a-grep.sh, arm-c-cosine.sh)
# grade with the SAME matcher semantics by construction, not by copy. The
# contract: caller sets $q (the question JSON) and $answer (a JSON object:
# fusion .nodes[] or graph_search .matches[], .nodes[0] being the top node, or
# {isError, error} for a failed call); grade_answer matches only the content
# answer_content extracts and sets $verdict, $reason, $nodes, $bodybytes,
# $topbytes.
# Changing this file changes every arm at once — which is the point.

# Grades one answer for one question. Reads $q and $answer; sets $verdict,
# $reason and the evidence metrics. Extracted from the loop so a question can be
# asked more than once and the verdicts compared — see the repeat logic below.
#
# Always returns 0: the body ends in conditionals whose natural exit status is
# "no match", which under `set -e` would abort the run rather than record a miss.
# answer_content prints the evidence an answer carries and nothing else: each
# node's name and body and the names of its related symbols (fusion tools), and
# each graph match's label and property values (graph_search). Keys, paths,
# handles, fragments and envelope blocks are never content. Matching the whole
# answer JSON passed questions on vacuous hits (#269): L05's "ast" only inside
# the key "last_synced", D09's "index" in the envelope's index block, I02's
# "doc" in the anchor's own path. An answer that is not JSON has no content.
answer_content() {
	printf '%s' "$answer" | jq -r '
		[ ((.nodes // [])[] | (.name // ""), (.body // ""),
		    ((.relations // {}) | to_entries[] | .value[]? | (.name // ""))),
		  ((.matches // [])[] | (.label // ""), ((.properties // {}) | .[] | tostring)) ]
		| map(select(type == "string" and . != "")) | join("\n")' 2>/dev/null || true
}

# relation_files prints the file stem (basename without extension) of every
# related symbol an answer returns. A question reads them only when it sets
# "grade_relation_files": true. In Java the class is the file, so "the closure
# reaches SystemDataProvider" lives only in a caller's path (OSH I01/P02). It is
# opt-in because paths are not content for any other question (#269), and it
# never reads the answer's own node paths, which carried the vacuous I02 hit.
relation_files() {
	printf '%s' "$answer" | jq -r '
		[ (.nodes // [])[] | (.relations // {}) | to_entries[] | .value[]? | (.path // "")
		  | select(type == "string" and . != "") | split("/") | last | sub("\\.[^.]*$"; "") ]
		| unique | join("\n")' 2>/dev/null || true
}

grade_answer() {
	lower=$(answer_content | tr '[:upper:]' '[:lower:]')
	if [ "$(jq -r '.grade_relation_files == true' <<<"$q")" = true ]; then
		lower=$(printf '%s\n%s' "$lower" "$(relation_files | tr '[:upper:]' '[:lower:]')")
	fi

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
	# Literals are matched with here-strings, never `printf | grep -q`: grep -q
	# exits at the first hit, printf then dies of SIGPIPE, and under pipefail
	# the pipeline reports failure, so a hit early in a large answer graded as
	# a miss (found regrading the #230 baseline).
	# isError is a failed call, never a graded answer: a refusal is not a miss,
	# and on a negative-band question it must not pass as a correct "nothing".
	# The callers record gateway, transport and non-JSON results as isError.
	if [ "$(printf '%s' "$answer" | jq -r '.isError == true' 2>/dev/null)" != "false" ]; then
		verdict="error"; reason="tool error: $(printf '%s' "$answer" | jq -r '.error // "unreadable answer"' 2>/dev/null | head -c 200)"
	fi
	if [ "$verdict" = "correct" ]; then
		for want in $(jq -r '(.expect_all // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			grep -qF -- "$w" <<<"$lower" || { verdict="miss"; reason="missing required: $w"; break; }
		done
	fi
	if [ "$verdict" = "correct" ] && [ "$(jq '(.expect_any // []) | length' <<<"$q")" -gt 0 ]; then
		hit=0
		for want in $(jq -r '(.expect_any // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			grep -qF -- "$w" <<<"$lower" && { hit=1; break; }
		done
		[ "$hit" = 1 ] || { verdict="miss"; reason="none of expect_any present"; }
	fi
	# Discrimination questions grade the TOP node alone, not the union of every
	# node returned. Grading the union cannot separate the two systems: an answer
	# carries up to 20 passages, so a confusable value elsewhere in the same
	# document rides along even when retrieval ranked the right passage first.
	# The claim being tested is narrower and is the one that matters to an agent:
	# the single best piece of evidence answers the question on its own.
	#
	# The answer side and the confusable side are evaluated INDEPENDENTLY, then
	# combined. Short-circuiting to "miss" on the answer side (as this did until
	# the v3 grader) made one of the three states unreachable: a top node holding
	# the twin but NOT the answer broke out as a plain miss before the confusable
	# check ever ran, so the most misleading outcome was scored as the most
	# innocuous one.
	if [ "$verdict" = "correct" ]; then
		top_all_hit=1; top_none_hit=0; top_missing=""; top_carried=""
		for want in $(jq -r '(.expect_top_all // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			grep -qF -- "$w" <<<"$toplower" || { top_all_hit=0; top_missing="$w"; break; }
		done
		for bad in $(jq -r '(.expect_top_none // [])[]' <<<"$q" | tr ' ' '\001'); do
			b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			grep -qF -- "$b" <<<"$toplower" && { top_none_hit=1; top_carried="$b"; break; }
		done
		# Four states, four verdicts:
		#
		#   answer + no twin  -> correct     the evidence settles the question
		#   answer + twin     -> IMPRECISE   carries both; cannot settle it
		#   twin, no answer   -> MISLEADING  argues for the WRONG answer
		#   neither           -> miss        returns nothing useful
		#
		# MISLEADING is separated from miss for the same reason IMPRECISE is
		# separated from FABRICATED: they are different failures, and folding the
		# damaging one into the innocuous one hides exactly what matters. A miss
		# tells a caller nothing; a MISLEADING top node tells it something false,
		# and an agent citing the first result will state it as fact.
		if [ "$top_all_hit" = 1 ] && [ "$top_none_hit" = 1 ]; then
			verdict="IMPRECISE"; reason="top node also carries the confusable: $top_carried"
		elif [ "$top_all_hit" = 0 ] && [ "$top_none_hit" = 1 ]; then
			verdict="MISLEADING"; reason="top node carries the confusable ($top_carried) but not the answer ($top_missing)"
		elif [ "$top_all_hit" = 0 ]; then
			verdict="miss"; reason="top node missing: $top_missing"
		fi
	fi
	# A fabrication is graded separately from a miss: they are different failures
	# and conflating them hides the one that actually matters.
	[ "$verdict" = "error" ] && return 0
	for bad in $(jq -r '(.expect_none // [])[]' <<<"$q" | tr ' ' '\001'); do
		b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
		grep -qF -- "$b" <<<"$lower" && { verdict="FABRICATED"; reason="asserted: $b"; break; }
	done
	return 0
}
