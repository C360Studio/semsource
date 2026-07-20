# A/B — splitting homogeneous key/value blocks (fix-default-vs-override-ranking)

**Date:** 2026-07-20. **questions.json version 3**, `SCORECARD_REPEATS=3`. Corpus held fixed:
`git archive d554bcc`, 206 ingestable documents, `scripts/scorecard/` excluded. Only the binary
varied; `docker compose down -v` between sides. Baseline is `v3-baseline.json`, candidate is
`v3-candidate.json`.

## Result — the change works

| | baseline | candidate |
|---|---|---|
| score | 20/22 | **21/22** |
| discrimination | 0/2 | **1/2** |
| **X01** | `MISLEADING` | **`correct`** |
| X02 | `UNSTABLE` | `UNSTABLE` |
| code / doc-early / doc-late / impact / negative | 6/6 · 3/3 · 7/7 · 2/2 · 2/2 | 6/6 · 3/3 · 7/7 · 2/2 · 2/2 |
| entities | 5410 | 5417 (**+7, +0.13%**) |
| median top-node body (doc bands) | 1228 B | 1102 B |
| time to fully ready | — | 150 s |

**X01 moved `MISLEADING` → `correct`**, which was the single pre-declared signal. Its top node is
now **218 bytes** — the `NATS_*` key group — instead of the 711-byte § Quick Start passage carrying
the workaround value. `MISLEADING` count across the whole set went 1 → 0.

**The prediction held, from cosine to stack.** Offline, the emitted 218-byte group scored **0.7663**
against the query versus the distractor's **0.7323**; the whole § Configuration block it replaced
scored 0.6569. A +0.034 margin was enough to reorder recall, and the live result matches. That the
offline harness predicted the stack outcome is worth as much as the outcome — it means the next
change of this shape can be evaluated for the cost of an embedding call rather than a stack rebuild.

## No regression, and the regression check is the point

The doc bands are **unchanged at 10/10 combined**. They are saturated, so they cannot show
improvement; their only job here is to catch prose being shredded by a splitter that now cuts inside
fenced blocks. They caught nothing.

Evidence got slightly tighter as a side effect: median top-node body 1228 → 1102 bytes. Not a goal,
and not claimed as one.

## The cost was small, and the gate is why

**+7 entities on 206 documents (+0.13%).** The three-group gate (design D6) means the trigger fires
only where a block genuinely carries several independent settings; most fenced blocks in this corpus
are code, and code is untouched. Time to fully ready was 150 s, in line with the baseline.

This is the number that would have grown alarmingly under a per-key split, which is the other reason
grouping beat atomising — 70-byte passages scored marginally higher (0.8133 vs 0.7663) but would
have multiplied entities and made thin evidence.

## What this does NOT establish

- **X02 is not evidence here, in either direction.** It stayed `UNSTABLE` — its verdict still varies
  across repeats because of [semstreams#597](https://github.com/C360Studio/semstreams/issues/597),
  which this change does not touch. It was declared a non-signal before the run, and it is not being
  read as one after it.
- **Nothing about passage bounds.** Ceiling, floor and hard max are unchanged. This change adds a
  split *trigger*; it does not retune a bound, and `SUMMARY-9.5-bounds.md` still stands.
- **Nothing about code fences generally.** The narrowing applies only to blocks that are
  predominantly `KEY=VALUE`. The original rationale — that splitting code hurts retrieval more than
  an oversized passage does — is untested for Go or shell bodies and is left intact for them.
- **A one-question improvement is a one-question improvement.** The discrimination band is two
  questions because this corpus supports two. X01 moving is real and pre-declared, not a fishing
  expedition, but it is not a broad claim about retrieval quality.

## Reproducing

```bash
COMPOSE_PROJECT_NAME=ranktest SEMSOURCE_HTTP_PORT=28080 \
  NATS_HOST_PORT=24222 NATS_MONITOR_HOST_PORT=28222 \
  SEMSOURCE_TARGET=<corpus> docker compose up -d --build
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh <label>
```

Corpus: `git archive d554bcc | tar -x -C <corpus>` then remove `scripts/scorecard/`.
