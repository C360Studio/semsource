# Retrieval scorecard

A fixed question set and a deterministic grader for measuring what SemSource can
actually answer about a corpus, run over the real MCP surface against a live stack.

It lives in the repository on purpose. The 2026-07-19 audit's graded set was built
in a session scratchpad and was gone by the time the next change wanted to re-run
it, which cost the project its only longitudinal retrieval measurement.

## Comparability — read this before quoting a number

**A score is only meaningful against another score taken with the same
`questions.json`.** The set is versioned (`version` field); bump it on any change
to a question, and never compare across versions.

In particular, **these numbers do not continue the audit's 13/19 → 16/19 → 17/19
series.** That series used a different, now-lost set. Quoting a score here as if it
extended that line would be inventing a trend.

## Running it

The harness does not provision anything, so the same script can be pointed at two
different builds. Stand up a stack, wait for ready, then:

```bash
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh <label>
```

Results land in `results/<label>.json` with every answer retained, so a verdict can
be re-examined without re-running the stack.

Knobs: `SEMSOURCE_HTTP_PORT` (default 28080), `SEMSOURCE_HOST`,
`SCORECARD_READY_TIMEOUT`, `SCORECARD_EMBED_SETTLE`, `SCORECARD_CALL_TIMEOUT`,
`SCORECARD_QUESTIONS`.

Always use an isolated `COMPOSE_PROJECT_NAME` and high ports — this machine also
runs other stacks, and `nats` on 4222 is generally not ours.

## The A/B procedure

To measure a change rather than a build, score **both sides against one set**:

1. Check out the baseline commit, bring up a stack with a **fresh** graph
   (`docker compose down -v`), wait for ready, run with a label.
2. Check out the candidate commit, `down -v` again, bring up, run with another label.
3. Compare per band.

The `down -v` matters. Doc identity and body handles changed in the passage-chunking
work, and a graph carried over from the other side is neither one thing nor the other.

## Why grading is deterministic

Substring matching, case-insensitive, no model in the loop. An LLM judge drifts
between runs, and a drifting judge cannot support an A/B — a score change becomes
indistinguishable from a judge change. The trade is that matchers are coarse: they
verify the answer *contains* the load-bearing fact, not that the prose around it is
good.

Three verdicts, deliberately distinct:

- **correct** — required content present.
- **miss** — the answer did not contain it. An honest failure.
- **FABRICATED** — the answer asserted something known to be false. This is not a
  worse miss, it is a different failure, and it outranks every other result in the
  summary. Zero fabrication is this product's actual moat; a set that scores well
  while inventing one answer has failed.

An `isError` result is recorded as `error`, never graded as an answer.

## Question bands

Bands exist so a score can be read rather than just totalled.

- **doc-early** — facts inside the first 8 KB of their document.
- **doc-late** — facts past 8 KB.
- **code** — symbol and concept retrieval on the code side.
- **impact** — dependents named, not merely counted.
- **negative** — must miss.

`doc-early` versus `doc-late` is the load-bearing split for passage chunking. The
substrate truncates embedding text at 8000 characters, so before chunking a
document's tail was silently unindexed. On this repository the README's own cut
falls right after the "UI profile" section: Quick Start, Source Types and the CLI
Reference sit before it; the Port Map, Status Phases, Config File and the whole
Fusion API sit after.

**`doc-early` is the control and it is not decoration.** A set made only of
`doc-late` questions would be rigged — chunking would improve it by construction.
The early band is what detects a regression: if chunking breaks something that
already worked, it shows up there, and a `doc-late` gain bought with a `doc-early`
loss is not a win.

## Adding questions

Each question needs a `why` explaining what it probes and where the fact lives.
Prefer facts that are verifiable in the corpus and stable across edits. Avoid
questions whose answer is a line number, a byte offset, or anything that churns on
unrelated commits.
