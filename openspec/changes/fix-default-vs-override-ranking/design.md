# Design — making the default outrank the workaround

## Context

A query for a canonical value ranks the section that overrides it above the section that defines
it. `proposal.md` carries the measurement; this document decides the mechanism.

Three things are already settled and are not reopened here:

- **The cause is embedding dilution.** The answer's own line scores **0.8133** against the query;
  the same fact inside its 1363-byte environment-variable block scores **0.6569** and loses to a
  **0.7323** distractor. Control validated — offline re-embedding reproduced the live cosines
  exactly.
- **Predicate salience cannot fix it.** Recall order spans 160 points, the gap is 24, and the
  doc-side salience range is `[-6, 0]`. The substrate states these weights are deliberately small
  so they "break ties … rather than dominating the graph's own ranking".
- **`doc_context` ranking is essentially pure cosine order** (`lexicalScore` is always 0 for a
  natural-language query), and for offloaded entities **only body bytes are embedded** — titles are
  excluded (upstream ask #21). So the body text SemSource emits is the entire lever.

### Why the existing pipeline cannot already do this

`splitPassages` subdivides a section **only when it exceeds the ceiling** (`splitOversized`, via
`subdivide`). § Configuration is **1363 bytes — under the 2000 ceiling**, so it is never a
candidate for subdivision at all. That is the mechanical reason the 9.5 bounds A/B moved nothing:
across 1000, 2000 and 4000 the block was never oversized, so no ceiling could have split it.

Two existing invariants constrain any fix, and both are enforced by tests:

1. **Passages tile the document.** Bodies concatenate in ordinal order to reproduce the input byte
   for byte (`splitter_test.go:301`, `passage_test.go:514`). Nothing may be duplicated or dropped.
2. **A fenced block is preserved whole** up to the hard max, on the stated grounds that "splitting
   code hurts retrieval more than an oversized passage does" (`splitOversized`).

Invariant 2 is what this change narrows, and the measurement is what earns the right to: that
claim is true for *code* and false for a *list of configuration keys*. A reader of one env var does
not need the other ten, and the vector proves it — isolating the line gains 0.156 cosine.

## Goals / Non-Goals

**Goals:**

- A query for a canonical value prefers the passage that defines it.
- The split trigger is **structural**, derivable from the bytes, and never a judgement about what
  prose means.
- No regression in the saturated doc bands.

**Non-Goals:**

- Re-tuning ceiling/floor/hardMax. Settled; untouched.
- Splitting code fences generally. Only homogeneous key/value lists.
- Prose classification of "canonical vs workaround". The signal here is block homogeneity, not
  rhetoric — which is what makes it safe.
- Fixing upstream #597, or waiting for it. X01 is stable across every position measured, and the
  v3 baseline graded it `MISLEADING` rather than `UNSTABLE`, meaning all three repeats agreed.

## Decisions

### D1 — Split on homogeneity, independently of size

Add a second reason to subdivide. Today the only trigger is "bigger than the ceiling"; this adds
"the block is a homogeneous list of independent facts", which applies at any size.

This is the crux. A size-triggered splitter cannot fix a 1363-byte block no matter how the bounds
are tuned, and every measurement in the bounds A/B is consistent with that.

*Alternative rejected:* lower the ceiling until the block splits. It would shred ordinary prose
corpus-wide to fix one shape of content, and the 9.5 A/B already showed finer bounds buy nothing on
the doc bands while costing evidence size.

### D2 — Scope to fenced blocks of `KEY=VALUE` lines, and say so

The trigger fires only when a fenced block is **predominantly `KEY=VALUE` lines** (a simple
threshold on the share of non-blank lines matching `^[A-Za-z_][A-Za-z0-9_]*=`). Markdown tables,
YAML, JSON and prose are out of scope.

Narrow on purpose. "Structured block" sounds general and would invite the splitter to guess at
table semantics and YAML nesting; `KEY=VALUE` is the shape actually measured, actually common in
this corpus's documentation, and unambiguous to detect. Widening it later is a change with its own
evidence, not an assumption smuggled in now.

### D3 — Group by lexical key prefix, not by meaning

Within a qualifying block, consecutive lines are grouped by the key's leading token up to the first
underscore: `NATS_URL`, `NATS_HOST_PORT`, `NATS_MONITOR_HOST_PORT` → one group.

Purely lexical. No dictionary, no model, no notion of what NATS *is*. That is what keeps this on
the right side of the "no prose classification" non-goal while still producing topical groups.

It is also the variant that was measured: heading plus the three `NATS_*` lines is **236 bytes at
0.7783**, comfortably past the 0.7323 distractor. Per-key atoms score marginally higher
(**0.8133**) but a 70-byte passage is thin evidence, and the grouped form already wins.

*Alternative rejected:* group on blank lines within the fence. This block has none, so it would
have been a no-op here — and formatting whitespace is a weaker signal than the key names
themselves.

### D4 — Do not repeat the section heading in each sub-passage

Tempting, and wrong twice over. It would break the tiling invariant (D-context 1) by duplicating
bytes, and the measurement says it buys nothing: heading-plus-line scores **0.8127** against the
bare line's **0.8133**. The heading contributes no measurable retrieval value here.

So sub-passages carry only their own bytes, tiling is preserved, and the fence markers land with
the first and last groups. Attribution is unaffected — a passage already identifies its parent
document and section through `DocSection` and the `CodeBelongs` edge.

This matters more than it looks: upstream #21 means the title is excluded from the vector anyway,
so putting the heading in the body would have been the only way to get it embedded — and it is not
worth breaking a tested invariant for a 0.0006 cosine difference.

### D5 — Exempt homogeneity groups from the floor merge — ~~required~~ **UNNECESSARY, corrected during implementation**

**The reasoning below was wrong about this codebase, and the exemption was not implemented.**

The concern was real in the abstract: every group this change produces is below the 400-byte floor,
so a merge pass that saw them would reassemble the block and make the change a silent no-op. That
would have been invisible — the detection tests would still pass.

It cannot happen here. `mergeSmallSections` operates on `[]section` and runs **before** `subdivide`
(`splitPassagesBounded`); the groups are created *inside* `subdivide`, after the merge has already
run. The floor merges sections, never spans within one. There is nothing to exempt.

**What was kept anyway:** the test the decision called for
(`TestKeyGroupsSurviveTheFloorMerge`). It asserts end-to-end through `splitPassages` that the NATS
group and the SEMSOURCE group land in different passages. That is no longer guarding against the
floor specifically — it guards against any future reordering that puts a merge pass after
subdivision, which would silently reintroduce exactly this failure.

Recorded rather than quietly dropped: a design that predicts a hazard the architecture already
prevents is worth correcting in place, so the next reader does not add the exemption believing it
is load-bearing.

### D6 — Gate the trigger so most documents are untouched

The split fires only when a qualifying block yields **at least three distinct prefix groups**. A
block of two related keys is not diluted in any way this change can measure, and splitting it would
add entities for nothing.

The gate bounds both entity growth and the blast radius of the re-ingest D7 forces.

### D7 — Accept identity churn; this needs a rebuild, not a reindex

Passage identity is `(path, ordinal)`. Inserting groups renumbers every later passage in an
affected document, so their entity IDs change. Combined with the substrate's inability to clear a
stored body reference in place, that means **a graph rebuild**, exactly as `doc-passage-chunking`
required.

Not hidden in the implementation: it goes in the migration note, because a consumer running an
in-place reindex would otherwise keep stale passages alongside new ones.

### D8 — Verification: one clean signal, one regression detector

- **The signal.** X01 moves `MISLEADING` → `correct`. It is stable — 711-byte top node in all seven
  runs across three positions, two graders and two binaries — and the v3 grader certified it by
  returning `MISLEADING` rather than `UNSTABLE` with `repeats=3`.
- **The regression detector.** The doc bands are saturated at 10/10, so they cannot show
  improvement; their job is to catch prose being shredded. A drop there kills the change.
- **The cost.** Entity count and time-to-ready recorded against the v3 baseline, per the existing
  `retrieval-ranking` requirement that corpus growth is measured rather than assumed.
- **X02 is not a signal here.** It may move as a side effect of re-chunking, and it may stay
  `UNSTABLE` while #597 is open. It will not be read either way.

Both sides use the fixed corpus (`git archive d554bcc`, `scripts/scorecard/` excluded) and
questions.json v3. The baseline side is already recorded (`results/v3-baseline.json`).

## Risks / Trade-offs

- **[A dominant prefix stays diluted]** → Eight `SEMSOURCE_*` keys would group into one passage
  that is still broad. Accepted: the fix is proportionate, not total, and the measured case
  resolves. A per-key split would address it at the cost of 70-byte evidence, and is a later change
  with its own evidence if a query ever demonstrates the need.
- **[Entity growth from many small passages]** → Bounded by D6's three-group gate and measured per
  D8. If growth is disproportionate, the gate threshold is the knob, not the mechanism.
- **[The tiling invariant breaks]** → Directly tested already; the change must keep those tests
  passing unmodified. If a proposed implementation requires editing the tiling test, that is the
  signal it is wrong, not that the test is.
- **[The floor exemption leaks]** → If marking spreads beyond homogeneity groups it would defeat
  the floor generally and mint passages per heading again. Confine the marker to spans D1 produced
  and pin it with a test.
- **[Splitting a fence produces odd-looking evidence]** → A middle group carries no fence markers.
  Acceptable: it is still verbatim source, correctly attributed. The alternative — duplicating
  fence lines — breaks tiling.
- **[Re-chunking moves something else]** → That is what the doc bands and the recorded baseline are
  for. This is why the scorecard was repaired first.

## Migration Plan

Rebuild, not reindex (D7). Same procedure `doc-passage-chunking` documented:
`docker compose down -v`, bring up on the new binary, wait for `phase: ready` **and**
`index.ready` **and** `embedding.ready`. No config change; no consumer contract changes — passage
identity, `DocSection`, and the parent edge are all unchanged in shape.

Rollback is `git revert` plus the same rebuild.

## Open Questions

- **Should the trigger extend to unfenced `KEY=VALUE` runs?** This corpus's cases are all fenced,
  so the narrow rule is enough to test the hypothesis. Deferred deliberately — widening it should
  follow a measurement, not precede one.
- **Is three groups the right gate (D6)?** Chosen to exclude trivial blocks while catching the
  measured one (five groups). Worth confirming against the entity-count delta rather than argued.
- **Does the fenced-blocks-are-atomic rule deserve revisiting for real code?** The measurement here
  says nothing about Go or shell bodies; the original rationale may well hold for them. Out of
  scope, noted so the narrowing is not later mistaken for a general finding.
