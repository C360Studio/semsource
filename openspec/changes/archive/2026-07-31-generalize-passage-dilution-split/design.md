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

### D1. The rule is fence ISOLATION, not entry grouping — and the harness overturned the first draft

This change's first draft proposed generalizing the `KEY=VALUE` rule to "independent peer entries",
so a block of shell commands would divide by command group. The harness killed it before any code was
written: **X02's block is two command lines**, and the existing `minKeyGroups = 3` threshold means
entry grouping never fires on it. The proposed fix would not have fixed the defect it was proposed
for.

What does fix it is separating the fenced block from the prose around it. Measured on the unmodified
document:

| candidate | bytes | cosine | margin vs distractor |
|---|---|---|---|
| whole section (today) | 4301 | 0.6116 | −0.0188 |
| **fence isolated** | **287** | **0.6535** | **+0.0231** |
| prose only | 4121 | 0.6153 | −0.0150 |

Isolating the block does not merely restore the pre-#109 margin (+0.0073) — it more than triples it,
because the isolated passage is almost pure signal. And the prose passage correctly still loses: it
does not contain the port.

This is the workflow the change argues for, applied to the change itself. The cost of being wrong was
one embedding call.

### D2. Two complementary rules, because isolation does not subsume grouping

Fence isolation and entry grouping solve different halves of the same problem:

- **Isolation** stops a block's facts competing with the *prose around it*. It keeps the block whole.
- **Grouping** stops a block's entries competing with *each other*. It divides within the block.

X01 needs grouping and isolation would not have fixed it: the whole `§ Configuration` key/value block
scored 0.6569 where the winning 218-byte key group scored 0.7663. X02 needs isolation and grouping
cannot reach it. So both stay, and the `KEY=VALUE` grouping rule is generalized to peer entries as a
secondary improvement rather than as the fix.

### D2b. Isolation triggers on prose volume, not on the ceiling

A fenced block is isolated when the section's non-fence content is at least the floor (400 B) — that
is, when there is enough prose present to dilute it. Below that there is nothing to separate from and
isolating would only mint tiny passages.

This is deliberately a different size test from the ceiling. The ceiling asks "is this passage too
big"; a 4x sweep of it moved no graded outcome, because dilution is about a passage's *contents*. The
floor here asks "is there competing prose", which is the thing that actually dilutes.

Continuation detection (indentation, trailing operators, here-docs) still governs *grouping*, where
splitting a continuous construct would produce meaningless fragments. Isolation never splits a block,
so it carries none of that risk.

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

### D6. Identity carries heading ancestry, because isolation's symmetric cost needs an asymmetric fix

Iteration 1 (isolation alone) fixed X02 live and regressed X01 `correct` → `MISLEADING`: isolating
`§ Quick Start`'s `git clone` fence left the port-conflict workaround blockquote as a sharper 463 B
prose passage, which then outscored the Configuration key group by 0.0009. Any further *splitting*
rule faces the same symmetry — sharpening an answer sharpens some other query's distractor.

The asymmetric lever is identity, not boundaries. The X01 answer lives under `### Configuration`
inside `## Docker Compose`, and the query names that parent ("in docker compose") — but the
splitter only carried the immediate heading, so the embedded identity read `SemSource §
Configuration` and the structural context was discarded. Passages now record the heading's full
ancestor chain, and the title spells it out: `SemSource § Docker Compose § Configuration`.
Measured: X01 0.7584 → 0.7803 (margin −0.0009 → +0.0210) with the distractor unchanged; X02
+0.0121, still positive and still rank 0 in a whole-file rank probe. No split boundary, passage ID,
or entity count changes; `DocSection` keeps the verbatim immediate heading because URL anchors
derive from it.

This stays inside the change's own constraint — structural, derivable from the document bytes, no
prose classification. The heading hierarchy was always in the document; discarding it made two
same-named facts indistinguishable.

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
