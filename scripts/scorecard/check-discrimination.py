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


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    corpus = sys.argv[1]
    questions_path = sys.argv[2] if len(sys.argv) > 2 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "questions.json")

    questions = json.load(open(questions_path, encoding="utf-8"))["questions"]
    discrimination = [q for q in questions if q.get("band") == "discrimination"]
    if not discrimination:
        print("no discrimination questions to check")
        return 0

    failed = False
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
