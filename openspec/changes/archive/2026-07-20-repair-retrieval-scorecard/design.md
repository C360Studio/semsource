# Design — repairing the retrieval scorecard

## Context

`retrieval-ranking` obliges passage retrieval to be verified against the graded set rather
than asserted. The instrument cannot currently discharge that: one discrimination question is
graded by a matcher that errors on its own literals, and verdicts vary with the position a
question occupies in the run. `scripts/scorecard/results/SUMMARY-instrument-diagnosis.md`
carries the full measurement; this document decides what to do about it.

The proposal left one question deliberately unresolved: **is the order dependence SemSource's
or the substrate's?** It is now answered by measurement, and the answer changes the shape of
the fix.

### The order dependence is substrate's, and it is transient

Reproduced in a single MCP session: 21 diverse queries, then X02.

```
after 21 diverse queries:
  X02 call 1: TIER2 ABSENT
  X02 call 2: TIER2 rank [1]
  X02 call 3: TIER2 rank [1]
  X02 call 4: TIER2 rank [1]
```

At that same instant, `graph.query.semantic` — bypassing fusion entirely — returned the Tier 2
passage at rank 1, similarity 0.6801, unchanged. So:

- **Recall is correct**; the entity is lost downstream, inside fusion's hydrate → rank →
  budget path.
- **It is the top-ranked entity that vanishes.** Other passages from the *same document*
  (`configs-tiers-README-md-0003`, `-0004`) survive at ranks 9 and 18.
- **It is transient and self-healing** — the immediately following identical call is correct.
  It is not persistent session state, and it is not call count (24 identical calls in a row
  were all correct).
- **It is completely silent.** Zero `WARN` or `ERROR` in the window. `fusion.Node` carries no
  score, `engine_lens.go` logs nothing, and `fusionnats.resolveSemantic` discards the
  similarity it receives.

The most plausible mechanism is the seed-hydration step: `fusionnats.Entities` calls
`graph.query.batch`, whose contract silently omits IDs it cannot return, under a 5 s
per-graph-call timeout (`fusionnats.New(deps.NATSClient, 0)`). A cold or slow first read after
a burst would drop the ID with no trace. That is a hypothesis about substrate internals, not a
finding — the finding is the black-box behaviour above, which is reproducible without it.

**Eliminated: the embedding service degrading under load.** The natural suspicion is that the
query-embedding path flakes on the first call after a burst — it would explain the transience,
the self-healing, and the shifted candidate set. It was tested directly: run the same 21-query
burst, then make `graph.query.semantic` the **first** call afterwards, bypassing fusion. The
recall list came back pristine — identical similarities (0.6801 / 0.6443 / 0.6155 / 0.6124) in
identical order, with no errors in the `semembed` log for the whole session.

Since fusion reaches the embedder through that same subject and handler, the burst does not
degrade query embedding, cosine scoring, or candidate selection. The defect is strictly
downstream of recall. (Worth stating explicitly because the stack runs **tier-1 only** —
`configs/mvp.json`, `semembed`; `seminstruct` is tier-2 and is *not running*. It appears here
only as corpus text that X02 asks about, and is never called.)

Per the Product Boundary this is not ours to fix. It is also **not merely an instrument
problem**: a consumer's first `doc_context` call after a busy period can silently lose the
single best piece of evidence, with nothing in the response indicating it happened.

## Goals / Non-Goals

**Goals:**

- A graded verdict reflects retrieval and nothing else — no verdict may be produced by a
  matcher that failed to evaluate.
- A question's verdict does not depend on its position in the run, **or** the instability is
  reported rather than hidden.
- The three-state discrimination outcome is named in three verdicts.
- The substrate defect is reduced to a minimal reproduction and filed.

**Non-Goals:**

- Fixing the fusion hydration defect. Substrate; issue only, never a PR to semstreams.
- Fixing the default-vs-override ranking defect (`fix-default-vs-override-ranking`).
- Widening the discrimination band, re-tuning bounds, or introducing an LLM judge.

## Decisions

### D1 — Terminate option parsing in all five matcher loops

`grep -qF -- "$w"`. Verified: this alone flips X02 `miss` → `correct` with no product change
(`results/x02-grepfix.json`, `results/full-grepfix-x02first.json`).

