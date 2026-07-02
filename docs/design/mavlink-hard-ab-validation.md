# Fusion Gateway A/B Validation — `mavlink-hard` — Design Document

> **Status:** Draft (test design) | **Date:** June 2026
> **Scope:** Downstream A/B test proving the fused `code_context` tool beats the
> status quo on a hard, realistic agent task. Supplies the empirical evidence
> behind [semstreams#376](https://github.com/C360Studio/semstreams/issues/376)
> and Fusion E (ADR-0004).
>
> **Update (2026-07):** The gateway under test has since **shipped and converged** onto
> semstreams `pkg/fusion` (see ADR-0004's convergence note; PRs #15/#16). This remains the A/B
> *test design* that motivated the upstream proposal — it predates the convergence, the recorded
> measurement result is not here, and one internal detail is now stale (the request no longer
> carries a server-side repo path; bodies are dereferenced from an ObjectStore handle).

---

## Motivation

ADR-0004 built the deterministic fusion gateway on a diagnosis, not a measurement:
agents bail to `grep` because triples-by-entity-ID are the wrong *exposure*, and a
fused, source-first response should fix it. That claim is plausible but unproven.
This test measures it on the canary task — SemSpec's `mavlink-hard` prompt — so the
upstream proposal rests on data, not assertion.

The hypothesis, stated to be falsifiable:

> Given the same agent, model, and task, exposing a fused `code_context` tool
> (verbatim source + callers/callees + impact, IDs demoted to handles, with a
> `ready ≠ not-found` envelope) **reduces tool-calls-to-solution and grep-fallback
> rate without regressing solve rate**, versus an agent on raw `graph.query.*` + grep.

If it doesn't, that is a real result — it tells us the gateway's value is elsewhere
(or absent) before we ask the framework to adopt it.

## Deployment topology under test

semsource runs **standalone as an external service** (its own graph subsystem +
ingested worktree + fusion gateway, co-located in one service); the agent harness
calls it over HTTP/NATS. This is the production topology after SemSpec's refactor —
semsource is an optional external dependency, no longer embedded headless — so the
test is fully representative, not a lab-only configuration.

Because hydration happens server-side where the worktree lives, only the *assembled*
response crosses the wire; the client never needs filesystem access. (See the
object-store discussion for the one rough edge this leaves: the request currently
carries a server-side repo path.)

## Experimental design

Identical agent, model, temperature, system prompt, and repo state across arms. The
only variable is the toolset.

| Arm | Toolset | Purpose |
|-----|---------|---------|
| **A — control** | grep / read + raw `graph.query.*` | the status quo that makes agents retreat to bash |
| **B — treatment** | A's tools **+** fused `code_context` | the gateway under test |
| **C — optional** | B, hydration via ObjectStore vs disk | validates the storage direction (consistency/latency), secondary |

Arm B keeps A's tools available on purpose: we want to see whether the agent
*chooses* `code_context`, not force it. Tool adoption is itself a result.

## Task

SemSpec's existing **`mavlink-hard`** prompt over the MAVLink codebase, unchanged
across arms. It is already the team's hardest e2e canary, so gains there are real
consumer pressure rather than a synthetic benchmark.

## Metrics

**Primary**

- **Tool-calls-to-solution** — invocations until a correct answer. The core
  efficiency claim: one fused call should replace several graph hops + re-reads.
- **Solve / correctness rate** — graded against a known-good rubric. B must not
  regress this; a faster wrong answer is not a win.
- **Wedge rate** — fraction of runs that loop, stall, or give up.
- **Grep-fallback rate** — fraction of runs where the agent abandons the graph for
  `grep`/`bash`. This is the exact `tool_filter.go` signal ADR-0004 cited.

**Secondary — honesty envelope**

- **Not-ready-fallback fraction** — did `ready ≠ not-found` stop the agent from
  reading a transient empty as "doesn't exist"?
- **Zero ambiguous-empty responses** — B must never emit a bare empty an agent can
  misread as absence. Target: 0.
- **Source-matches-disk** — returned verbatim body byte-equals the file. Target:
  100%. This is also where Arm C shows up: ObjectStore hydration should match the
  *ingested snapshot* the structure was derived from (no line drift).
- **Per-call latency** — wall-clock per `code_context` call. A fused response is
  several `graph.query.*` round-trips; if p95 is high, that is the empirical case for
  caching/materialization (feeds #376 Q2), not a reason to abandon the approach.

## Protocol

- **Sample size:** ≥ 10–20 runs per arm to see past model variance; vary the seed per
  run, hold everything else fixed.
- **Trajectory capture:** record the full tool-call sequence per run. Tool-call counts,
  grep-fallback detection, and source-match are computed from trajectories
  automatically; correctness is graded blind to arm.
- **Pre-registered success criteria** (fixed before running, so we don't move the
  goalposts):
  - B reduces **median tool-calls-to-solution ≥ 30%** vs A.
  - B **grep-fallback rate < ½** of A.
  - B **solve rate ≥ A** (no correctness regression).
  - B **zero** ambiguous-empty responses; **100%** source-matches-disk.
  - `code_context` **p95 latency** within an agreed bound (set with the SemSpec lead).
- **Stop / iterate:** if B misses the tool-call or fallback target, inspect whether
  the cause is the tool *shape* or its *description* (below) before concluding the
  approach failed.

## Tool exposure is half the experiment

The `tool_filter.go` post-mortem is as much about tool *description* as data: agents
ignored graph tools partly because the tools didn't read as code-native. Arm B must
present `code_context` in code-native terms ("get a function's source plus its callers
and callees") — if the description is wrong, a null result tests the description, not
the gateway. Lock the tool spec with the SemSpec lead and keep it identical across runs.

## Coordination

SemSpec owns `mavlink-hard` and the agent harness; the **SemSpec lead** drives the run.
semsource ships the standalone service, the `code_context` endpoint, and the grading
hooks (trajectory export, source-match assertion). Agree the tool spec, latency bound,
and rubric before the first run.

## Risks and confounds

- **Model variance** — mitigated by sample size and fixed model/temperature.
- **Tool-spec leakage** — the same description and tool set must be used every run;
  document it in the result set.
- **Prompt contamination** — arms must not share state across runs.
- **Grader bias** — correctness graded blind to arm; automate what can be automated.

## Deliverable

A results table (the metrics above, per arm, with CIs) that either backs or refutes
the ADR-0004 hypothesis. On a positive result it becomes the evidence section of the
upstream ADR seeded by #376; on a negative result it redirects the effort honestly.

## Out of scope

- Headless / co-located deployment (being retired; not the production topology).
- The ObjectStore migration itself — tracked separately; Arm C only *compares* it.
- Multi-lens (docs) validation — a later, separate study once code is proven.
