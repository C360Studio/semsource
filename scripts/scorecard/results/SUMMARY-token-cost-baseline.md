# Token-cost baseline — first three-arm run (2026-08-09)

First run of the cost dimension added by the `scorecard-token-cost-arms` change
(#130). One stack, one corpus, one `questions.json` (v3), all three arms.

**Setup.** Stack built from `feat/scorecard-token-cost-arms` (main @ `4fcea34` +
harness commits, SemStreams `v1.0.0-beta.159`), isolated Compose project, corpus
= `git archive` of the same commit with `scripts/scorecard/` removed, 6,226
entities, `down -v` fresh. Arm B ran with repeats=3 (zero UNSTABLE). Arm C ran
cosine over the product's own 6,226 `EMBEDDING_INDEX` vectors, top-20, with a
`graph.query.>` subscriber attached for the whole run: **zero messages** — the
ranking path provably never touched the graph query surface.

## The table

| band | A: grep-and-read | B: MCP surface | C: cosine-only |
|---|---|---|---|
| code | 6/6 — 186,995 B | 6/6 — 67,596 B | 6/6 — 101,752 B |
| discrimination | 0/2 — 50,490 B | 2/2 — 38,275 B | 2/2 — 30,209 B |
| doc-early | 3/3 — 99,212 B | 3/3 — 49,571 B | 3/3 — 41,744 B |
| doc-late | 5/7 — 423,778 B | 7/7 — 162,420 B | 7/7 — 117,328 B |
| impact | 2/2 — 42,277 B | **2/2 — 4,572 B** | 2/2 — 32,315 B |
| negative | 2/2 — 45,463 B | 2/2 — 21,006 B | 2/2 — 26,240 B |
| **TOTAL** | **18/22 — 848,215 B (~212k tok)** | **22/22 — 343,440 B (~86k tok)** | **22/22 — 349,588 B (~87k tok)** |

Session-fixed schema overhead (B only): **7,087 B (~1,771 tok), 9 tools** —
reported separately, never folded into per-question figures.

## Findings

1. **B dominates A on every axis.** Full recall (22/22 vs 18/22), 2.5× cheaper
   overall, and the discrimination band flips 0/2 → 2/2: whole-file reading
   carries the confusable twin by construction; passage retrieval's top evidence
   answers alone. The grep floor's misses were honest (D04/D05's port facts sat
   in files its term ranking never opened).
2. **Break-even is immediate.** B saves ~23 KB of context per question against
   the grep floor (mean 15.6 KB vs 38.6 KB); the 7.1 KB schema registration pays
   for itself before the first question completes. There is no session length at
   which grep-and-read is the cheaper strategy on this corpus.
3. **Impact is the graph's signature band.** 4,572 B for both impact questions —
   9× cheaper than the grep floor and 7× cheaper than cosine, at equal recall.
   Structural questions ("what depends on X?") are where a queryable graph is
   irreplaceable: cosine has to haul twenty bodies to luck into the answer;
   the graph returns the dependents.
4. **Cosine alone matches the full surface on this set — read that carefully.**
   22/22 at essentially equal total cost is a real result, and the honest
   reading cuts three ways: (a) on doc bands it was predicted — `doc_context`
   ranking is nearly pure cosine after recall, so C-vs-B measures the recall
   stage and salience terms, which this set does not stress; (b) on code and
   impact, B is 1.5–7× cheaper at equal recall — the structural machinery earns
   its keep in cost, not recall, here; (c) the set has no question that
   *requires* multi-hop structure or salience to answer at all — C's parity is
   as much a statement about the question set as about the systems. A follow-up
   set that stresses composition (multi-entity questions, relationship-chains)
   would separate them on recall, not just cost.

## Caveats — do not quote past these

- This set was **designed to validate this product's retrieval claims**
  (chunking bands, discrimination pairs). It is an internal instrument, not a
  neutral benchmark; cross-tool comparisons of these figures are meaningless.
- Arm C's cost scales with its fixed top-20; a tuned K would shift its column
  arbitrarily. Its role is isolating the query layer, not being a product.
- Arm A charges whole files (bias direction: inflates A's cost; per-file bytes
  are in the results JSON for deriving a window-charged variant).
- Bytes are measured; tokens are `bytes/4` estimates. n=22, one corpus, one
  machine (M3 Pro, arm64).
