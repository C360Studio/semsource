# Scorecard instrument diagnosis — the discrimination band was never measuring retrieval

**Date:** 2026-07-20. **questions.json version 2.** Corpus held fixed: `git archive d554bcc`,
206 ingestable documents, `scripts/scorecard/` excluded. Stack:
`COMPOSE_PROJECT_NAME=ranktest`, `configs/mvp.json` (semembed `arctic-embed-s`,
`query_prefix` set), 5410 entities, gated on `phase` + `index.ready` + `embedding.ready`.
**One binary throughout** — nothing here is a product change.

This diagnosis was run to settle a question `SUMMARY-9.5-bounds.md` left open: *why does the
discrimination band score 0/2 at every ceiling?* The answer is that, for one of the two
questions, it was never scoring retrieval at all.

## Result

| run | grader | X02 position | X02 verdict | discrimination |
|---|---|---|---|---|
| `diagnostic-baseline` | as shipped | last | `miss` (top=575) | 0/2 |
| `diagnostic-rerun` | as shipped | last | `miss` (top=575) | 0/2 |
| `x02-alone` | as shipped | only question | `miss` (top=**831**) | 0/1 |
| `x02-first` | as shipped | first | `miss` (top=**831**) | 0/2 |
| `x02-grepfix` | **fixed** | only question | **`correct`** | 1/1 |
| `full-grepfix-x02first` | **fixed** | first | **`correct`** | **1/2** |
| `full-grepfix` | **fixed** | last | `miss` (top=**0**) | 0/2 |

Two independent defects, separable by exactly these runs.

## Defect 1 — the matcher cannot evaluate X02's literals

`run.sh`'s five matcher loops call `grep -qF "$w"` with no `--` terminator. X02's expected
value is `-p 8083:8083`, so `grep` parses it as options and exits **2**. The loops treat any
non-zero exit as "not found", so **X02 grades `miss` regardless of what retrieval returned**.

```
$ printf 'docker run -d -p 8083:8083 foo\n' | grep -qF "-p 8083:8083"; echo $?
grep: invalid option - 8083:8083
2
$ printf 'docker run -d -p 8083:8083 foo\n' | grep -qF -- "-p 8083:8083"; echo $?
0
```

X02 **retrieves correctly today.** `graph.query.semantic` ranks the Tier 2 (`seminstruct`)
section first at 0.6801 — stable across 6 identical calls — and `doc_context` returns it as
node 1 carrying `-p 8083:8083` and not the twin `-p 8081:8081`. Adding `--` flips it to
`correct` with no product change (`x02-grepfix`, `full-grepfix-x02first`).

This is the failure mode the README already warns about — a question that reports the same
verdict "on every system forever, hiding real regressions behind a constant failure" — reached
through a mechanism `check-discrimination.py` does not cover. That checker validates the
*corpus* relationship between a confusable pair; it never checks that the grader can match the
literals.

## Defect 2 — a question's verdict depends on its position in the run

With the grader fixed, the **same question against the same stack** gives opposite verdicts
depending only on where it sits in the run:

- **X02 first** in the 22-question set → `correct`, top node 831 B (Tier 2 section).
- **X02 last** in the same set → `miss`, and the correct passage is absent from the **entire
  20-node response**.

It is not call count: **24 identical `doc_context` calls in one session all returned the
correct node.** Doc-only (`x02-after-doc`) and code-only (`x02-after-code`) prefixes both
return it correctly. It tracks query *diversity*.

**The failure mode is not even stable.** Across runs the wrong top node was, variously,
`ADR-0002 § Service Choices` (575 B) and a node with **no body at all** (`top=0`).

**Recall is not the cause.** `graph.query.semantic` returns byte-identical ordering across
repeated calls — including at exact cosine ties (two entities at 0.6124 keep their relative
order across 5 runs), so unstable sorting over tied scores is ruled out. The passage is lost
**downstream of recall**, inside fusion's hydrate → rank → budget path, which exposes **no
score, rank, or reason** on any surface (`fusion.Node` has no score field, `engine_lens.go` has
no logging, and `fusionnats.resolveSemantic` discards the similarity it receives).

## A product defect surfaced in passing — body-less dependency nodes lead the answer

In the `full-grepfix` run, `doc_context`'s **top node was `github.com/containerd/platforms`**:
a `c360.semsource.config.workspace.dependency.*` Go-module entity with **no `body` field**,
class `DesignativeInformationContentEntity`. The caller's first citation is empty.

`dropNavigationalNodes` (`processor/code-context/component.go:318`) filters only
`Kind=="document" && Body==""`. A config dependency entity is neither, so it survives — and
`{org}.semsource.config` is half of `doc_context`'s default scope, so these compete on every
doc query. This is the body-less-lead defect that change 10.3 addressed, resurfacing through a
class the guard never covered. **This one is SemSource's, not substrate's.**

It also means the method note in `SUMMARY-9.5-bounds.md` — which dismissed an observed
body-less top node as "a mis-parse of the SSE stream picking up a non-result frame" — was
probably wrong. The node above was parsed from a well-formed response in a run whose other 21
questions parsed correctly.

