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

echo
if [ "$fails" -eq 0 ]; then
	echo "matcher tests passed"
	exit 0
fi
echo "$fails matcher test(s) failed" >&2
exit 1
