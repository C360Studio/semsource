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
python3 "$here/check-composition.py" "$corpus" >/dev/null 2>&1
[ $? -eq 0 ] && ok "composition gate passes on a clean corpus" || fail "composition gate rejected a clean corpus"
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

echo
if [ "$fails" -eq 0 ]; then
	echo "matcher tests passed"
	exit 0
fi
echo "$fails matcher test(s) failed" >&2
exit 1
