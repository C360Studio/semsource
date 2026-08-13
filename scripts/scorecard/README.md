# Retrieval scorecard

A fixed question set and a deterministic grader for measuring what SemSource can
actually answer about a corpus, run over the real MCP surface against a live stack.

It lives in the repository on purpose. The 2026-07-19 audit's graded set was built
in a session scratchpad and was gone by the time the next change wanted to re-run
it, which cost the project its only longitudinal retrieval measurement.

## Comparability — read this before quoting a number

**A score is only meaningful against another score taken with the same
`questions.json`.** The set is versioned (`version` field); bump it on any change
to a question, and never compare across versions.

In particular, **these numbers do not continue the audit's 13/19 → 16/19 → 17/19
series.** That series used a different, now-lost set. Quoting a score here as if it
extended that line would be inventing a trend.

**Version 3 does not compare to version 2**, for two reasons — both grader changes,
neither a product change, so numbers moved without retrieval moving:

1. **The matchers were broken for one question.** `grep -qF "$w"` had no `--`, so
   X02's `-p 8083:8083` was parsed as options; grep exited 2 and the loop read that
   as "not found". X02 graded `miss` on every system, forever, while retrieval was
   correct.
2. **`MISLEADING` was added**, and it takes results that v2 scored as plain `miss`.

Consequently the `discrimination 0/2` recorded in
[`results/SUMMARY-9.5-bounds.md`](results/SUMMARY-9.5-bounds.md) was never a
retrieval result and must not be quoted as one. That summary's *bounds* conclusion
is unaffected and stands.

**Version 4 adds the composition band and does not compare to version 3.** Every
v3 question is retained verbatim, so the old bands still grade identically — but
the totals include four new questions, and quoting a v4 total against a v3 total
would move a number without retrieval moving. The v3 results remain in `results/`
under their version.

**Version 5 adds the loose band and does not compare to version 4.** Same rule,
same reason: every v4 question is retained verbatim and the unchanged bands still
grade identically, but the totals include seven new questions. Per-band
comparisons across v4/v5 stay valid for the unchanged bands; totals do not.

**Two more comparability rules ship with v4:**

- **Corpora never compare.** The OSH question set (`questions-osh.json`) is its
  own comparability domain: OSH scores never merge with dogfood scores, in either
  direction. `compare.sh` enforces this mechanically because the sets carry
  different `questions_version` values.
- **Latency figures only compare on one machine class.** Wall-clock is
  nondeterministic by nature; the results header records the host arch (M3 Pro
  and the Intel dev mini produce different numbers for identical retrieval), and
  `compare.sh` warns when joined files disagree on it. Recall and bytes stay
  comparable across machines; milliseconds do not.

## What this does NOT measure

The scorecard grades the **fusion** MCP tools (`code_context`, `code_impact`, `code_search`,
`doc_context`, `code_changes`) — deterministic, citable retrieval whose answers a string matcher can
grade the same way twice.

It does **not** grade the graph-query tools' *escalating* surfaces (community summaries,
LLM-synthesized answers), even though `graph_search` is on the MCP surface. Those answers change
with the stack tier, grading synthesized prose needs a judge, and the reason this harness has no
judge is stated in `run.sh` — an LLM judge drifts between runs, and a drifting judge cannot
support an A/B. Adding one here would silently convert this from a comparable instrument into an
incomparable one.

One deliberately narrow exception: the OSH v2 config band (G01–G03) grades `graph_search`'s
**deterministic match-list rendering** — labels and bounded value properties, exact substrings
over a non-synthesized surface — which is the same grading contract as every other band. The
escalating surfaces above it remain ungraded.

A GraphRAG answer-quality instrument is therefore a **separate** instrument with its own pass/fail
reporting, not a section of this file. What *is* pinned deterministically about the graph-query tools
— that they answer at every tier and disclose the retrieval rung they reached — lives in the
mcp-gateway unit tests and the core-profile compose smoke.

Keeping the two apart matters beyond tidiness: measuring only the deterministic surface quietly
redefines "quality" as retrieval precision, which is a metric the product can win while getting worse
at answering questions.

## Running it

