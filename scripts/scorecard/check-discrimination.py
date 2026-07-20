#!/usr/bin/env python3
"""Validate every discrimination question against the corpus it will be scored on.

A discrimination question asserts that the top-ranked passage carries the answer
and NOT its confusable twin. That only measures retrieval if the two literals
actually live far enough apart to land in different passages. When they do not,
the question reports IMPRECISE on every system for a reason that has nothing to
do with retrieval quality, and a real regression hides behind the constant
failure.

Two ways that goes wrong, both observed on this corpus:

  * Real docs co-locate the pair. A candidate pairing the ui-dev overlay against
    the released ghcr image looked clean in README.md, but ROADMAP.md mentions
    both TWO LINES apart. Same for a SemStreams version pair — beta.153 and
    beta.144 sit two lines apart in docs/testing/readme-surface-coverage.md.
    Both were dropped by this check after passing a careful read-through.

  * The scorecard describes itself. questions.json and this directory's README
    quote both literals of every question, side by side. Ingesting them puts a
    guaranteed-IMPRECISE passage in the corpus — the measuring apparatus
    corrupting the measurement. scripts/scorecard/ is therefore excluded here
    and must be excluded from the ingested corpus too.

Usage:
    scripts/scorecard/check-discrimination.py <corpus-dir> [questions.json]

Exits non-zero if any question is unsafe, so it can gate a run.
"""

import json
import os
import subprocess
import sys

# Passages are byte-bounded, so line distance is a proxy — but a coarse one is
# enough here. Under 5 lines nothing can separate them; under 25 only an
# aggressively small ceiling would, which is exactly the knob being tuned, so
# such a question would measure the tuning instead of the retrieval.
FATAL_LINES = 5
RISKY_LINES = 25

INGESTED_SUFFIXES = (".md", ".mdx", ".txt")
EXCLUDED_DIRS = ("scripts/scorecard",)


def ingested_files(corpus):
    for root, _dirs, files in os.walk(corpus):
        rel_root = os.path.relpath(root, corpus)
        if any(rel_root.startswith(d) for d in EXCLUDED_DIRS):
            continue
        for name in files:
            if name.endswith(INGESTED_SUFFIXES):
                yield os.path.join(root, name)


def closest_cooccurrence(corpus, good, bad):
    """Return (relpath, line_distance) for the tightest co-occurrence, or None."""
    worst = None
    for path in ingested_files(corpus):
        try:
            lines = open(path, encoding="utf-8", errors="ignore").read().splitlines()
        except OSError:
            continue
        good_at = [i for i, line in enumerate(lines) if good in line]
        bad_at = [i for i, line in enumerate(lines) if bad in line]
        if not good_at or not bad_at:
            continue
        distance = min(abs(g - b) for g in good_at for b in bad_at)
        if worst is None or distance < worst[1]:
            worst = (os.path.relpath(path, corpus), distance)
    return worst


MATCHER_FIELDS = ("expect_all", "expect_any", "expect_none",
                  "expect_top_all", "expect_top_none")


def matcher_argv(literal, terminated=True):
    """The grep invocation run.sh grades with, for one literal.

    Kept here deliberately rather than described in prose: this gate is only
    meaningful if it exercises the SAME matcher the grader does.
    """
    return ["grep", "-qF", "--", literal] if terminated else ["grep", "-qF", literal]


def evaluable(literal, terminated=True):
    """Can the grader actually evaluate this literal?

    Behavioural, not syntactic. Feed the literal through the real matcher against
    a string that contains it and one that does not, and require the two to
    disagree. A syntactic rule (say, "reject literals starting with -") would
    only enumerate the footgun already known; this catches the next one too.

    Returns (ok, detail).
    """
    hit = "xx" + literal + "xx"
    miss = "nothing to see here"
    argv = matcher_argv(literal, terminated)
    try:
        r_hit = subprocess.run(argv, input=hit, text=True, capture_output=True)
        r_miss = subprocess.run(argv, input=miss, text=True, capture_output=True)
    except OSError as exc:  # pragma: no cover - grep missing is an environment fault
        return False, f"could not run matcher: {exc}"
    # 0 = matched, 1 = no match. Anything else is the matcher failing to run,
    # which the grader records as "not found" — a silent, permanent miss.
    if r_hit.returncode == 0 and r_miss.returncode == 1:
        return True, ""
    if r_hit.returncode > 1 or r_miss.returncode > 1:
        err = (r_hit.stderr or r_miss.stderr).strip().splitlines()
        return False, f"matcher errored (exit {max(r_hit.returncode, r_miss.returncode)}): {err[0] if err else 'no stderr'}"
    return False, (f"matcher cannot distinguish present from absent "
                   f"(hit exit {r_hit.returncode}, miss exit {r_miss.returncode})")


def check_evaluable(questions, terminated=True):
    """Gate every literal in every question, not just the discrimination band.

    X02 shipped for months grading `miss` on every system because its expected
    value began with `-p` and grep parsed it as options. The corpus checks below
    could never have caught that: they validate the relationship between a pair
    of literals in the text, never that the grader can match them at all.
    """
    failed = False
    for q in questions:
        for field in MATCHER_FIELDS:
            for literal in q.get(field, []):
                ok, detail = evaluable(literal.lower(), terminated)
                if not ok:
                    print(f"{q['id']}  FATAL     {field} literal {literal!r} "
                          f"is not evaluable by the grader: {detail}")
                    failed = True
    return failed


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    argv = [a for a in sys.argv[1:] if a != "--simulate-unterminated"]
    # Proves this gate catches the bug it exists for: with the pre-v3 matcher
    # (no `--`), X02's `-p 8083:8083` must fail here.
    terminated = "--simulate-unterminated" not in sys.argv
    corpus = argv[0]
    questions_path = argv[1] if len(argv) > 1 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "questions.json")

    questions = json.load(open(questions_path, encoding="utf-8"))["questions"]

    # Evaluability first: it applies to every question, and a literal the grader
    # cannot match makes every other check about that question meaningless.
    failed = check_evaluable(questions, terminated)
    if not terminated:
        print("(--simulate-unterminated: graded with the pre-v3 matcher)")

    discrimination = [q for q in questions if q.get("band") == "discrimination"]
    if not discrimination:
        print("no discrimination questions to check")
        return 1 if failed else 0

    for q in discrimination:
        for good in q.get("expect_top_all", []):
            for bad in q.get("expect_top_none", []):
                found = closest_cooccurrence(corpus, good, bad)
                if found is None:
                    print(f"{q['id']}  CLEAN     never co-occur in any ingested doc")
                    continue
                where, distance = found
                if distance <= FATAL_LINES:
                    verdict, failed = "FATAL", True
                elif distance < RISKY_LINES:
                    verdict = "risky"
                else:
                    verdict = "ok"
                print(f"{q['id']}  {verdict:9} closest: {where} ({distance} lines apart)")

    # A substring relation between the pair silently defeats the question: bare
    # "8222" matches inside "28222", so the twin would satisfy the answer check.
    for q in discrimination:
        for good in q.get("expect_top_all", []):
            for bad in q.get("expect_top_none", []):
                if good in bad or bad in good:
                    print(f"{q['id']}  FATAL     {good!r} and {bad!r} are substrings of "
                          f"one another; use a longer, prefixed literal")
                    failed = True

    if failed:
        print("\nunsafe discrimination questions — fix before scoring", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
