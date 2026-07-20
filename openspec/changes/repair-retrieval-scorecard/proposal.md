# The scorecard cannot currently measure retrieval

## Why

`retrieval-ranking` requires that passage retrieval be **verified against the graded
interrogation set rather than asserted**. The instrument that requirement depends on —
`scripts/scorecard/` — is currently unable to discharge it. Measured on a live stack on
2026-07-20, its discrimination band reports failures that are not retrieval failures, and
its results depend on the order questions are asked in.

The band's recorded result, `discrimination 0/2` at all three bounds in
`results/SUMMARY-9.5-bounds.md`, is therefore **not a retrieval result**. It was read as
one, and a change was proposed against it (`fix-default-vs-override-ranking`) whose
headline evidence came from that reading.

Two independent defects, both reproduced:

**1. A matcher that fails on every system, forever.** `run.sh`'s five matcher loops call
`grep -qF "$w"` with no `--` terminator. X02's expected value is `-p 8083:8083`, so `grep`
parses it as options and exits 2. The loop treats any non-zero exit as "not found", so X02
grades `miss` regardless of what retrieval returned.

X02 in fact **retrieves correctly today**: `graph.query.semantic` ranks the Tier 2
(`seminstruct`) section first at 0.6801, stable across 6 identical calls, and `doc_context`
returns it as node 1 carrying `-p 8083:8083` and not the twin `-p 8081:8081`. It should
grade `correct`.

This is exactly the failure mode the scorecard README already warns about — a question that
"would have reported IMPRECISE on every system forever, hiding real regressions behind a
constant failure" — arriving through a mechanism the README's own checker does not cover.
`check-discrimination.py` validates the *corpus* relationship between a pair; it does not
validate that the grader can match the literals.

**2. Results depend on question order.** With the full 22-question set, X02 asked **last**
loses the correct passage from the **entire 20-node response** (top node becomes
`ADR-0002 § Service Choices`, 575 B). The same question asked **first** in the same set
returns the correct 831 B node. Doc-only and code-only subsets both return it correctly.

It is not call count: 24 identical `doc_context` calls in one session all returned the
correct node. It is query *diversity*. The recall layer is not the cause —
`graph.query.semantic` returns an identical, correctly-ordered candidate list across
repeated calls — so the passage is lost downstream of recall, inside fusion's hydration or
budget stage.

An instrument whose verdict depends on what was asked before it cannot support an A/B. Both
sides of every comparison this project has run were scored with it.

## What Changes

- **Fix the matchers.** Terminate option parsing (`grep -qF -- "$w"`) in all five matcher
  loops, so an expected or forbidden value may begin with `-`. **This changes recorded
  outcomes**: X02 is expected to move `miss` → `correct` with no product change.
- **Extend `check-discrimination.py` to gate the grader, not just the corpus.** A question
  whose literals the matcher cannot evaluate must fail validation before it is ever scored.
  The current checker would have passed X02 indefinitely.
- **Root-cause the order dependence**, and either fix it in SemSource or — if it is
  substrate, which the evidence currently suggests — reduce it to a minimal reproduction,
  file it per the Product Boundary, and make the instrument robust to it in the meantime.
  A scorecard that cannot be trusted across question orders is not fixed by re-ordering the
  questions; the design must say plainly which of the two it is delivering.
- **Add the third verdict.** A top node carrying the confusable twin **but not** the answer
  is graded a plain `miss` today, though it is worse than absent: it argues for the wrong
  answer. It gets its own verdict alongside `correct`, `IMPRECISE` and `FABRICATED`.
  **BREAKING** for comparability: bumps `questions.json` to **version 3**; version 2 results
  do not compare across it.
- **Re-baseline.** Record a version-3 baseline on the fixed instrument, and annotate
  `SUMMARY-9.5-bounds.md` so its `0/2` is not quoted again as a retrieval finding.

## Capabilities

### New Capabilities

None. This change repairs the instrument that an existing requirement already depends on.

### Modified Capabilities

- `retrieval-ranking`: the existing requirement *"Passage-level retrieval measurably improves
  answer precision"* obliges verification against the graded set but says nothing about the
  set being **able** to render a correct verdict. Add requirements that a graded verdict
  reflects retrieval and nothing else — matchers must evaluate their literals rather than
  erroring, and a question's verdict must not depend on its position in the run.

## Impact

- `scripts/scorecard/run.sh` — matcher loops, verdict set, summary output.
- `scripts/scorecard/questions.json` — version 2 → 3.
- `scripts/scorecard/check-discrimination.py` — new grader-evaluability gate.
- `scripts/scorecard/README.md` — the four-verdict section becomes five; comparability note.
- `scripts/scorecard/results/SUMMARY-9.5-bounds.md` — annotated, not rewritten; its bounds
  conclusion (a 4x ceiling range changed no graded outcome) is unaffected and stands.
- Possibly `docs/upstream/semstreams-asks.md` — if the order dependence is substrate.
- No product code is expected to change. If the order dependence turns out to be
  SemSource's, that expectation is void and the design says so.

## Non-goals

- **Fixing the default-vs-override ranking defect.** That is
  `fix-default-vs-override-ranking`, which this change unblocks. Its root cause is already
  measured (embedding dilution: the answer's own line scores 0.8133 against the query while
  the same fact inside its 1363-byte env-var block scores 0.6569, losing to a 0.7323
  distractor). Fixing it is out of scope here; being able to *prove* the fix is the point of
  this change.
- **Reimplementing fusion ranking.** Substrate, per the Product Boundary. If the order
  dependence is the engine's, the outcome is a GitHub issue and an entry in
  `docs/upstream/semstreams-asks.md` — never a PR to semstreams.
- **Widening the discrimination band.** An automated sweep found only one usable confusable
  pair in this corpus beyond the two shipped. Growing the band needs a different corpus and
  is separate work.
- **Re-tuning passage bounds.** Settled in `doc-passage-chunking` and untouched here.
- **Replacing deterministic grading with an LLM judge.** A drifting judge cannot support an
  A/B; this change makes the deterministic grader correct rather than abandoning it.

## Consumers

The scorecard is an in-repo instrument, not a shipped surface, so it has no runtime
consumers. Its output is what the project uses to make retrieval claims about
`code_context` / `doc_context` — the surfaces consumed by **SemSpec** (whose hard e2e prompt
is the project's canary), **SemTeams**, and any agent driving SemSource over MCP. The
practical consumer of this change is `fix-default-vs-override-ranking`, which cannot be
verified until it lands.