The harness does not provision anything, so the same script can be pointed at two
different builds. Stand up a stack, wait for ready, then:

```bash
SEMSOURCE_HTTP_PORT=28080 scripts/scorecard/run.sh <label>
```

Results land in `results/<label>.json` with every answer retained, so a verdict can
be re-examined without re-running the stack.

Knobs: `SEMSOURCE_HTTP_PORT` (default 28080), `SEMSOURCE_HOST`,
`SCORECARD_READY_TIMEOUT`, `SCORECARD_CALL_TIMEOUT`,
`SCORECARD_QUESTIONS`, `SCORECARD_REPEATS` (default 3 — see *Repeats* below).

Before scoring, validate the question set. This gates the three ways a question
rots — the corpus relationship between a confusable pair, the corpus relationship
between a composition question's facts, and whether the grader can evaluate the
literals at all:

```bash
scripts/scorecard/check-discrimination.py <corpus-dir>
scripts/scorecard/check-composition.py <corpus-dir>
scripts/scorecard/test-matcher.sh              # no stack needed
```

Always use an isolated `COMPOSE_PROJECT_NAME` and high ports — this machine also
runs other stacks, and `nats` on 4222 is generally not ours.

## The A/B procedure

To measure a change rather than a build, score **both sides against one set**:

1. Build the baseline binary from the baseline commit (a `git worktree` keeps the
   main checkout intact), bring up a stack with a **fresh** graph, wait for ready,
   run with a label.
2. Rebuild from the candidate commit, `docker compose down -v` again, bring up,
   run with another label.
3. Compare per band.

**Hold the corpus fixed and vary only the binary.** Point `SEMSOURCE_TARGET` at the
same checkout for both runs. Ingesting each side's own tree would confound the
result: the candidate's tree contains files the baseline's does not, so a question
about new code could not pass the baseline for reasons having nothing to do with
retrieval.

The `down -v` matters. Doc identity and body handles changed in the passage-chunking
work, and a graph carried over from the other side is neither one thing nor the other.

## The arms — measuring cost, not only recall

The product's claim to agent consumers is a **cost** claim: querying the graph is
supposed to be cheaper than reading source. Recall alone cannot test that, so the
harness runs the same question set through competing retrieval procedures ("arms")
and records, per question, the **context bytes ingested** — everything the caller
would have had to read. Bytes are the measured figure; tokens are `bytes/4`, an
estimate, and labeled as one. See issue #130 for the methodology's provenance and
the decision that this harness owns the offline arms while semdev stays the live
two-arm harness.

- **Arm A — `arm-a-grep.sh <corpus-dir> <label>`.** A deterministic grep-and-read
  floor: stopword-filtered query terms, files ranked by grep hits, read in rank
  order until every term found anywhere has been seen (cap 5 files), charged at
  **full file size**. Retrieval consumes only `args.query` — the `expect_*` fields
  are read solely by the grader, after the read set is fixed. Whole-file charging
  biases the comparison *against* arm B's competitor (it inflates grep's cost);
  per-file bytes land in the results so a match-window variant can be derived
  later without re-running. It is a floor, not a simulation: real agentic grep
  iterates, re-greps, and costs more on hard questions. No repeats — the arm is a
  pure function of the corpus.
- **Arm B — `run.sh <label>`.** The MCP surface, as before. `context_bytes` is the
  full decoded result of each question's **first** call, charged whether it
  succeeded or returned `isError` — an error still costs an agent its bytes.
  Repeat calls are instability instrumentation, never charged.
- **Arm C — `arm-c-cosine.sh <label>`.** Embeddings without the query machinery:
  embed `args.query` against semembed directly, cosine-rank the product's own
  stored vectors, grade the top-K bodies. Ranking never consults the graph — the
  graph serves bytes for grading only, after ranking, so the C-vs-B delta
  isolates what the query/fusion layer adds over raw cosine. Read the doc bands
  with care: `doc_context` ranking is already nearly pure cosine order *after*
  candidate recall, so doc-band deltas measure the recall stage and salience
  terms; the code bands (vs the structural name index) are the discriminating
  ones.

