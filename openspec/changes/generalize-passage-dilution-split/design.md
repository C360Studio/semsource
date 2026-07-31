## Context

The splitter's one size-independent rule divides a fenced block when its lines are predominantly
`KEY=VALUE`. It was added to fix graded question X01, and it worked: the emitted 218-byte key group
beat its distractor offline and then beat it live.

The rule's comment states the principle correctly — "a list of independent settings dilutes its own
vector at any size" — but the implementation tests for `KEY=VALUE`, which is one *instance* of
independence rather than independence itself. Graded question X02 sits in the gap: its answer is a
`docker run` command in a fenced block that is not `KEY=VALUE`, in a section that has stayed just
under the 2000-byte ceiling. Both gates miss it.

The measurement, on the real splitter and the deployed embedder:

| corpus state | answer passage | cosine | distractor (constant) | margin | live rank |
|---|---|---|---|---|---|
| pre-#109 | 839 B | 0.6481 | 0.6407 | **+0.0074** | 0 — `correct` |
| post-#109 | 1964 B | 0.6386 | 0.6407 | −0.0021 | 1 — `miss` |
| current | 1943 B | 0.6116 | 0.6407 | −0.0291 | 4 — `miss` |

Two ordinary documentation edits moved a graded verdict, and the offline harness reproduces the live
ordering at all three states including the sign of the margin.

## Goals / Non-Goals

**Goals**

- Generalize the split rule from `KEY=VALUE` to the property it is an instance of.
- Land the offline cosine harness in the repository as a re-runnable instrument.
- Prove the rule offline before spending a live A/B, then confirm live.
- Guard X02 so this decay cannot silently return.

**Non-Goals**

- Tuning ceiling/floor/hardMax. A 4x ceiling sweep already moved no graded outcome.
- Any bespoke ranker, prose classification, or salience reweighting.
- Rewording documentation to restore a score.
- GraphRAG on MCP, and tier-2 answer grading. Both are real; both are separate changes.

## Decisions

### D1. Generalize the property, rather than add a second special case

The tempting fix is a `KEY=VALUE`-style clause for shell commands. That would fix X02 and leave the
next shape — flag lists, CLI option tables, one-per-line examples — to be discovered by another
regression.

The property that actually matters is **independence**: a block whose lines are self-contained peers
carries N unrelated facts in one vector, and any single fact is a 1/N fraction of it. `KEY=VALUE` was
never the cause, only the first observed instance. So the rule becomes "independent peer entries",
with `KEY=VALUE` and whole shell commands as two recognised forms.

### D2. The discriminator is continuation, not language

The dangerous mistake is splitting a block whose lines are *not* independent — a Go function body, a
JSON document, a multi-line pipeline. Splitting those produces fragments that are individually
meaningless and would damage retrieval rather than help it.

Language tags cannot decide this: 112 of 205 blocks in the corpus carry no tag at all, and a `bash`
block may still be one continuous pipeline. The signal that survives is **structural continuation** —
indentation, trailing operators (`{`, `[`, `(`, `,`, `\`, `|`, `&&`), and here-document markers all
indicate a line that depends on its neighbours.

The rule is therefore conservative by construction: a block divides only when *no* line shows
continuation and it yields at least three groups. A block we cannot confidently call independent stays
whole, which preserves today's behaviour for the ~142 continuous blocks and accepts under-splitting
as the safe failure.

### D3. The harness is admissible because it already predicted a live outcome

An offline proxy that has never been checked against reality is a way to be confidently wrong more
cheaply. This one reproduced X02's live ordering at three corpus states, including the sign of the
margin at the crossover — and it did so only after the identity text (`document § heading`) was
prepended the way graph-embedding actually embeds. A first attempt without that prefix produced the
right *trend* and the wrong *ranking*, which is exactly the failure mode the admissibility rule
exists to catch.

So the spec requires the harness to have reproduced a known live ordering before it is quoted as
evidence, and requires the live result to win any disagreement.

### D4. Fix the splitter, not the document

Rewording `configs/tiers/README.md` would restore X02 today and leave every other command block
diluting. Worse, it would make the graded set measure the corpus's wording rather than the product's
behaviour — the scorecard's stated failure mode, where an instrument protects its own score.

The repair is therefore demonstrated on the **unmodified** document, and the spec says so.

### D5. Accept that this changes many passage boundaries

Re-chunking every peer block changes those documents' embeddings and therefore their ranking, which
is the point. The graded set is the control: `questions.json` does not change, the baseline corpus is
fixed, so the before/after is directly comparable and any band that regresses is visible.

The risk that matters is not "boundaries moved" but "a fact that used to be found alongside its
explanatory prose is now split away from it". That is measurable — it would show as a doc-band
regression — and is the specific thing the live A/B is for.

## Risks / Trade-offs

- **Over-splitting continuous code.** The worst outcome: fragments of a Go function as separate
  passages. Mitigated by D2's conservative test and pinned by a scenario requiring continuous blocks
  to stay whole. This is the risk to review hardest.
- **Losing prose context.** A command separated from the sentence explaining it may retrieve worse
  for "how do I…" questions even as it retrieves better for "what port…" questions. The doc-late band
  contains both shapes; a regression there is the signal.
- **Passage-count growth.** More passages per document means more entities and more embedding work.
  X01's equivalent change cost +7 entities (+0.13%); this one will cost more and should be recorded.
- **Harness/stack divergence.** The harness approximates what graph-embedding embeds. It matched at
  three states, but it is an approximation, and the spec makes the live result authoritative.

## Rollout Plan

1. Land the offline harness and pin its credibility with the X02 three-state reproduction.
2. Implement peer-block detection with the continuation discriminator; unit-test both directions
   (divides independent commands, keeps continuous constructs whole).
3. Score candidate rules offline across the graded set's answer/distractor pairs.
4. Live A/B on the fixed baseline corpus: re-score and compare band by band against
   `beta159-fixed-corpus.json` (22/22).
5. Add the X02 regression guard and record the result beside the existing runs.

Rollback is reverting the split rule; passage IDs are unaffected, so a rebuild restores prior
behaviour with no state migration.
