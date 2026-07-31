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

- [x] 2.1 ~~Add continuation detection~~ Superseded by design D1: the harness killed peer-entry
  division before code was written (X02's block is two commands; `minKeyGroups = 3` never fires).
  The generalization is fence ISOLATION, not entry grouping.
- [x] 2.2 Implement `isolateFencedBlocks`: a fenced block that is not a homogeneous key/value list
  becomes its own passage when the section's non-fence content is at least the floor. The block is
  never divided; `KEY=VALUE` grouping is retained unchanged.
  - Test: `handler/doc/isolation_test.go` — isolation fires with substantial prose, stays out below
    the floor, and never divides a block.
- [x] 2.3 Guard the failure that matters: a block whose lines form one continuous construct is kept
  whole.
  - Test: Go function bodies, JSON documents, multi-line pipelines, and here-documents each remain a
    single passage; the corpus's ~142 continuous blocks produce the same boundaries as before.
- [x] 2.4 Preserve the tiling and merge invariants under the generalized rule.
  - Test: divided passages still tile the document byte for byte with no gap, overlap, or duplicated
    text including fence markers; below-floor groups are still emitted rather than merged.

## 3. Score offline before spending a live A/B

- [x] 3.1 Score the generalized rule offline on the unmodified `configs/tiers/README.md`: X02's answer
  passage against the ADR-0002 distractor.
  - Test: the margin returns to positive without any edit to the document; if it does not, the rule
    does not proceed to task 4. **Result: +0.0121 (isolation + heading-path identity).**
- [x] 3.2 Score the rule across the graded set's other answer/distractor pairs to catch a candidate
  that fixes X02 by damaging something else.
  - Test: no pair's margin moves negative; any pair that does is named and explained before
    proceeding. **Result: X01 is in the harness as a protected pair; +0.0210 after iteration 2.**
- [x] 3.3 Record passage-count growth over the fixed baseline corpus.
  - Test: total passage and entity counts are reported against the 5417-entity baseline.
    **Result: 5623 entities (+3.8%), all from isolation; heading-path identity adds none.**

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

### Iteration 2 — heading-path identity restores X01 without touching split shapes

Correction to the iteration-1 causal note first: the sharpened X01 competitor is **not** the fenced
`docker compose` block — the whole-file rank probe shows the fence passage nowhere near the top. It
is the 463 B prose passage LEFT BEHIND when the fence was isolated out of `§ Quick Start`: the
port-conflict workaround blockquote, no longer diluted by the `git clone` fence beside it. Isolation
is symmetric, but the symmetric effect lands on the leftover prose, not the isolate.

The fix: passages now record their heading's full ancestor chain (`passage.HeadingPath`), and
`passageTitle` spells it out — the identity text for the answer becomes
`SemSource § Docker Compose § Configuration` instead of `SemSource § Configuration`. The query
names the parent section ("in docker compose"); the document always declared that structure; the
splitter was discarding it. Measured offline: X01 answer 0.7584 → 0.7803, margin −0.0009 → **+0.0210**,
distractor unchanged; X02 margin +0.0121, still positive, answer still rank 0 in the whole-file
probe. Split shapes, passage IDs, and entity counts are untouched; `DocSection` keeps the immediate
heading so URL anchors are unaffected. The credibility pin still reproduces the live ordering
(+0.0067 / −0.0028 / −0.0298 — signs unchanged).

- [x] 2.5 Carry heading ancestry on passages and into `dc.terms.title`; keep `DocSection` verbatim.
  - Test: `TestSplitPassages_HeadingPathCarriesAncestry`, `_HeadingPathSetextLevels`,
    `_MergedSectionsKeepFirstPath`, `TestPassageTitle_AncestryAndCollapse`; harness `identityText`
    now calls the real `passageTitle` so it cannot drift from production.

## 4. Confirm live, on the fixed baseline corpus

- [x] 4.1 Re-score the graded set on `git archive d554bcc` (206 ingestable docs,
  `scripts/scorecard/` excluded), gated on `phase` + `index.ready` + `embedding.ready`.
  - Test: `SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh <label>` completes with
    `SCORECARD_REPEATS=3`; result lands in `scripts/scorecard/results/`.
    **Result: `headingpath-fixed-corpus` — 22/22, repeats=3, 5623 entities, 0 MISLEADING.**
- [x] 4.2 Compare band by band against `beta159-fixed-corpus.json` (22/22) and account for every
  changed verdict.
  - Test: the comparison names each changed verdict; the score does not fall, and any band that moves
    is explained rather than averaged away. **Result: verdict-identical to the baseline — no
    verdict changed in any band; entity growth 5417 → 5623 (+3.8%) is the isolation cost already
    recorded in 3.3.**
- [x] 4.3 Re-score on the current-main corpus, where X02 is presently a `miss`.
  - Test: X02 returns `correct` with the answer passage top-ranked, on the unmodified document.
    **Result: `headingpath-current-main` — 22/22, X02 `miss` → `correct`, top node 180 B, on the
    unmodified document. 5716 entities.**
- [x] 4.4 Write the A/B summary beside the existing ones, stating the corpus, question-set version,
  and what did not move.
  - Test: the summary records the questions.json version and states explicitly that it does not
    compare across versions. **`results/SUMMARY-headingpath.md`.**

## 5. Guard the regression

- [x] 5.1 Add a splitter-level guard that the passage carrying X02's answer stays below the size at
  which its margin went negative, on the current document.
  - Test: `TestScorecardAnswerPassagesStayDivided` — always-on (no embedder), over the real
    `configs/tiers/README.md` and `README.md`, asserts both graded answers sit in passages ≤400 B
    divided from their diluting prose.
- [x] 5.2 Record in the harness the answer/distractor pairs it protects, so a future dilution is
  caught offline rather than by a graded miss.
  - Test: the harness run reports every declared pair's margin and fails on a negative one.
    **`protectedPairs` holds X01 and X02; `TestDilutionMargin` fails on any negative margin.**

## 6. Review and release gates

- [x] 6.1 Run the full gate set.
  - Test: `task check`, `task test:race`, `task test:e2e`, and `task core:smoke` all pass with revive
    clean. **All four passed on the iteration-2 tree (2026-07-31).**
- [x] 6.2 Update `doc-passage-chunking`'s Purpose, which currently describes the `KEY=VALUE` rule as
  the exception, to describe independent-peer blocks.
  - Test: the archived spec's Purpose matches the shipped behaviour; no reference to `KEY=VALUE` as
    the sole divisible form remains. **Done at sync: Purpose now names both size-independent rules
    (key-group division and fence isolation) and the ancestry-qualified titles.**