`compare.sh <results.json> [...]` joins any set of same-version result files into
the per-band × per-arm table and refuses mismatched `questions_version` values.

**Schema overhead is reported separately, never amortized.** Registering an MCP
tool surface costs a session its schema bytes whether the tools are used or not.
`run.sh` measures `tools/list` once and records it as a session-fixed figure next
to — never inside — the per-question costs. The honest accounting is "fixed cost F
plus marginal cost per question"; folding F into the questions would hide F. This
is the measurement side of issue #126.

**Cost comparability adds two rules to the score rules above:** cross-arm figures
are only comparable for runs over the same corpus checkout (and, for arms B/C, the
same stack), and `stopwords.txt` is part of arm A's procedure — changing it
invalidates cost comparison against earlier arm A runs, exactly as a
`questions.json` bump invalidates score comparison.

## Why grading is deterministic

Substring matching, case-insensitive, no model in the loop. An LLM judge drifts
between runs, and a drifting judge cannot support an A/B — a score change becomes
indistinguishable from a judge change. The trade is that matchers are coarse: they
verify the answer *contains* the load-bearing fact, not that the prose around it is
good.

Six verdicts, deliberately distinct:

- **correct** — required content present.
- **miss** — the answer did not contain it. An honest failure.
- **IMPRECISE** — the top-ranked evidence carried the answer *and* a confusable
  value it could be mistaken for, so the evidence does not settle the question.
  Only discrimination questions can produce this.
- **MISLEADING** — the top-ranked evidence carried the confusable value **instead
  of** the answer. Only discrimination questions can produce this.
- **UNSTABLE** — the same question returned different verdicts across repeated
  calls, so it has no defensible verdict at all. See *Repeats* below.
- **FABRICATED** — the answer asserted something known to be false. This is not a
  worse miss, it is a different failure, and it outranks every other result in the
  summary. Zero fabrication is this product's actual moat; a set that scores well
  while inventing one answer has failed.

**IMPRECISE is deliberately not folded into FABRICATED.** A whole-file body that
happens to contain both the answer and its twin has invented nothing — it is
imprecise, not dishonest. Merging the two would destroy the fabrication signal,
which is the single result that outranks everything else here.

**MISLEADING is deliberately not folded into `miss`,** for the same reason. A miss
returns nothing useful; a MISLEADING top node argues for the *wrong* answer, and an
agent citing the first result will state it as fact. Until version 3 this state was
unreachable: the answer-side check short-circuited to `miss` before the confusable
check ever ran, so the most damaging outcome was scored as the most innocuous one.

An `isError` result is recorded as `error`, never graded as an answer.

### Repeats — why a question is asked more than once

Each question is asked `SCORECARD_REPEATS` times (default 3) and the verdicts
compared. If they disagree the verdict is **UNSTABLE**, recording every distinct
outcome; it is counted separately and never resolved to either the passing or the
failing result.

This is not defensive coding. A verdict was measured to depend on a question's
**position in the run**: the same question against an unchanged stack returned the
correct passage when asked first and lost it from the entire response when asked
last — transiently, self-healing on the very next call, with nothing logged. That
is a live platform defect, reproduced and filed upstream, not an instrument
artifact. See [`results/SUMMARY-instrument-diagnosis.md`](results/SUMMARY-instrument-diagnosis.md).

A warm-up call or a retry-until-pass would produce a cleaner number by concealing a
defect a real caller hits on their first request. A scorecard that protects its own
score is worse than one that admits it cannot measure.

**`SCORECARD_REPEATS=1` cannot detect instability**, so a one-repeat run must not be
quoted as evidence that anything is stable. The run prints that caveat itself.

### Matchers

`expect_all`, `expect_any` and `expect_none` match against the **whole answer**.
`expect_top_all` and `expect_top_none` match against the **top-ranked node's body
only** — see the discrimination band below for why that distinction is the whole
point.

## Question bands

Bands exist so a score can be read rather than just totalled.

- **doc-early** — facts inside the first 8 KB of their document.
- **doc-late** — facts past 8 KB.
- **code** — symbol and concept retrieval on the code side.
- **impact** — dependents named, not merely counted.
- **negative** — must miss.
- **discrimination** — the top node must answer on its own.
- **composition** — no single passage can answer; see below.
- **loose** — UI-shaped phrasings of facts other bands already probe; twins
  isolate phrasing from retrieval; see below.

