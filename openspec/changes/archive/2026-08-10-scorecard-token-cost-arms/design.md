# Design: scorecard-token-cost-arms

## Context

See `proposal.md` — Why. Constraints inherited from the existing harness, all load-bearing:

- **Deterministic grading only.** No LLM judge, no live agent: a drifting judge cannot support an A/B
  (`run.sh` header comment). Every arm must be a fixed procedure.
- **The harness does not provision.** Arms that need the stack (B, C) point at one already up and gate
  on the status signals; arm A needs only the corpus checkout.
- **Comparability discipline.** Scores are only meaningful within one `questions.json` version. This
  change adds no questions and bumps nothing; cost figures inherit the same rule and add one more:
  cross-arm comparison requires the same corpus checkout and, for arms B/C, the same stack.
- **First-answer retention.** Repeats exist to detect instability, not to shop for verdicts. Cost
  accounting follows the same rule (Decision D5).
- Corpus rules stand: same corpus for every arm, `scripts/scorecard/` excluded from ingestion.

## Goals / Non-Goals

**Goals (design-level):**

- Every arm consumes only a question's `args.query` and the corpus/stack — never `expect_*` fields.
  Blindness is the fairness property that makes the comparison publishable.
- One results JSON shape across arms, so comparison is a `jq` join, not a bespoke parser.
- Bytes are the primary metric; tokens are a derived estimate, never measured backwards from a
  tokenizer choice.

**Non-goals (design-level):**

- Simulating agent *iteration* (query reformulation, follow-up greps). Arm A is one deterministic
  pass; the README will state that real agentic grep is more expensive than this floor on hard
  questions and cheaper on easy ones — it is a floor, not a simulation.
- Byte-parity with any external benchmark's numbers. We adopt the methodology, not the figures.

## Decisions

### D1 — Arm A (grep-and-read) procedure: term-coverage reading, whole-file charging

The baseline answers each question with a fixed pipeline:

1. **Term extraction:** lowercase `args.query`, split on non-alphanumerics, drop words in a fixed
   stopword list committed to the repo (`stopwords.txt` — articles, interrogatives, auxiliaries).
   Remaining terms are the search terms. No stemming, no synonyms — deterministic and inspectable.
2. **File ranking:** `grep -ril` each term over the corpus; rank files by distinct-terms-matched,
   tie-break by total match count, then lexicographic path. Deterministic on any box.
3. **Reading:** read files in rank order until every search term has appeared at least once in the
   read set, or a fixed file cap (default 5) is exhausted. Charge the **full bytes of every file
   opened**. The concatenated read set is the arm's "answer"; the top-ranked file plays the role of
   the top node for discrimination grading.
4. **Grading:** the unchanged `grade_answer` matchers run over the read set.