## What this invalidates, and what it does not

**Invalidated:** the `discrimination 0/2` line in `SUMMARY-9.5-bounds.md`, at all three
bounds, and the inference drawn from it that ranking fails on both questions. Half of it was a
grep bug; the rest is order-dependent. It must not be quoted again as a retrieval finding.

**Not invalidated:** that summary's actual conclusion — that a 4x range of passage ceilings
(1000/2000/4000) changed no graded outcome and only moved evidence size. Every non-discrimination
band scored identically across all three, and those questions' matchers contain no leading `-`.
The recommendation to keep 2000/400 stands.

**Also not invalidated:** X01's failure. It is real, reproduces at every position, and its root
cause has since been measured independently (embedding dilution — see
`openspec/changes/fix-default-vs-override-ranking/proposal.md`). What changed is its
description: `README.md § Configuration` is **not** absent, as previously recorded. It is
recalled at rank 7 and returned at node rank 3. The band grades the top node alone, which is
why a two-position ranking miss presented as a total absence.

## Reproducing

Recall order with the similarities fusion discards — the single most useful probe here, because
no product surface exposes them:

```bash
docker run --rm --network <project>_c360 natsio/nats-box:latest \
  nats -s nats://nats:4222 req graph.query.semantic \
  '{"query":"<q>","limit":40,"scope":["c360.semsource.web","c360.semsource.config"]}' --raw
```

Position dependence: run the full set with X02 last, then reorder it first, changing nothing
else.

## Follow-up: localising the order-dependent drop

Three further probes, same stack and corpus.

**It is transient and self-healing.** 21 diverse queries in one MCP session, then X02 four
times in a row:

```
X02 call 1: TIER2 ABSENT
X02 call 2: TIER2 rank [1]
X02 call 3: TIER2 rank [1]
X02 call 4: TIER2 rank [1]
```

Only the first call after the burst fails. Other passages from the same document
(`configs-tiers-README-md-0003`, `-0004`) survive that call at ranks 9 and 18; it is the
**top-ranked** entity that vanishes. Nothing is logged — zero `WARN`/`ERROR` in the window.

**It is not the embedding service.** The obvious suspicion is that the query-embedding path
degrades under load. Tested by running the same burst and then making `graph.query.semantic`
the *first* call afterwards, bypassing fusion: the recall list came back pristine — identical
similarities (0.6801 / 0.6443 / 0.6155 / 0.6124) in identical order — with no errors in the
`semembed` log for the whole session. Fusion reaches the embedder through that same subject and
handler, so query embedding, cosine scoring and candidate selection are all intact. The defect
is strictly downstream of recall.

(For the avoidance of doubt: this stack is **tier-1 only** — `configs/mvp.json`, `semembed`.
`seminstruct` is tier-2 and is not running. It appears only as corpus text that X02 asks
about.)

**It is `doc_context`-only, so far.** After the same burst, `code_search` (C04/C05/C06 — the
embedding-backed code lens) returned an identical top node across 4 consecutive calls each. Not
proof of immunity, but the reproduction should not be generalised across lenses.

**It is not a mixed vector population.** A natural second suspicion is that entities are first
embedded statistically and progressively replaced with neural vectors, so a candidate set would
depend on which entities had been upgraded yet. That is not how this works: `embedder_type` is
a single config value resolved once at startup (`createEmbedder`,
`processor/graph-embedding/component.go:674`), a hard switch on `bm25` vs `http` with no
runtime escalation and no replacement pass. A failure to resolve the HTTP endpoint errors the
component out rather than silently downgrading; the "BM25 as a fallback" note in
`graph/embedding/doc.go` refers to selecting BM25 *by configuration* (tier 0), not at runtime.

Confirmed in the stored data, not just the code. Vectors live content-addressed in
`EMBEDDING_DEDUP` (4,998 records; `EMBEDDINGS_CACHE` is empty). Sampling 40 of them: every one
is 384-dimensional with **0% zero components and ~50% negative components**.

That discriminator was validated against the BM25 implementation rather than assumed:
`computeBM25Vector` (`graph/embedding/bm25_embedder.go`) hashes each term to a dimension and
accumulates a BM25 score whose IDF is clamped to a minimum of **+0.01**, so every contribution
is non-negative and any dimension no term hashed to stays exactly zero. **A BM25 vector here is
sparse and non-negative; the stored population is dense and signed.** No statistical vectors are
present. Note a dimensionality check alone would not have discriminated — BM25 here is also
384-dimensional — and the dedup records carry no per-record `model` field, so the sparsity/sign
signature is the available evidence.

Supporting: `embedder=http` on every probe, the 100-result similarity distribution is unimodal
(0.5741–0.7322) where a mix would be bimodal, and LPA is not running at all in this stack
(`enableClustering := false`, `cmd/semsource/run.go:744`; `mvp.json` does not enable it).

What *does* progress over time is embedding **coverage**, as `indexed_revision` climbs toward
`target_revision` — which is why `embedding.ready` exists and why `run.sh` gates on it. Every
measurement here was taken at a stable 10038/10038.