`doc-early` versus `doc-late` is the load-bearing split for passage chunking. The
substrate truncates embedding text at 8000 characters, so before chunking a
document's tail was silently unindexed. On this repository the README's own cut
falls right after the "UI profile" section: Quick Start, Source Types and the CLI
Reference sit before it; the Port Map, Status Phases, Config File and the whole
Fusion API sit after.

**`doc-early` is the control and it is not decoration.** A set made only of
`doc-late` questions would be rigged — chunking would improve it by construction.
The early band is what detects a regression: if chunking breaks something that
already worked, it shows up there, and a `doc-late` gain bought with a `doc-early`
loss is not a win.

## The discrimination band — what fact-presence cannot measure

Fact-presence grading cannot separate whole-file retrieval from passage retrieval.
A 31 KB README body trivially contains every fact in it, so a substring matcher
passes whether the system found the right paragraph or dumped the document. The
first A/B run said so in its own summary and had to fall back on bytes-per-answer
as the real result. Bytes are a proxy: they show the evidence got smaller without
showing that anything became answerable that was not answerable before.

A discrimination question closes that gap. It targets a fact whose document also
contains a **confusable twin** in a different section — the same env var with a
different value, the same command shape with a different port. The question then
asserts both directions against the top-ranked node:

```json
"expect_top_all":  ["NATS_MONITOR_HOST_PORT=8222"],
"expect_top_none": ["NATS_MONITOR_HOST_PORT=28222"]
```

Whole-file retrieval puts both strings in one body, so the evidence cannot settle
which is the default — `IMPRECISE`. Passage retrieval returns the section holding
one of them, so the evidence answers on its own — `correct`. That is a capability
difference, not a size difference.

**Why the top node and not the whole answer.** An answer carries up to 20 passages.
Grading the union would let a confusable value elsewhere in the same document ride
along even when retrieval ranked the right passage first, so both systems would
fail and the question would measure nothing. The narrower claim — the single best
piece of evidence stands alone — is also the one an agent actually depends on.

**The two questions have different sensitivities, on purpose.** X01's pair is 260
lines (~13.7 KB) apart, so it separates under any plausible ceiling — it asks
whether chunking happened at all. X02's pair is 42 lines (~3.1 KB) apart in an
8257 B file, so whether it separates depends on the ceiling — it is the one that
responds to tuning.

### Validating a discrimination question — do not skip this

Run the checker before scoring; it gates on the two ways these questions rot:

```bash
scripts/scorecard/check-discrimination.py <corpus-dir>
```

**The pair must not be a substring of one another.** Bare `8222` matches inside
`28222`, so the twin would satisfy the answer check and the question would pass on
every system while measuring nothing. X01 matches on `NATS_MONITOR_HOST_PORT=8222`
for exactly this reason.

**The pair must not co-occur closely in ANY ingested doc** — not just the document
you designed against. Two candidates died here after surviving a careful read: a
ui-dev-overlay/released-image pair (clean in README.md, but ROADMAP.md names both
two lines apart) and a SemStreams version pair (two lines apart in
docs/testing/readme-surface-coverage.md). Both would have reported IMPRECISE on
every system forever, hiding real regressions behind a constant failure.

**Exclude `scripts/scorecard/` from the ingested corpus.** This directory quotes
both literals of every question side by side, so ingesting it plants a
guaranteed-IMPRECISE passage in the corpus — the measuring apparatus corrupting
the measurement. The checker excludes it; your corpus build must too.

A consequence worth stating plainly: this repository's docs contain very few
well-separated confusable pairs. An automated sweep of every `KEY=VALUE` literal
found exactly one usable pair beyond the two shipped here. The band is small
because the corpus supports a small band, not because two questions is a target.

## The composition band — what single-shot retrieval cannot measure

