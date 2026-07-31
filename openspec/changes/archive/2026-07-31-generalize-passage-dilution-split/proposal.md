## Why

SemSource's splitter has exactly one size-independent split rule, and it fires only on a fenced block
that is a homogeneous `KEY=VALUE` list. Counted over the current corpus of 205 fenced blocks, **4
qualify.** A conservative classification — at least three unindented lines with no continuation —
puts roughly **63** blocks in the independent-peer shape the rule was written for, most of them
`bash`. The remaining ~142 are continuous or structured (`go`, `json`, multi-line constructs) and
must stay whole.

So the rule reaches about 4 of ~63 blocks whose facts it was meant to protect. The other ~59 dilute
freely.

The gap is measured, not inferred. Graded question X02 asks which port the seminstruct container
publishes. Its answer lives in a fenced `docker run` block, which is not `KEY=VALUE`, inside a section
that has stayed just under the 2000-byte ceiling — so it falls through the homogeneity gate and the
size gate both. As prose accumulated around that command across two ordinary documentation edits, the
passage grew 839 B → 1964 B → 1943 B and its cosine against the query fell 0.6481 → 0.6386 → 0.6116
while an unrelated ADR passage held constant at 0.6407. The answer crossed below the distractor at
the first edit and X02 has been a `miss` ever since. Nothing outranked the answer; the answer
dissolved.

That decay is invisible to every gate the project currently runs. It is not a compile error, not a
test failure, and not a change to the retrieval code — it is what ordinary documentation growth does
to a passage whose splitter cannot subdivide it.

Two edits to one file moved a graded verdict. The same mechanism applies to every command, code
sample, table, and flag list in the corpus, and it gets worse as documentation matures.

## What Changes

- **BREAKING**: add a second size-independent split rule — fence ISOLATION. A fenced block that is
  not a homogeneous key/value list becomes its own passage whenever the section's non-fence content
  is at least the floor, so a command or code sample no longer competes with the prose around it.
  The block is never divided; `KEY=VALUE` grouping is retained unchanged. (The first draft proposed
  dividing any "independent peer" block by entry group; the offline harness killed that before code
  was written — see design D1.)
- Passage titles carry the heading's full ancestor chain (`SemSource § Docker Compose §
  Configuration`, not `SemSource § Configuration`). The title is the identity text retrieval ranks
  by; the ancestry is document structure the splitter previously discarded, and it is what lets a
  canonical section beat a workaround note for a query that names the parent section. `DocSection`
  keeps the verbatim immediate heading, so URL anchors are unaffected (see design D6).
- Land the offline cosine harness as a first-class instrument under `handler/doc`, alongside the
  existing `TestBoundsSweep`. It splits candidate documents with the real splitter, embeds passages
  through semembed with the arctic-embed query prefix applied to the query only, and reports each
  candidate's margin against a named distractor.
- **Require a candidate split rule to be scored offline before any live A/B.** The harness reproduced
  X02's live ordering exactly — including the sign of the margin at each of three corpus states —
  which is what makes it admissible. A rule that cannot be shown to move the margin offline does not
  earn a stack rebuild.
- Add a regression guard pinning X02's answer passage above the ADR-0002 distractor, so this specific
  decay cannot silently return.
- Re-score the graded set on the fixed baseline corpus after the change and record the result beside
  the existing runs.

## Non-goals

- Changing the ceiling, floor, or hard max. The sweep already showed a 4x ceiling sweep moves no
  graded outcome, because dilution is a property of a passage's contents rather than its size.
- Any bespoke ranker, prose classification, or salience reweighting. `retrieval-ranking` is explicit
  that ranking is essentially the embedding's cosine order and that what SemSource controls is the
  body text it emits; this change stays on that side of the line.
- Editing documentation to work around the defect. Rewording `configs/tiers/README.md` would restore
  X02 and leave the mechanism intact for every other block in the corpus.
- GraphRAG on the MCP surface. That is a real gap with an owner ruling behind it, but it is a
  product-surface change touching the ADR-0004 boundary and belongs in its own change.
- Tier-2 / GraphRAG answer-quality grading. It needs a separate instrument with a judge, and the
  scorecard's own rationale explains why a judge must not join the deterministic set.

## Consumers

Every MCP and HTTP consumer of `doc_context` and `code_search` — the agent path — reads the passages
this splitter emits. SemSpec, SemDragon, and SemOps consume the same retrieval surface. No outward
contract changes; passage *identity* is unaffected because IDs derive from parent path and ordinal.

## Capabilities

### Modified Capabilities

- `doc-passage-chunking`: add fence isolation as a second size-independent split reason (a fenced
  block is separated whole from diluting prose), and qualify passage titles with the heading's full
  ancestor chain while `DocSection` keeps the verbatim immediate heading.
- `retrieval-ranking`: extend the anti-dilution guarantee beyond homogeneous lists to any passage
  whose specific fact competes with unrelated prose, and require that a candidate change to the
  emitted body text be scored offline against a named distractor before a live A/B.

## Impact

Affects `handler/doc/splitter.go` and its tests, adds an offline harness under `handler/doc`, and
changes passage boundaries for documents containing command or code blocks — which changes their
embeddings and therefore their ranking. Passage IDs, parent linkage, and the byte-for-byte tiling
guarantee are unaffected. Existing graded results remain comparable because `questions.json` does not
change.
