# SemSource — the case for sponsors and adopters, in three numbers

> **One-liner.** AI coding agents answer questions about your codebase three ways today: grep and
> read files, vector search, or a code graph. We measured all three head-to-head on the same
> questions: the graph was the only one that could answer *"what breaks if I change this?"* — and
> it did it at a ninth of the token cost and in a tenth of a second, with zero fabricated answers.

**Last verified:** 2026-08-12 · questions v4 / questions-osh v1 · commit `1694edc` ·
apples-to-apples method in `scripts/scorecard/` (see *Refreshing this page* below).

## 1. There is a class of question the other approaches physically cannot answer

Questions like *"what depends on this function?"* have answers scattered across files — an
automated checker proves no single file or passage contains the full answer, so the result is
structural, not cherry-picked. On those questions:

| approach | multi-file questions | context spent on them |
| --- | --- | --- |
| grep-and-read (agent floor) | 0/4 | 70.5 KB |
| vector search, top-20 (RAG) | 0/4 | 72.2 KB |
| **SemSource graph** | **4/4** | **7.8 KB** |

Vector search fails even when handed its twenty best results: relationships are stored by the
graph, not hoped for by retrieval. On the "find me the right snippet" questions, RAG and the
graph tie — which is exactly the honest framing: *RAG finds passages; the graph knows structure.
Agents need both classes of answer, and only one tool provides both.*

## 2. Tokens are money, and the difference is a line item you can compute

Answering the full 26-question set (identical questions, identical grading):

| approach | recall | total context an agent must ingest |
| --- | --- | --- |
| grep-and-read | 18/26 | 919 KB (~230k tokens) |
| vector search | 22/26 | 425 KB (~106k tokens) |
| **SemSource graph** | **26/26** | **347 KB (~87k tokens)** |

Multiply by your agents' questions per day. On the hard multi-file questions specifically, the
graph is ~9× cheaper than either alternative — naming edges is smaller than shipping passages.

## 3. It scales — proven on a codebase we do not control

Pointed cold at Open Sensor Hub core (1,932 Java files — an early adopter's real workload, not a
benchmark): **32,157 entities indexed to full readiness in 22 minutes**, 9/10 questions answered
with **zero fabricated answers on first contact**, and first-call latency **flat at ~108 ms
median at 5× the entity count** of our own repo. The naive vector-scan alternative degraded
linearly to ~9 s per question on the same corpus.

## Why the numbers can be believed

- **Reproducible.** Questions, grader, and harness are published in `scripts/scorecard/`;
  grading is exact string matching — no LLM judging its own homework.
- **Self-honest.** Answers that vary between runs are reported as *unstable*, never silently
  resolved to the better result. Question sets are version-locked; scores are never quoted
  across versions or corpora.
- **It can hurt us.** The benchmark's first run at scale found three bugs in our own product;
  we filed them publicly the same day (#141, #142, #143). A benchmark that can embarrass its
  authors is one an adopter can trust.
- **Zero fabrication, both corpora.** Deterministic ingestion means the graph never invents an
  entity — the "must miss" questions miss, every run.

## Talking-point cheat sheet

- **For an adopter** (especially a Java shop): lead with §3 — *their* codebase shape, cold
  start, ~100 ms answers, nothing made up.
- **For a sponsor**: lead with §2 and multiply — token cost × questions/day × agents.
- **On "why not just RAG?"**: don't argue; point at the §1 table. RAG tied on snippets, lost on
  relationships — and impact analysis is what agents ask before touching production code.
- **Keep 26/26 humble**: "on our own repo, with a published, checker-gated question set." The
  moment it sounds like a leaderboard, you inherit every AI benchmark's credibility problem.

## Refreshing this page

The numbers above come from committed scorecard baselines — regenerate, then update the tables
and the *Last verified* stamp:

```bash
scripts/scorecard/corpus-osh.sh <dir>            # OSH corpus at its pinned SHA
scripts/scorecard/check-composition.py <corpus>  # gates before every scored run
scripts/scorecard/run.sh <label>                 # arm B (the product)
scripts/scorecard/arm-c-cosine.sh <label>        # arm C (vector floor)
scripts/scorecard/arm-a-grep.sh <corpus> <label> # arm A (grep floor)
scripts/scorecard/compare.sh results/<...>.json  # the per-band table
```

Sources of record: `scripts/scorecard/results/SUMMARY-v4-baseline.md` (dogfood) and
`SUMMARY-osh-v1.md` (OSH scale point). Rules that keep the page honest: never mix corpora, never
mix question-set versions, latency compares only same-machine runs.
