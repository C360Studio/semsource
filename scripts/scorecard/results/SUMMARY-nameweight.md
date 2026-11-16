# graph/lexical NameWeight on SemSource's entities (semengine#620)

`graph/lexical`'s `NameWeight` (SemEngine `v0.1.0-alpha.5`, `graph/lexical/index.go:17`, `const
NameWeight = 2`) multiplies the term weight of words in an entity's Name. Upstream swept it on a
frozen set whose names are file headings shared by every section, so that set could not show
whether weighting real names helps. This sweep runs it on SemSource's real entities.

**Verdict:** keep 2.
- Weight 1 is the worst on both corpora.
- Weights 2 and 3 are within one question of each other.
- Weight 5 trades a dogfood top-1 hit for top-5 recall.
- No weight changes a dogfood verdict.

The differences are small: at most one question at rank 1 per corpus.

**Date:** 2026-11-16. **Machine:** M3 Pro (arm64/Darwin). **Sets:** dogfood `questions.json` v7
(36 questions, 8,192 entities), OSH `questions-osh.json` v3 (13 questions, 29,422 entities).
These are the #230 corpora (tree hashes `b4c2cfd9026b20b5` and `9b1e77bc0bf0291f`).

## Method

- **Builds.** SemSource main `28295c7` was built once per weight against a local copy of the
  alpha.5 module, with only that constant changed (`go mod edit -replace`).
  - Weight 2 is built the same way from an unmodified copy, so every build goes through the same
    replace path. That copy's `index.go` is byte-identical to the module cache's.
  - Binary hashes are in [`nameweight/binaries.sha256`](nameweight/binaries.sha256).
  - Nothing was pushed to SemEngine.
- **Runs.** Each run uses a fresh `nats:2.14-alpine` JetStream container and the native binary
  with the #233 tier 0 config (`alpha5-tier0/tier0.json`, sources pointed at the corpus). The
  script is [`nameweight/sweep.sh`](nameweight/sweep.sh).
  - Native runs mint different entity IDs from the container runs (another source path, another
    authority). Only native-run results are compared with each other.
  - At weight 2, scores reproduce #233's: 32/36 on dogfood, 12/13 on OSH.
- **Metric.** Pass/fail is too coarse for a weight change; every dogfood weight scores 32/36. The
  extra metric is [`first-hit.sh`](../first-hit.sh): for each question with expected facts, the
  rank of the first node or `graph_search` match whose content holds one of them. The tables
  report mean reciprocal rank (MRR) and hits@1/5/20. Rank movement against weight 2 uses
  `rank-diff.sh` after [`nameweight/strip.sh`](nameweight/strip.sh) removes the per-store
  authority segment from the IDs.
- **Noise floor.** Weight 2 was repeated on fresh stores for both corpora, and weight 3 for OSH
  ([`nameweight/rep.sh`](nameweight/rep.sh)).
  - The dogfood repeat is rank-identical, 36/36. Every dogfood movement below is therefore
    caused by the weight.
  - The OSH repeats differ on 2 and 1 of 13 rankings, with identical metrics.

## Dogfood (36 questions; 32 carry expected facts)

| weight | score | MRR | hits@1 | hits@5 | hits@20 | rank-identical vs 2 |
|---|---|---|---|---|---|---|
| 1 | 32/36 | 0.676 | 18 | 27 | 30 | 12/36 |
| **2** | 32/36 | **0.705** | **19** | 27 | 30 | (repeat: 36/36) |
| 3 | 32/36 | 0.700 | 19 | 27 | 30 | 13/36 |
| 5 | 32/36 | 0.686 | 18 | **29** | 30 | 11/36 |

Weight changes reorder most word-index rankings: the doc, loose, code and lexical bands
move between 20 and 22 questions. The structural tools (`code_context`, `code_impact`) don't
move. The movement shifts which passage comes first, but it doesn't change a verdict.

## OSH (13 questions; 11 carry expected facts)

| weight | score | MRR | hits@1 | hits@5 | hits@20 | rank-identical vs 2 |
|---|---|---|---|---|---|---|
| 1 | 12/13 | 0.841 | 9 | 10 | 10 | 6/13 |
| **2** | 12/13 | 0.848 | 9 | 10 | 10 | (repeat: 11/13) |
| 3 | **13/13** | 0.853 | 9 | 10 | **11** | 6/13 (repeat: 12/13) |
| 5 | **13/13** | **0.854** | 9 | 10 | **11** | 6/13 |

At weights 3 and 5, C03 (`code_search`) passes; at weights 1 and 2 no returned node holds the
expected fact. That is the only OSH score change. At weight 3 the first hit lands at rank 20, the
last of the 20 slots returned, in both runs, so one more competing name would push it out. At
weight 5 it lands at rank 16.

## Reading it

- **Down-weighting names (1) is worse on both corpora.** It loses a rank-1 hit on dogfood and has
  the lowest MRR on each. On real entity names, a Name word is better evidence than a Body word.
  That is the effect upstream's heading-name set could not show.
- **Weights 2 and 3 are a wash.** On dogfood, 3 moves two first hits down (D02 3 → 5, L03 6 → 7)
  and none up; hits@k are unchanged. On OSH, 3 gains C03 at rank 20.
- **Weight 5 over-weights names.** On dogfood it loses a rank-1 hit (D04 1 → 3) and moves D02
  down (3 → 4), while lifting L01 (8 → 5) and L03 (6 → 4) into the top 5, so hits@5 goes 27 → 29.
  On OSH it gains C03 at rank 16. That is a precision-for-recall trade, not a clear improvement.

No weight dominates 2 on both corpora: 3 is slightly worse on dogfood for one last-slot hit on
OSH, and 5 lowers dogfood MRR (0.705 → 0.686) and top-1 for top-5 recall. The measured case
supports keeping 2. The measurement has a scope limit: 43 graded questions on two corpora, at one
machine and one SemEngine revision. Per-question ranks come from `first-hit.sh`'s `ranks` field.

## Replay

```sh
# From this checkout, with the #230 corpora at /tmp/base230/corpus-{dogfood,osh}
set -euo pipefail
m=$(go list -m -f '{{.Dir}}' github.com/c360studio/semengine)
mkdir -p /tmp/nw
for w in 1 2 3 5; do
  cp -R "$m" /tmp/nw/semengine-w$w && chmod -R u+w /tmp/nw/semengine-w$w
  sed -i '' "s/^const NameWeight = 2$/const NameWeight = $w/" /tmp/nw/semengine-w$w/graph/lexical/index.go
  grep -q "^const NameWeight = $w$" /tmp/nw/semengine-w$w/graph/lexical/index.go
  d=/tmp/nw/src-w$w; mkdir -p $d && git archive 28295c7 | tar -x -C $d
  (cd $d && go mod edit -replace github.com/c360studio/semengine=/tmp/nw/semengine-w$w \
     && go build -o /tmp/nw/semsource-w$w ./cmd/semsource)
done
scripts/scorecard/results/nameweight/sweep.sh "$PWD"
RUNS="dogfood:2 osh:2 osh:3" scripts/scorecard/results/nameweight/rep.sh "$PWD"
scripts/scorecard/first-hit.sh scripts/scorecard/questions.json scripts/scorecard/results/nw-w2-dogfood.json
```
