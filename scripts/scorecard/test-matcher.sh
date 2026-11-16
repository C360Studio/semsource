#!/usr/bin/env bash
# Regression test for the scorecard grader's matcher.
#
# This exists because of a defect no product test could have caught: the grader
# called `grep -qF "$w"` with no `--`, so a literal beginning with `-` (X02's
# `-p 8083:8083`) was parsed as options. grep exited 2, the loop read any
# non-zero exit as "not found", and the question graded `miss` on every system
# forever while retrieval was in fact correct.
#
# Needs no stack: it tests the matcher and the source, not retrieval.
#
#   scripts/scorecard/test-matcher.sh
set -uo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
fails=0

ok()   { printf '  ok   %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1"; fails=$((fails + 1)); }

echo "1. a literal beginning with '-' is matched as content, not parsed as options"
if printf 'docker run -d -p 8083:8083 img\n' | grep -qF -- "-p 8083:8083"; then
	ok "leading-dash literal matches when present"
else
	fail "leading-dash literal did not match when present"
fi
if printf 'docker run -d -p 8081:8081 img\n' | grep -qF -- "-p 8083:8083"; then
	fail "leading-dash literal matched when absent"
else
	ok "leading-dash literal does not match when absent"
fi

echo "2. the grader's own invocations all terminate option parsing"
# Guards the source directly: a future edit that drops `--` reintroduces a bug
# whose only symptom is a permanently wrong verdict. The grader lives in
# grade.sh (shared by every arm); run.sh stays covered for any matcher use
# outside the shared function.
for src in grade.sh run.sh arm-a-grep.sh; do
	[ -f "$here/$src" ] || continue
	if grep -nE 'grep -qF "\$' "$here/$src" >/dev/null 2>&1; then
		fail "$src has a 'grep -qF \"\$...\"' without '--':"
		grep -nE 'grep -qF "\$' "$here/$src" | sed 's/^/       /'
	else
		ok "no un-terminated 'grep -qF \"\$...\"' in $src"
	fi
done

echo "3. the evaluability gate catches the bug it exists for"
corpus="$(mktemp -d)"
printf 'placeholder\n' > "$corpus/README.md"
python3 "$here/check-discrimination.py" "$corpus" >/dev/null 2>&1
[ $? -eq 0 ] && ok "gate passes with the current matcher" || fail "gate rejected the current matcher"
python3 "$here/check-discrimination.py" "$corpus" --simulate-unterminated >/dev/null 2>&1
[ $? -eq 1 ] && ok "gate fails under the pre-v3 matcher" || fail "gate did NOT fail under the pre-v3 matcher"
rm -rf "$corpus"

echo "4. the composition gate fires on co-location and stays quiet past the window"
# The gate's whole value is refusing questions whose facts one passage can
# carry. Prove both directions against synthetic corpora: --simulate plants a
# one-window doc (must FAIL), real separation must pass, and the doc-window /
# whole-code-file asymmetry must hold — two facts far apart in one .go file
# still FAIL because a single body could span them.
corpus="$(mktemp -d)"
printf 'placeholder\n' > "$corpus/README.md"
# The real question set's facts are absent from this placeholder corpus, so the
# gate must refuse it — an absent fact is unanswerable, not "clean" (#269).
python3 "$here/check-composition.py" "$corpus" >/dev/null 2>&1
[ $? -eq 1 ] && ok "composition gate refuses a corpus missing the questions' facts" || fail "composition gate passed a corpus missing every fact"
python3 "$here/check-composition.py" "$corpus" --simulate >/dev/null 2>&1
[ $? -eq 1 ] && ok "composition gate fires under --simulate" || fail "composition gate did NOT fire under --simulate"

qfile="$corpus/questions-synth.json"
cat > "$qfile" <<'JSON'
{"version": 0, "questions": [{"id": "S1", "band": "composition",
  "expect_all": ["synthetic-fact-alpha", "synthetic-fact-beta"]}]}
JSON
# Both facts inside one doc window -> FAIL.
printf 'synthetic-fact-alpha and synthetic-fact-beta together\n' > "$corpus/close.md"
python3 "$here/check-composition.py" "$corpus" "$qfile" >/dev/null 2>&1
[ $? -eq 1 ] && ok "co-located doc facts FAIL" || fail "co-located doc facts passed"
# Facts pushed past the 6000 B doc window -> pass.
{ printf 'synthetic-fact-alpha\n'; head -c 7000 /dev/zero | tr '\0' 'x'; printf '\nsynthetic-fact-beta\n'; } > "$corpus/close.md"
python3 "$here/check-composition.py" "$corpus" "$qfile" >/dev/null 2>&1
[ $? -eq 0 ] && ok "doc facts past the window pass" || fail "doc facts past the window failed"
# Same separation in a CODE file -> still FAIL (whole-file window).
mv "$corpus/close.md" "$corpus/close.go"
python3 "$here/check-composition.py" "$corpus" "$qfile" >/dev/null 2>&1
[ $? -eq 1 ] && ok "code-file facts FAIL regardless of distance" || fail "code-file facts passed on distance"
# A single-fact composition question is not compositional -> FAIL.
rm -f "$corpus/close.go"
cat > "$qfile" <<'JSON'
{"version": 0, "questions": [{"id": "S2", "band": "composition",
  "expect_all": ["only-one-fact"]}]}
JSON
python3 "$here/check-composition.py" "$corpus" "$qfile" >/dev/null 2>&1
[ $? -eq 1 ] && ok "single-fact composition question FAILS" || fail "single-fact composition question passed"
rm -rf "$corpus"

echo "5. the grader matches answer content, never keys, paths, or envelope blocks (#269)"
. "$here/grade.sh"
grade() { # $1 = question JSON, $2 = answer JSON; prints the verdict
	q="$1"; answer="$2"; grade_answer; printf '%s' "$verdict"
}
# L05: "ast" only inside the key "last_synced".
v=$(grade '{"expect_all":["ast"]}' '{"index":{"last_synced":"t"},"nodes":[{"name":"Docker Compose","body":"C360_PORT=3000"}]}')
[ "$v" = "miss" ] && ok "a fact present only in a key does not pass (L05)" || fail "key-only 'ast' graded $v"
# D09: "index" only in the envelope's index block.
v=$(grade '{"expect_any":["index"]}' '{"index":{"ready":true},"nodes":[{"name":"x","body":"unrelated"}]}')
[ "$v" = "miss" ] && ok "a fact present only in the envelope block does not pass (D09)" || fail "envelope-only 'index' graded $v"
# I02: "doc" only in the anchor's own path.
v=$(grade '{"expect_any":["doc"]}' '{"nodes":[{"name":"splitPassages","path":"handler/doc/splitter.go","handle":"a.b.doc.c"}]}')
[ "$v" = "miss" ] && ok "a fact present only in a path or handle does not pass (I02)" || fail "path-only 'doc' graded $v"
# Content still passes: body, node name, related symbol name, graph match.
v=$(grade '{"expect_all":["ast"]}' '{"nodes":[{"name":"Source Types","body":"ast, docs, config"}]}')
[ "$v" = "correct" ] && ok "a fact in a node body passes" || fail "body fact graded $v"
v=$(grade '{"expect_any":["ingestFileEntityStates"]}' '{"nodes":[{"name":"splitPassages","relations":{"caller":[{"name":"ingestFileEntityStates","path":"handler/doc/entities.go"}]}}]}')
[ "$v" = "correct" ] && ok "a related symbol name passes" || fail "relation name graded $v"
v=$(grade '{"expect_all":["32.1.3-jre"]}' '{"matches":[{"id":"a.b.c.d.dependency.x","label":"guava","properties":{"source.config.dependency.version":"32.1.3-jre"}}]}')
[ "$v" = "correct" ] && ok "a graph match property value passes" || fail "graph match property graded $v"

# P02 is answerable from what code_impact returns: the seed's body plus its
# direct callers' names, never a caller's body (the shape of the audit frames).
p02=$(jq -c '.questions[] | select(.id == "P02")' "$here/questions.json")
seed=$(jq -nc --arg q "$(jq -r '.args.query' <<<"$p02")" '{name: $q, path: "graph/lifecycle.go",
	body: "func LifecycleOwner(instance, root string) string {\n\tsum := sha256.Sum256([]byte(root))\n\treturn instance + \"/\" + hex.EncodeToString(sum[:6])\n}"}')
v=$(grade "$p02" "$(jq -c '{nodes: [. + {relations: {caller: [{name: "initializeWatchers"}, {name: "ownerFor"}, {name: "owner"}]}}]}' <<<"$seed")")
[ "$v" = "correct" ] && ok "P02 passes on the seed body plus direct caller names" || fail "P02 on a correct code_impact shape graded $v"
v=$(grade "$p02" "$(jq -c '{nodes: [.]}' <<<"$seed")")
[ "$v" = "miss" ] && ok "P02 misses on the seed body alone" || fail "P02 on the seed alone graded $v"

# Relation file stems count only for a question that opts in (OSH I01/P02:
# in Java the class is the file), and never the answer's own node path.
rel='{"nodes":[{"name":"checkDataStreamKey","path":"lib/DataStoreUtils.java","relations":{"caller":[{"name":"checkKey","path":"impl/InMemoryDataStreamStore.java"}]}}]}'
v=$(grade '{"expect_any":["InMemoryDataStreamStore"],"grade_relation_files":true}' "$rel")
[ "$v" = "correct" ] && ok "an opted-in question matches a related symbol's file stem" || fail "opted-in relation file graded $v"
v=$(grade '{"expect_any":["InMemoryDataStreamStore"]}' "$rel")
[ "$v" = "miss" ] && ok "relation file stems are not content without the opt-in" || fail "relation file without opt-in graded $v"
v=$(grade '{"expect_any":["DataStoreUtils"],"grade_relation_files":true}' "$rel")
[ "$v" = "miss" ] && ok "the opt-in never reads the answer's own node path" || fail "own node path graded $v"
v=$(grade '{"expect_any":["impl"],"grade_relation_files":true}' "$rel")
[ "$v" = "miss" ] && ok "the opt-in reads the file stem, not the directory" || fail "relation directory graded $v"

# A hit at the start of a large answer must pass under pipefail (this file
# runs with pipefail, like every arm).
# The hit sits on the first line of 2 MB of later lines, so grep -q stops
# reading long before its writer finishes.
big=$({ printf 'needle-at-start\n'; head -c 2000000 /dev/zero | tr '\0' 'x' | fold -w 100; } \
	| jq -Rsc '{nodes:[{name:"n",body:.}]}')
v=$(grade '{"expect_all":["needle-at-start"]}' "$big")
[ "$v" = "correct" ] && ok "an early hit in a large answer passes under pipefail" || fail "early hit in a large answer graded $v"

echo "6. gateway, transport, and non-JSON failures grade error, never miss or correct (#269)"
v=$(grade '{"expect_any":["sources"]}' '{"isError":true,"error":"fusion query failed: semantic is not enabled"}')
[ "$v" = "error" ] && ok "a gateway refusal grades error" || fail "gateway refusal graded $v"
# N02 passed on a refusal: a negative-band question must not read an error as "nothing found".
v=$(grade '{"expect_none":["quantum entanglement subsystem is"]}' '{"isError":true,"error":"refused"}')
[ "$v" = "error" ] && ok "a refusal on a negative question grades error, not correct" || fail "negative refusal graded $v"
v=$(grade '{"expect_any":["x"]}' 'fusion query failed: plain text')
[ "$v" = "error" ] && ok "a non-JSON answer grades error" || fail "non-JSON answer graded $v"
v=$(grade '{"expect_none":["anything"]}' '{"nodes":[]}')
[ "$v" = "correct" ] && ok "an honest empty answer still passes a negative question" || fail "empty negative graded $v"

echo "7. the composition gate fails on a fact the corpus does not contain (#269)"
corpus="$(mktemp -d)"
qfile="$corpus/questions-synth.json"
cat > "$qfile" <<'JSON'
{"version": 0, "questions": [{"id": "S3", "band": "composition",
  "expect_all": ["synthetic-fact-alpha", "synthetic-fact-gone"]}]}
JSON
printf 'synthetic-fact-alpha\n' > "$corpus/a.md"
python3 "$here/check-composition.py" "$corpus" "$qfile" >/dev/null 2>&1
[ $? -eq 1 ] && ok "an absent fact FAILS the gate" || fail "an absent fact passed the gate"
rm -rf "$corpus"

echo "8. the R1 rank diff ranks valid evidence and refuses the rest"
rd="$(mktemp -d)"
res() { # $1 = file, rest = "id|answer-json" pairs
	f="$1"; shift
	printf '%s\n' "$@" | jq -Rn '{results: [inputs | split("|") | {id: .[0], answer: .[1]}]}' > "$f"
}
n1='{"nodes":[{"handle":"a"},{"handle":"b"}]}'
n2='{"nodes":[{"handle":"b"},{"handle":"a"}]}'
res "$rd/ok.json" "Q1|$n1" 'Q2|{"matches":[{"id":"m1"}]}' 'Q3|{"nodes":[]}' 'Q4|{"misses":[{"query":"x"}]}'
res "$rd/swap.json" "Q1|$n2" 'Q2|{"matches":[{"id":"m1"}]}' 'Q3|{"nodes":[]}' 'Q4|{"misses":[{"query":"x"}]}'
rdiff() { "$here/rank-diff.sh" "$1" "$2" >/dev/null 2>&1; echo $?; }
[ "$(rdiff "$rd/ok.json" "$rd/ok.json")" = 0 ] && ok "identical rankings, including no-hit answers, are RANK-STABLE" || fail "identical rankings did not pass"
[ "$(rdiff "$rd/ok.json" "$rd/swap.json")" = 1 ] && ok "a reordering is RANK-UNSTABLE" || fail "a reordering passed"
res "$rd/err.json" 'Q1|{"isError":true,"error":"down"}'
[ "$(rdiff "$rd/err.json" "$rd/err.json")" = 2 ] && ok "an error answer is refused" || fail "an error answer was ranked"
res "$rd/bad.json" 'Q1|not JSON'
[ "$(rdiff "$rd/bad.json" "$rd/bad.json")" = 2 ] && ok "a non-JSON answer is refused" || fail "a non-JSON answer was ranked"
res "$rd/shape.json" 'Q1|{"index":{}}'
[ "$(rdiff "$rd/shape.json" "$rd/shape.json")" = 2 ] && ok "an answer with neither nodes nor matches is refused" || fail "a shapeless answer was ranked"
echo '{"results":[]}' > "$rd/empty.json"
[ "$(rdiff "$rd/empty.json" "$rd/empty.json")" = 2 ] && ok "an empty result set is refused" || fail "an empty result set passed"
res "$rd/dup.json" "Q1|$n1" "Q1|$n1"
[ "$(rdiff "$rd/dup.json" "$rd/dup.json")" = 2 ] && ok "duplicate question ids are refused" || fail "duplicate question ids passed"
res "$rd/less.json" "Q1|$n1" 'Q2|{"matches":[{"id":"m1"}]}' 'Q3|{"nodes":[]}'
[ "$(rdiff "$rd/less.json" "$rd/ok.json")" = 2 ] && ok "a question only in the second file is refused" || fail "an added question passed"
[ "$(rdiff "$rd/ok.json" "$rd/less.json")" = 2 ] && ok "a question missing from the second file is refused" || fail "a removed question passed"
res "$rd/ids.json" 'Q1|{"nodes":[{"id":"x","body":"1"},{"id":"y","body":"2"}]}'
res "$rd/ids-rev.json" 'Q1|{"nodes":[{"id":"y","body":"2"},{"id":"x","body":"1"}]}'
[ "$(rdiff "$rd/ids.json" "$rd/ids-rev.json")" = 1 ] && ok "an id-only (arm C) reordering is RANK-UNSTABLE" || fail "an id-only reordering passed"
res "$rd/anon.json" 'Q1|{"nodes":[{"body":"1"},{"body":"2"}]}'
[ "$(rdiff "$rd/anon.json" "$rd/anon.json")" = 2 ] && ok "anonymous nodes are refused" || fail "anonymous nodes were ranked"
res "$rd/anonm.json" 'Q1|{"matches":[{"label":"x"}]}'
[ "$(rdiff "$rd/anonm.json" "$rd/anonm.json")" = 2 ] && ok "matches without an id are refused" || fail "anonymous matches were ranked"
rm -rf "$rd"

echo
if [ "$fails" -eq 0 ]; then
	echo "matcher tests passed"
	exit 0
fi
echo "$fails matcher test(s) failed" >&2
exit 1