*Alternative considered:* `printf '%s' "$hay" | grep -qF -e "$w"`. Equivalent, but `--` is the
conventional and more obvious form. *Alternative rejected:* rewriting the matcher in `jq` with
`contains`. It would sidestep `grep` quoting entirely and is tempting, but it changes the
matching semantics of every existing question at the same time as the questions are being
re-versioned — two variables at once, in the one place this project cannot afford them.

### D2 — Gate grader-evaluability in `check-discrimination.py`, not just corpus shape

The current checker validates the *corpus* relationship between a confusable pair (not a
substring of one another; not co-occurring closely in any ingested doc). It has no opinion on
whether the grader can evaluate the literals, which is why X02 passed validation and then
failed forever.

Add a check that every literal in `expect_all`, `expect_any`, `expect_none`, `expect_top_all`
and `expect_top_none` is *evaluable*: feed it through the same matcher the grader uses, against
a string known to contain it and a string known not to, and require the two to disagree. A
literal that cannot distinguish those two cases fails validation.

This is deliberately a **behavioural** check rather than a syntactic one (e.g. "reject literals
starting with `-`"). A syntactic rule enumerates today's known footgun; the behavioural check
catches the next one — an unbalanced bracket, an embedded newline — without anyone predicting
it.

### D3 — Measure the instability; do not paper over it

This is the load-bearing decision.

The obvious workaround is a warm-up call, or retrying a question and taking the successful
result. **Both are rejected.** They would convert a live, consumer-facing product defect into
a number that looks clean, and this project's stated value is that evidence can be trusted
without re-deriving it. An instrument that hides a defect to protect its own score is worse
than one that cannot measure at all, because it is confidently wrong.

Instead: **each question is asked N times (default 3) and disagreement is a reported outcome.**

- All N agree → the verdict stands, as today.
- The N disagree → verdict `UNSTABLE`, recording each distinct outcome and the position in
  the run. `UNSTABLE` is counted separately and never silently folded into `correct` or
  `miss`.

This costs 3x wall-clock on a run that is already dominated by stack provisioning, and it
turns retrieval stability into a measured dimension rather than an assumption. It also gives
the upstream issue a continuously-running reproduction rather than a one-off anecdote.

*Alternative considered:* keep single calls and simply document the caveat. Rejected — the
caveat is precisely what a future reader would skip before quoting a score, which is exactly
how `discrimination 0/2` came to be quoted as a retrieval finding.

*Alternative considered:* randomise question order per run. Rejected — it converts a
reproducible defect into noise, making runs less comparable rather than more.

### D4 — Name the third verdict `MISLEADING`

The three discrimination states and their verdicts:

| top node holds | verdict |
| --- | --- |
| the answer, not the twin | `correct` |
| the answer **and** the twin | `IMPRECISE` |
| the twin, **not** the answer | `MISLEADING` |
| neither | `miss` |

`MISLEADING` is separated from `miss` for the same reason `IMPRECISE` is separated from
`FABRICATED`: they are different failures and conflating them hides the one that matters. A
`miss` returns nothing useful; a `MISLEADING` result actively argues for the wrong answer, and
an agent citing the top node will state it as fact. It is the verdict X01 earns today, and the
one that would show `fix-default-vs-override-ranking` working.

Detection: `expect_top_none` matched **and** `expect_top_all` unmatched. This needs no new
question fields — the existing pair already expresses it — so version 3 changes the grader's
interpretation, not the question schema.

*Naming alternatives considered:* `WRONG` (too close to `miss` in a scan), `CONFUSED`
(describes the system's state rather than the evidence's effect), `INVERTED` (implies exactly
two candidates, which is not general).

### D5 — Version 3, and no cross-version arithmetic

`questions.json` goes to version 3 because D1 and D4 both change recorded outcomes without any
product change. Version 2 results are not comparable across it, and the README's existing
comparability rule already forbids that comparison — this change adds the version bump and a
line naming the two reasons, so a future reader understands *why* the numbers moved.

A version-3 baseline is recorded on the fixed instrument. Expected on the current binary:
X02 `correct`, X01 `MISLEADING`, discrimination 1/2. Recording it is a task, not a prediction
to be asserted — if it lands differently, the difference is the finding.

### D6 — The body-less dependency node is SemSource's, and it lands here

Surfaced during diagnosis: `doc_context` returned `github.com/containerd/platforms` — a
`{org}.semsource.config.…dependency.*` Go-module entity with no `body` field — as the **top**
node. `dropNavigationalNodes` (`processor/code-context/component.go:318`) filters only
`Kind=="document" && Body==""`; a config dependency entity is neither, so it survives, and
`{org}.semsource.config` is half of `doc_context`'s default scope, so these compete on every
doc query.

This is the body-less-lead defect change 10.3 addressed, resurfacing through a class the guard
never covered. It is SemSource's, not substrate's.

It belongs in this change rather than a third one because it is an *instrument* blocker: a
body-less top node produces `top_body_bytes=0` and a guaranteed `miss` on any question graded
against the top node, which is a second way for the scorecard to report a failure that is not
a ranking failure. Fixing it is a precondition for the band measuring what it claims to.

The fix generalises the guard from "a document with no body" to "any node with no retrievable
body", which is the property that actually makes a node useless as evidence. Whether that is
better expressed as a `doc_context` filter or as salience on the dependency class is an
implementation choice for the tasks; the guard is the decision.

### D7 — File the substrate defect with the reproduction, and keep our own record

An entry in `docs/upstream/semstreams-asks.md`, triaged framework-shaped, plus a GitHub issue
on semstreams — never a PR. The ask has two parts, and the second is the more valuable:

1. Seed hydration must not silently drop an ID. Either surface the omission in the response or
   fail the call.
2. **Fusion needs score observability.** `fusion.Node` carries no score or rank; the engine
   logs nothing; `fusionnats.resolveSemantic` decodes `entity_id` and discards `Similarity`
   (`pkg/fusion/fusionnats/client.go:153-168`). Diagnosing this required bypassing the product
   surface entirely and calling `graph.query.semantic` over raw NATS. Any consumer hitting a
   ranking surprise has no recourse at all.

## Risks / Trade-offs

- **[D3 makes runs 3x longer]** → The run is dominated by stack provisioning and embedding, not
  by queries (22 questions complete in well under a minute against a ready stack). Make N a
  knob (`SCORECARD_REPEATS`, default 3) so a quick check can set it to 1, and state plainly in
  the README that a 1-repeat run cannot detect instability.
- **[`UNSTABLE` could become background noise that is routinely ignored]** → It is reported in
  the summary block alongside fabrication, with the count of affected questions. If it becomes
  routine, that is a finding about the product, not a reason to suppress the verdict.
- **[D6 could over-filter and drop legitimate evidence]** → The guard keys on *no retrievable
  body*, which is exactly the property that makes a node useless as a citation; a node with a
  body is untouched. `dropNavigationalNodes` already refuses to empty the result set, and that
  protection is preserved. The doc bands are saturated (10/10) and act as the regression
  detector.
- **[The version bump breaks the longitudinal series again]** → It already broke: the audit's
  13/19 → 16/19 → 17/19 series used a different, lost set, and version 2's discrimination
  numbers are being retracted here as never having measured retrieval. Better to break it
  knowingly, once, with the reason recorded, than to preserve a series that was not measuring
  what it claimed.
- **[The substrate defect may never be fixed upstream]** → D3 means the instrument reports it
  rather than depending on it being fixed. The scorecard stays usable either way.

## Migration Plan

No runtime migration; the scorecard is an in-repo instrument. Ordering matters only in that
D1, D4 and D6 all move recorded outcomes, so the version-3 baseline is recorded **once, after
all three land**, against the same fixed corpus (`git archive d554bcc`, `scripts/scorecard/`
excluded) used throughout this diagnosis.

Rollback is `git revert`; no state, no consumers, no deployed surface.

## Open Questions

- **Is D6's guard better placed as a `doc_context` filter or as negative salience on the
  dependency class?** The measurement in `fix-default-vs-override-ranking` shows salience is a
  weak lever (range `[-6, 0]` against a 160-point recall span), which argues for a filter — but
  a filter is a blunt instrument the substrate cannot see through. Decide in tasks, with the
  doc bands as the regression check.
- ~~**Does the transient drop affect `code_context` too?**~~ **Probed.** After the same
  21-query burst, `code_search` (C04/C05/C06 — the embedding-backed code lens, not just the
  structural `byName` path) returned an identical top node across 4 consecutive calls each,
  with no instability. So the reproduction is `doc_context`-only on the evidence available, and
  the upstream issue should say exactly that rather than generalising across lenses. It is not
  yet strong enough to claim the code lens is *immune* — only that the burst did not perturb
  it — and the difference between the lenses (docs resolve via `ResolveModeNL`, code seeds
  through `exactSeedClient`) is the obvious place to look if it ever does.