*Why term-coverage stopping over a fixed always-read-K:* it adapts the way a reader does ("I have
now seen context for every term I searched") without ever consulting the expected answers — adaptive
fairness with zero answer-leakage. A fixed K over-charges easy questions and under-charges hard ones.

*Why whole-file charging:* it matches how agents actually read (and how the reference benchmark
charged its grep arm). The bias it introduces runs **against** arm B's competitor — inflating the
baseline's cost — so it must be stated in the README; per-file byte components are recorded in the
results JSON so a context-window (`grep -C`) variant can be computed later without re-running.
The reference benchmark's grep arm won its code-detail questions *despite* whole-file charging,
so the bias direction is known not to decide the headline comparison by itself.

*Alternative rejected:* charging only grep match windows (`grep -C 20`) — underestimates real
reading and is harder to defend than an honest, direction-known bias.

### D2 — Token figures are `bytes / 4`, bytes are primary

No tokenizer dependency: deterministic, offline, and not tied to any one model's vocabulary. The
results JSON stores bytes; reporting derives tokens at display time and labels them as estimates.
A real tokenizer can be swapped into the *display* layer later without invalidating stored runs.

### D3 — Arm C (semembed-only): product vectors, cosine-only ranking, bodies fetched for grading

Arm C isolates the query/fusion layer by holding chunking and embedding constant:

1. Embed `args.query` via semembed `/v1/embeddings` with `query_prefix` on the query side only —
   the exact pattern that reproduced live cosines bit-for-bit during the ranking work.
2. Rank the product's own stored vectors (`EMBEDDING_INDEX`, entity-keyed) by cosine. Using the
   product's vectors eliminates chunker drift: the *only* difference from arm B is the scoring and
   candidate-selection machinery on top.
3. Fetch bodies for the top-K ranked entities through the existing graph query surface — the graph
   acts as a dumb body store here; ranking never consults it, so the isolation property holds.
4. Grade top-K bodies with the unchanged grader (rank-1 body is the top node); charge their bytes
   plus the (recorded, small) query-embedding request/response bytes.

K defaults to the node cap arm B's answers carry, recorded per question, so cost parity is
structural rather than tuned.

**Fallback** if `EMBEDDING_INDEX` payloads prove impractical to parse from a script: enumerate
passage entities via graph query, re-embed their bodies through `/v1/embeddings`, and rank those.
The ranking work demonstrated offline re-embedding reproduces live cosines (0.7323 vs 0.7322), so
the fallback is evidence-equivalent, just slower. Either path gates on `embedding.ready` exactly as
`run.sh` does.

*Known interpretation caveat (goes in the README):* `doc_context` ranking is already nearly pure
cosine order **after** candidate recall, so doc-band C-vs-B deltas measure the recall stage and
salience terms, not the whole fusion stack; code-band deltas (vs the structural `NAME_INDEX` path)
are the discriminating ones.

### D4 — Schema overhead: measured at session init, reported session-level

After `initialize`, arm B issues one `tools/list` and records the response's byte length and tool
count in the results JSON header. It is never amortized into per-question figures — the honest
break-even framing is "fixed cost F plus marginal cost per question," and folding F in would just
hide F. Feeds #126's measurement.

### D5 — What "including failed attempts" means for arm B

The first call per question is charged, whether it succeeds or returns `isError` — an error result
still costs an agent its bytes. Repeat calls (instability detection) are instrumentation, not agent
behavior, and are never charged. This keeps cost accounting consistent with the existing
first-answer-retention rule.

### D6 — One results shape, three writers, one comparator

`run.sh` (arm B) gains cost fields; new `arm-a-grep.sh` and `arm-c-cosine.sh` emit the same results
JSON shape (id, band, verdict, reason, cost fields, arm label). A new `compare.sh` joins any set of
result files and renders the per-band × per-arm table (recall and cost). Existing result files
remain readable — new fields are additive.

## Risks / Trade-offs

- [Whole-file charging inflates arm A's cost] → bias direction documented; per-file components
  recorded so a window-charged variant is derivable from stored runs.
- [Stopword list becomes a hidden tuning knob] → committed to the repo, covered by the README's
  comparability rules: changing it invalidates cross-run cost comparison, same as a questions bump.
- [`EMBEDDING_INDEX` payload format is internal and may change under semstreams bumps] → fallback
  path (D3) is evidence-equivalent; if both paths break, that is a real upstream-visibility gap to
  log in `docs/upstream/semstreams-asks.md`, not something to work around silently.
- [Arm C's body-fetch could be misread as "using the graph"] → README states the isolation claim
  precisely: ranking is cosine-only; the graph serves bytes for grading, after ranking.
- [Cost numbers get quoted against other tools' published figures] → README comparability section
  extends to cost: figures are internally comparable across arms/runs on the same corpus only.

## Migration Plan

Additive tooling; no deploy surface. Existing labels/results stay valid (new JSON fields are
additive). Rollback is deleting the new scripts. No product code changes, so CI impact is nil
beyond shellcheck-style hygiene if the repo lints scripts.

## Open Questions

- Whether to also emit a `grep -C`-window cost variant for arm A in `compare.sh` (derivable from
  stored per-file data either way — display-layer choice, defer until first real run).
- Exact default file cap for arm A (5 unless the first dogfood run shows term-coverage rarely
  terminating earlier) — parameter, not architecture.
