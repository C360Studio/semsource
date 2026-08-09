#!/usr/bin/env bash
# Shared grader for every scorecard arm. Sourced, not executed.
#
# Extracted verbatim from run.sh so arm scripts (arm-a-grep.sh, arm-c-cosine.sh)
# grade with the SAME matcher semantics by construction, not by copy. The
# contract: caller sets $q (the question JSON) and $answer (a JSON object
# whose .nodes[].body strings hold the evidence, .nodes[0] being the top
# node); grade_answer sets $verdict, $reason, $nodes, $bodybytes, $topbytes.
# Changing this file changes every arm at once — which is the point.

# Grades one answer for one question. Reads $q and $answer; sets $verdict,
# $reason and the evidence metrics. Extracted from the loop so a question can be
# asked more than once and the verdicts compared — see the repeat logic below.
#
# Always returns 0: the body ends in conditionals whose natural exit status is
# "no match", which under `set -e` would abort the run rather than record a miss.
grade_answer() {
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
			printf '%s' "$lower" | grep -qF -- "$w" || { verdict="miss"; reason="missing required: $w"; break; }
		done
	fi
	if [ "$verdict" = "correct" ] && [ "$(jq '(.expect_any // []) | length' <<<"$q")" -gt 0 ]; then
		hit=0
		for want in $(jq -r '(.expect_any // [])[]' <<<"$q" | tr ' ' '\001'); do
			w=$(printf '%s' "$want" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$lower" | grep -qF -- "$w" && { hit=1; break; }
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
			printf '%s' "$toplower" | grep -qF -- "$w" || { top_all_hit=0; top_missing="$w"; break; }
		done
		for bad in $(jq -r '(.expect_top_none // [])[]' <<<"$q" | tr ' ' '\001'); do
			b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
			printf '%s' "$toplower" | grep -qF -- "$b" && { top_none_hit=1; top_carried="$b"; break; }
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
	for bad in $(jq -r '(.expect_none // [])[]' <<<"$q" | tr ' ' '\001'); do
		b=$(printf '%s' "$bad" | tr '\001' ' ' | tr '[:upper:]' '[:lower:]')
		printf '%s' "$lower" | grep -qF -- "$b" && { verdict="FABRICATED"; reason="asserted: $b"; break; }
	done
	return 0
}
