#!/usr/bin/env python3
"""Validate every composition question against the corpus it will be scored on.

A composition question asserts that answering it requires graph traversal —
an impact closure, a relation join, a cross-source join — because its facts do
not live together in any single retrievable passage. That property is about the
DATA, and it rots silently: an edit to a README, a refactor that moves a caller
into the callee's file, or a new doc that summarizes both sides of a join puts
the full fact set into one passage, and from that moment single-shot retrieval
(arm C, or doc_context) can pass the question. The band keeps reporting numbers;
the numbers stop measuring composition. Same rot class check-discrimination.py
guards against, so it gets the same treatment: verified against the corpus
before every scored run, never assumed.

The gate: a question in the composition band FAILS if its full `expect_all` set
co-occurs inside any single window of any ingested file.

Window semantics mirror what the INGESTER makes retrievable, erring strict:

  * Doc files (.md/.mdx/.txt) are chunked into passages, so the window is the
    LARGEST passage the splitter can legally emit — hardMax (6000 B in
    handler/doc/splitter.go's defaultBounds), not the 2000 B ceiling. A fenced
    code block is kept whole past the ceiling, so gating on the ceiling would
    admit a question that one oversized passage answers. If the splitter's
    bounds change, DOC_WINDOW_BYTES must follow.

  * Code and config files are ONE window each — a symbol body never extends
    beyond its file, so whole-file is a strict upper bound on any body-sized
    passage, and it needs no per-language parsing. This is deliberately harsher
    than the doc rule: two facts 9 KB apart in one Go file still FAIL, because
    a single function body could legally span them.

A composition question must also carry at least TWO expect_all literals: with
fewer, "the facts do not co-occur" is vacuously true of every corpus and the
question is not measuring composition at all.

Usage:
    scripts/scorecard/check-composition.py <corpus-dir> [questions.json]
                                           [--window N] [--simulate]

--simulate proves the gate fires: it appends a synthetic in-memory document
carrying a composition question's full fact set inside one window, and the run
must then FAIL (exit 1). test-matcher.sh asserts both directions, so an edit
that breaks the scan cannot pass silently as "no co-occurrence found".

Exits non-zero if any composition question is unsafe, so it can gate a run.
"""

import json
import os
import sys

# handler/doc/splitter.go defaultBounds.hardMax — see module docstring for why
# hardMax and not the ceiling.
DOC_WINDOW_BYTES = 6000

