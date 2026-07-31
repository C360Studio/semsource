## 1. Land the instrument and pin its credibility

- [x] 1.1 Add the offline cosine harness under `handler/doc` alongside `TestBoundsSweep`: split named
  documents with the real splitter, embed passages through semembed with the arctic-embed query
  prefix applied to the query only and the `document § heading` identity text prepended to each body,
  and report the signed margin between a named answer and a named distractor.
  - Test: `SCORECARD_EMBED=<url> go test ./handler/doc/ -run TestDilutionMargin -v` reports a margin
    for a declared answer/distractor pair; it skips cleanly when the embedder env is unset.
- [x] 1.2 Pin the harness's admissibility with the X02 three-state reproduction: the answer passage at
  the pre-#109, post-#109, and current versions of `configs/tiers/README.md` against the ADR-0002
  distractor.
  - Test: the harness reports margins of approximately +0.0074, −0.0021 and −0.0291, i.e. the sign
    flips between the first and second state — matching the observed live ranks 0, 1 and 4.
- [x] 1.3 Record the corpus, embedder image digest, and query prefix alongside the harness output, so
  a later run is comparable.
  - Test: the emitted record names all three; a run with a different embedder digest is
    distinguishable from one with the same.

## 2. Generalize the split rule

- [ ] 2.1 Add continuation detection: a line is a continuation when it is indented, ends in a
  continuation token (`{`, `[`, `(`, `,`, `\`, `|`, `&&`), or opens/continues a here-document.
  - Test: table test over indented lines, trailing-operator lines, here-doc bodies, and plain
    commands classifies each correctly.
- [ ] 2.2 **BREAKING** Replace the `KEY=VALUE`-specific qualifier with independent-peer detection: a
  fenced block divides only when no line is a continuation and it yields at least three leading-token
  groups. `KEY=VALUE` remains a recognised peer form.
  - Test: a fenced block of three or more distinct shell commands divides into one passage per
    command group; the existing `KEY=VALUE` division is unchanged.
- [x] 2.3 Guard the failure that matters: a block whose lines form one continuous construct is kept
  whole.
  - Test: Go function bodies, JSON documents, multi-line pipelines, and here-documents each remain a
    single passage; the corpus's ~142 continuous blocks produce the same boundaries as before.
- [x] 2.4 Preserve the tiling and merge invariants under the generalized rule.
  - Test: divided passages still tile the document byte for byte with no gap, overlap, or duplicated
    text including fence markers; below-floor groups are still emitted rather than merged.

## 3. Score offline before spending a live A/B

- [ ] 3.1 Score the generalized rule offline on the unmodified `configs/tiers/README.md`: X02's answer
  passage against the ADR-0002 distractor.
  - Test: the margin returns to positive without any edit to the document; if it does not, the rule
    does not proceed to task 4.
- [ ] 3.2 Score the rule across the graded set's other answer/distractor pairs to catch a candidate
  that fixes X02 by damaging something else.
  - Test: no pair's margin moves negative; any pair that does is named and explained before
    proceeding.
- [ ] 3.3 Record passage-count growth over the fixed baseline corpus.
  - Test: total passage and entity counts are reported against the 5417-entity baseline.

### Iteration 1 result — fence isolation fixes X02 and REGRESSES X01. Not shippable.

Live A/B on the fixed baseline corpus (5417 -> 5623 entities, +3.8%): **21/22 against a 22/22
baseline**, with X01 moving `correct` -> `MISLEADING` — the verdict `retrieval-ranking` singles out
as the worst, because the top evidence argues for the wrong answer.

Cause: **isolation is symmetric.** X01's distractor is itself a fenced `docker compose` block, so
isolating sharpened the workaround passage exactly as it sharpened X02's answer. Offline margins now
read X02 `+0.0127` and X01 `-0.0009` — the regression is a thousandth of a point, and real.

Two process findings, both recorded rather than smoothed over:

1. Task 3.2 said to score the other pairs offline BEFORE the live A/B. That step was skipped and the
   live run paid for it. The X01 pair is now in the harness so the next iteration costs an embedding
   call.
2. The harness initially reported X01 `+0.0893` — a false pass — because it selected the distractor by
   heading, and isolation had split `§ Quick Start` into an 88 B prose fragment and the 463 B fence.
   Distractors are now selected by literal. This is exactly the disagreement case the spec anticipates:
   the live result stood and the harness was treated as incomplete.

## 4. Confirm live, on the fixed baseline corpus

- [ ] 4.1 Re-score the graded set on `git archive d554bcc` (206 ingestable docs,
  `scripts/scorecard/` excluded), gated on `phase` + `index.ready` + `embedding.ready`.
  - Test: `SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh <label>` completes with
    `SCORECARD_REPEATS=3`; result lands in `scripts/scorecard/results/`.
- [ ] 4.2 Compare band by band against `beta159-fixed-corpus.json` (22/22) and account for every
  changed verdict.
  - Test: the comparison names each changed verdict; the score does not fall, and any band that moves
    is explained rather than averaged away.
- [ ] 4.3 Re-score on the current-main corpus, where X02 is presently a `miss`.
  - Test: X02 returns `correct` with the answer passage top-ranked, on the unmodified document.
- [ ] 4.4 Write the A/B summary beside the existing ones, stating the corpus, question-set version,
  and what did not move.
  - Test: the summary records the questions.json version and states explicitly that it does not
    compare across versions.

## 5. Guard the regression

- [ ] 5.1 Add a splitter-level guard that the passage carrying X02's answer stays below the size at
  which its margin went negative, on the current document.
  - Test: a unit test over `configs/tiers/README.md` asserts the answer-bearing passage is divided
    from the prose around it.
- [ ] 5.2 Record in the harness the answer/distractor pairs it protects, so a future dilution is
  caught offline rather than by a graded miss.
  - Test: the harness run reports every declared pair's margin and fails on a negative one.

## 6. Review and release gates

- [ ] 6.1 Run the full gate set.
  - Test: `task check`, `task test:race`, `task test:e2e`, and `task core:smoke` all pass with revive
    clean.
- [ ] 6.2 Update `doc-passage-chunking`'s Purpose, which currently describes the `KEY=VALUE` rule as
  the exception, to describe independent-peer blocks.
  - Test: the archived spec's Purpose matches the shipped behaviour; no reference to `KEY=VALUE` as
    the sole divisible form remains.