By 2026-08 both product arms had scored 22/22 for three runs straight: the
instrument was saturated, and a saturated instrument cannot answer the question
that matters — does the fusion layer's recall actually beat raw cosine (the
C-parity finding), or is the query machinery not earning its keep? Every v3 band
rewards finding *one good chunk*, and both arms had become reliably good at that.

A composition question is one whose answer provably does not live in one chunk:
its `expect_all` facts **do not co-occur within any single ingested passage or
code file**, so answering requires the graph edge set — a reverse-dependency
closure whose members live in different files — not retrieval luck. The harness
stays one-tool-call-per-question and fully deterministic: composition happens
server-side in the structural tools (`code_impact` names each resolved node's
direct dependents), while arm C must try to luck the same facts into its top-20
and arm A must try to grep them into a five-file read set. This is the band
where arm B must beat arm C on **recall**, or prove — against questions verified
compositional — that they do not separate, which is itself the decision-grade
answer.

**The property is about the DATA and it rots silently** — a refactor that moves a
caller into the callee's file, or a new doc summarizing both sides of a join,
makes the question single-passage-answerable without anyone noticing. So it gets
the discrimination treatment: `check-composition.py` scans the corpus before
every scored run and FAILS any composition question whose full fact set co-occurs
in one window. Doc windows are 6000 B — the splitter's `hardMax`, not its 2000 B
ceiling, because a fenced block is kept whole past the ceiling and gating on the
smaller number would admit a question one oversized passage answers. Code and
config files are one window each: a symbol body never extends beyond its file,
so whole-file is a strict, language-neutral upper bound. `--simulate` plants a
synthetic one-window doc carrying a real question's facts and requires the gate
to fire; `test-matcher.sh` asserts both directions plus the window arithmetic.

**The band is impact-closure-only, and that is the finding, not an oversight.**
The other three designed shapes are inadmissible on this corpus, each for a
reason worth knowing:

- **Version composition** (`code_changes`) needs two ingested versions, and the
  corpus recipe is `git archive` — no `.git`, no commits, no version entities.