DOC_SUFFIXES = (".md", ".mdx", ".txt")
# The AST handler's languages plus the cfgfile handler's basenames; mirror the
# ingester, not the filesystem (the check-discrimination lesson: a checker that
# disagrees with the ingester about corpus membership reports failures nobody
# can fix).
CODE_SUFFIXES = (".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".java", ".svelte")
CONFIG_BASENAMES = ("go.mod", "package.json", "Dockerfile", "pom.xml")
CONFIG_SUFFIXES = (".gradle",)
EXCLUDED_DIRS = ("scripts/scorecard", "openspec", "vendor", "node_modules", ".git")


def ingested_files(corpus):
    """Yield (path, kind) where kind is 'doc' | 'code'; config counts as code
    (whole-file window)."""
    for root, _dirs, files in os.walk(corpus):
        rel_root = os.path.relpath(root, corpus)
        if any(rel_root == d or rel_root.startswith(d + os.sep) for d in EXCLUDED_DIRS):
            continue
        for name in files:
            path = os.path.join(root, name)
            if name.endswith(DOC_SUFFIXES):
                yield path, "doc"
            elif (name.endswith(CODE_SUFFIXES) or name in CONFIG_BASENAMES
                  or name.endswith(CONFIG_SUFFIXES)):
                yield path, "code"


def occurrences(text, literal):
    """All (start, end) byte offsets of literal in text; both already lowered."""
    hits, start = [], 0
    while True:
        i = text.find(literal, start)
        if i < 0:
            return hits
        hits.append((i, i + len(literal)))
        start = i + 1


def tightest_span(text, literals):
    """Smallest byte span of text containing one occurrence of EVERY literal,
    or None if some literal is absent. Both sides already lowercased."""
    per_lit = [occurrences(text, lit) for lit in literals]
    if any(not hits for hits in per_lit):
        return None
    events = sorted(
        (start, end, idx)
        for idx, hits in enumerate(per_lit)
        for start, end in hits
    )
    best = None
    for i, (anchor_start, _, _) in enumerate(events):
        seen, max_end = set(), 0
        for start, end, idx in events[i:]:
            seen.add(idx)
            max_end = max(max_end, end)
            if len(seen) == len(literals):
                span = max_end - anchor_start
                if best is None or span < best:
                    best = span
                break
    return best


def check_question(q, texts, window):
    """Return (verdict, detail) for one composition question against the corpus.

    texts: list of (display_path, kind, lowered_content).
    """
    literals = [lit.lower() for lit in q.get("expect_all", [])]
    if len(literals) < 2:
        return ("FATAL", "composition question needs >= 2 expect_all literals "
                f"(has {len(literals)}); a single fact cannot require a join")

    worst = None  # (path, kind, span) for the tightest co-occurrence anywhere
    for path, kind, text in texts:
        span = tightest_span(text, literals)
        if span is None:
            continue
        if worst is None or span < worst[2]:
            worst = (path, kind, span)
        if kind == "code":
            return ("FATAL", f"full fact set inside one file: {path} "
                    f"(a single code body could carry it)")
        if span <= window:
            return ("FATAL", f"full fact set within {span} B in {path} "
                    f"(<= {window} B window; one passage can carry it)")
    if worst is None:
        return ("CLEAN", "facts never co-occur in any single ingested file")
    path, _, span = worst
    return ("ok", f"tightest doc co-occurrence: {path} ({span} B apart, "
            f"window {window} B)")


def main():
    args = [a for a in sys.argv[1:] if not a.startswith("--")]
    simulate = "--simulate" in sys.argv
    window = DOC_WINDOW_BYTES
    if "--window" in sys.argv:
        window = int(sys.argv[sys.argv.index("--window") + 1])
        args = [a for a in args if a != str(window)]
    if not args:
        sys.exit(__doc__)
    corpus = args[0]
    questions_path = args[1] if len(args) > 1 else os.path.join(
        os.path.dirname(os.path.abspath(__file__)), "questions.json")

    questions = json.load(open(questions_path, encoding="utf-8"))["questions"]
    composition = [q for q in questions if q.get("band") == "composition"]

    if simulate and not composition:
        # Keeps the --simulate proof meaningful on a question set with no
        # composition band yet (or ever — the OSH set may admit none).
        composition = [{
            "id": "SIM",
            "band": "composition",
            "expect_all": ["synthetic-fact-alpha", "synthetic-fact-beta"],
        }]

    if not composition:
        print("no composition questions to check")
        return 0

    texts = []
    for path, kind in ingested_files(corpus):
        try:
            content = open(path, encoding="utf-8", errors="ignore").read()
        except OSError:
            continue
        texts.append((os.path.relpath(path, corpus), kind, content.lower()))

    if simulate:
        # A synthetic doc whose one line carries the first question's whole
        # fact set — the exact shape the gate exists to catch.
        q = composition[0]
        line = " ".join(lit.lower() for lit in q.get("expect_all", []))
        texts.append(("(simulated single-window doc)", "doc", line))
        print(f"(--simulate: injected a one-window doc carrying {q['id']}'s "
              "full fact set; the gate MUST fire)")

    failed = False
    for q in composition:
        verdict, detail = check_question(q, texts, window)
        if verdict == "FATAL":
            failed = True
        print(f"{q['id']}  {verdict:9} {detail}")

    if failed:
        print("\nunsafe composition questions — fix before scoring", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
