# A/B — fence isolation + heading-path identity (generalize-passage-dilution-split, iteration 2)

**Date:** 2026-07-31. **questions.json version 3**, `SCORECARD_REPEATS=3`. This summary does not
compare across question-set versions. Two runs, both with the iteration-2 binary (fence isolation
from commit `a05c1c3` **plus** heading-path passage titles); `docker compose down -v` between
corpora.

- `headingpath-fixed-corpus.json` — corpus `git archive d554bcc`, 206 ingestable documents,
  `scripts/scorecard/` excluded. Compares against `beta159-fixed-corpus.json` (22/22).
- `headingpath-current-main.json` — corpus `git archive 6edf3de` (main), `scripts/scorecard/`
  excluded. Compares against `beta159-current-main` (21/22, X02 `miss`).

## Result — both corpora at 22/22

| | beta159 fixed (baseline) | **headingpath fixed** | beta159 current-main | **headingpath current-main** |
|---|---|---|---|---|
| score | 22/22 | **22/22** | 21/22 | **22/22** |
| X01 | `correct` | **`correct`**, top node 218 B | `correct` | **`correct`**, top node 218 B |
| X02 | `correct` | **`correct`**, top node 180 B | `miss` | **`correct`**, top node 180 B |
| MISLEADING / imprecise | 0 / 0 | **0 / 0** | 0 / 0 | **0 / 0** |
| entities | 5417 | 5623 (+3.8%) | 5504 | 5716 (+3.9%) |
| median top-node body (doc bands) | — | 609 B | — | 609 B |

**On the fixed corpus the run is verdict-identical to the 22/22 baseline** — no verdict changed in
any band. On the current-main corpus, **X02 moved `miss` → `correct` on the unmodified document**,
which is the defect this change exists to fix. Entity growth is the isolation cost recorded at
iteration 1; heading-path identity adds no entities, no passage IDs change, and no split boundary
moves.

## What iteration 2 changed, and why iteration 1 alone failed

Iteration 1 (isolation only) graded **21/22** on the fixed corpus: X01 fell `correct` →
`MISLEADING`. The whole-file rank probe located the real competitor — not the isolated fence, but
the **463 B prose passage left behind** when the `git clone` fence was isolated out of
`§ Quick Start`: the port-conflict workaround blockquote, now undiluted, at cosine 0.7594 against
the answer's 0.7584.

The fix is identity, not boundaries. X01's answer lives under `### Configuration` inside
`## Docker Compose`, and the query names that parent ("in docker compose") — but passage titles
carried only the immediate heading, so the embedded identity read `SemSource § Configuration`.
Passages now record the heading's full ancestor chain and the title spells it out:
`SemSource § Docker Compose § Configuration`.

Offline, before the live runs: X01 answer 0.7584 → **0.7803** (margin −0.0009 → **+0.0210**),
distractor unchanged; X02 margin **+0.0121**, still positive; both answers rank 0 across every
passage of their battleground files; the harness credibility pin still reproduces the historical
live ordering with unchanged signs. The prediction held from cosine to stack, on both corpora.

## Traps this run respected

- Corpus counts verified before ingest (206 md fixed / 229 md main, `scripts/scorecard/` removed).
- `test-matcher.sh` and `check-discrimination.py` passed before either run.
- Readiness gated on `phase` + `index.ready` + `embedding.ready`, not phase alone.
- Distractor passages selected by literal, never by heading (a heading can name several passages
  once a section splits).