- **Cross-source joins** stay unreachable in one call on the default response
  path: `graph_search`'s ranked compact shape carries no property values (a
  dependency's `ConfigDepVersion`, say). Gateway rendering for the full-entity
  shape shipped with `graph-search-match-properties` (#166), but its
  default-path enablement is blocked on semstreams#958 — and on THIS corpus
  the shape stays inadmissible for composition questions regardless: Go import
  syntax puts dependency paths into every consuming file, co-locating the pair.
- **Caller+callee relation joins** need the caller, the symbol, and the callee in
  three different files; this codebase keeps helpers beside their callers, and
  callee-*pair* facts are inadmissible **by construction** — a function's own
  body co-locates every callee it names.

Authoring rules learned the careful way: the fact pair must be checked with
**test files included** (tests are ingested and only demoted in ranking, so they
count toward `code_impact`'s 12-per-role naming cap — a symbol with more than 12
distinct dependents will have one silently evicted, and a question expecting the
evicted name fails on every system forever). P01's anchor sits at exactly 12;
its `why` records the arithmetic. A candidate anchor with 13 was rejected for
exactly this reason.

## The loose band — phrasing, not retrieval

A human typing into a UI field is the one caller with no agent in the loop to
reformulate a weak query (#170). Every NL surface currently runs the semstreams
classifier chain at its keyword tier only, and that tier's vocabulary is
sensor-heritage — so the open question is whether the embedding stage absorbs
loose human phrasing on its own, or whether the dormant T1 embedding-classifier
tier (`domain_examples_path` on the GraphQL gateway) needs feeding. This band is
the gate for that decision: it must be run and found wanting **before** any
classifier work is justified — the same measure-first bar that demoted the #137
reranker.

Each question restates a fact an existing question already probes — its **twin**,
named in the question's `twin` field — in words a person would actually type,
never containing the identifiers the twin leans on. Expects are identical to the
twin's, so the pair grades on the same terms. The pairing is the instrument:

- twin passes + loose fails → a **phrasing gap**: the NL front-end requires the
  caller to speak the schema's language. This is the outcome that justifies
  feeding T1.
- both pass → **saturation**: loose phrasing is absorbed and classifier work
  stays unjustified (#170 closes as measured-no-gap).
- both fail → a **retrieval gap** that was never about phrasing — file it
  against retrieval, not the classifier.

Looseness dimensions covered: vocabulary-free (L01), effect-described (L02),
colloquial-where (L03), behavior-described symbol hunt (L04 — the twin uses
`code_context` with the symbol; a human without the symbol necessarily lands on
the NL tool, so the tool differs by scenario, not by slip), maximally colloquial
(L05), single-transposition typo (L06), and one twin-less verified single-hop
fact phrased by effect (L07, `handler/excludes.go`). Arm A is expected to
collapse on this band — its terms come from the query, and the queries are
built to share no tokens with the evidence; that collapse is a measurement of
the grep floor under human phrasing, not a rigged loss (the twins keep A's
performance on precise phrasings in view).

## Latency — the dimension the harness never measured

v4 records per-question wall-clock in every arm: `latency_ms` is the **first**
call's (consistent with first-answer retention — cold starts are visible, not
hidden), `latency_samples` keeps all repeats. `compare.sh` renders median/p95
per band per arm. No warm-up special-casing: the first call's latency is the one
a real caller pays.

What each arm's figure covers: arm B, the MCP round-trip; arm A, the full
grep-rank-read procedure; arm C, embed + cosine-rank + body fetch (`armc-dump`
is built once per run so the figure measures retrieval, not `go run`'s
staleness checks). The shared clock lives in `timing.sh` — perl `Time::HiRes`
on every machine, so the spawn overhead is identical everywhere it lands.

Rules: same-machine comparisons only (the header records host arch and
`compare.sh` warns on mixes), never across corpora, and medians/p95 over means —
three samples on a shared dev machine are spread detection, not a distribution.

## The OSH corpus — the second scale point

`corpus-osh.sh <target-dir>` builds an Open Sensor Hub core checkout at a
**pinned** SHA (recorded in the script; `master` per the early adopter) via
`git archive` — the first Java/Gradle-family corpus this project has measured,
at roughly 3x the dogfood corpus. Score it with:

```bash
SCORECARD_QUESTIONS=scripts/scorecard/questions-osh.json scripts/scorecard/run.sh <label>
```

`questions-osh.json` (version 1) is authored against exactly that pin; bumping
the pin means re-verifying the set with both checkers and bumping its version.
OSH scores are their own comparability domain and never merge with dogfood
scores — see Comparability.

**Version 2 adds the config band** (G01–G03; a new comparability domain — v2
scores never compare with v1), and the band is **expected red until
semstreams#958 lands** — deliberately, on the P01/#141 precedent: a red band
naming a real gap is the instrument working. The trail: v1 had no config band
because gradle dependencies were unlabeled (#142 → #157 added titles), then
`graph-search-match-properties` (#166) shipped gateway rendering of labels +
bounded value properties from entity triples — but the default response path
requests the substrate's ranked COMPACT shape (the full-entity alternative was
tried and falsified: no reply-size guard, cache-order truncation, no relevance
scores), and compact digests label dependencies with hash instances and carry
no property values. That digest defect is upstream's (semstreams#958). The
rendering itself is unit-tested and was live-verified 13/13 under the
threshold-0 experiment (`results/166-pass-after-osh-B.json`, preserved). Only
configurations the regex parser ingests exist in the graph — see
`handler/cfgfile/parsers.go`.

## Arm-D readiness — dormant LLM-cost fields

A future research-graph deep-search arm will spend LLM calls at query time. The
results schema is ready for it without a break: every header carries
`arm_uses_llm` (false for A/B/C), and per-question `llm_calls` (int) and
`llm_cost_note` (string) are defined but absent until an arm emits them.
`compare.sh` renders the LLM column only when a result carries it, so today's
tables stay clean. Grading for such an arm stays the deterministic matcher over
whatever evidence its pipeline returns; run variance lands in the existing
UNSTABLE machinery.

## Adding questions

Each question needs a `why` explaining what it probes and where the fact lives.
Prefer facts that are verifiable in the corpus and stable across edits. Avoid
questions whose answer is a line number, a byte offset, or anything that churns on
unrelated commits.
