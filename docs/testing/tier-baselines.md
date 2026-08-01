# Tier baselines — measured, on a real corpus

Real numbers from a real codebase, so tier guidance rests on evidence rather than
intuition. Measured **2026-08-01** against SemSource `d21752c` / SemStreams
`v1.0.0-beta.159`.

**This is not the retrieval scorecard.** The scorecard
([`scripts/scorecard/README.md`](../../scripts/scorecard/README.md)) grades the
deterministic fusion tools with a string matcher and must stay comparable across
runs. This document records boot timings and qualitative observations, including
about LLM-synthesized prose, which the scorecard deliberately refuses to grade —
a drifting judge cannot support an A/B. Keep the two apart.

## Corpus

[`opensensorhub/osh-core`](https://github.com/opensensorhub/osh-core), the
`sensorhub-core` module only — real Java, not a fixture.

| | |
| --- | --- |
| Java files | 499 |
| Entities | 6,685 |
| Entity mix | 1,932 files, 1,958 classes, 18,081 methods, 437 interfaces (full repo) |

The full repo is 1,932 Java files (~4x this). The single module keeps embedding
tractable while staying representative. **Do not linearly extrapolate** —
embedding and clustering do not scale linearly.

## Reproducing

```bash
git clone --depth 1 https://github.com/opensensorhub/osh-core.git
mkdir -p /tmp/oshws && cp -R osh-core/sensorhub-core osh-core/README.md /tmp/oshws/

# Tier 1 (semantic, no LLM) — the shipped default shape
SEMSOURCE_TARGET=/tmp/oshws docker compose up -d --wait

# Tier 2 (clustering + LLM community summaries)
SEMSOURCE_TARGET=/tmp/oshws SEMSOURCE_CONFIG=tiers/tier2-compose-dev.json \
  docker compose -f docker-compose.yml -f docker-compose.tier2-dev.yml up -d --wait
```

`tier2-compose-dev.json` is the **only** shipped config with
`enable_clustering` and `clustering_llm` true; `tier2-semantic-instruct.json`
ships them false.

## Tier-2 timings

From `docker compose up` to each milestone. Mac mini, Docker, seminstruct
`qwen3-0.6b`, image already pulled.

| Milestone | Elapsed |
| --- | --- |
| Containers healthy (nats, semembed, seminstruct, semsource) | 33 s |
| Ingest phase `ready`, 6,685 entities | 47 s |
| Structural index ready | 108 s (1.8 min) |
| Semantic index ready | **305 s (5.1 min)** |
| Community summaries + LLM answers live | **≤342 s (5.7 min)** |

**Embedding is the long pole, not the LLM.** Clustering plus LLM summarization
added roughly 37 s on top of the semantic index. A cold first run adds a ~1 GB
seminstruct image pull.

**How to tell when summaries are ready.** There is no `graph-clustering`
readiness envelope ([semstreams#820](https://github.com/C360Studio/semstreams/issues/820)),
so poll the user-visible signal: call `graph_search` and watch
`retrieval.rung` move from `entities_only` to `llm_answer`.

## Quality observations

### Structural tools: strong

| Probe | Result |
| --- | --- |
| `code_context ModuleRegistry` | Exact class, correct path, lines 77–1408, full body |
| `code_impact IModuleProvider` | **7/7 implementers, no false positives** (verified against `grep`) |

One gap: `code_impact IModule` named one implementer where the corpus has two.
The missing one, `DummyModule`, lives in `src/testFixtures/` and **is** indexed.
Given 7/7 on main-source code, this looks like test-source handling rather than a
parser miss — but it is undocumented, so an agent cannot predict it.

### `graph_search`: cheap and cross-type, ranking needs work

For *"how does a module get started and its state persisted"*:

| Tool | Bytes | Top hits |
| --- | --- | --- |
| `code_search` | 65,048 | `IModule.start`, `IModuleBase.start`, `ModuleEvent.Type`, `AbstractModule.state` — with bodies |
| `graph_search` | 7,073 | ranks 1–4 were **`folder` entities**; the useful methods sat at ranks 6–8 |

The cross-type reach is real — `graph_search` surfaced a `your-first-sensor.md`
tutorial chunk alongside code, which `code_search` structurally cannot do.

But navigational container entities (`folder`, `repo`) crowd the top of the
ranking: they match semantically because their names contain the query terms,
while carrying no content. Filtering them from results is a small, testable
improvement.

### Tier 2: honest plumbing, weak answers

The capability disclosure is **correct**, verified live at the top rung for the
first time:

```json
{"rung":"llm_answer","community_backed":true,"answer_source":"llm","answer_model":"seminstruct"}
```

The answers are another matter, and the cause is upstream of the LLM:

- **One community held 5,487 of 6,685 entities — 82% of the graph in a single
  cluster.** "Community-backed" is then technically true and epistemically close
  to meaningless.
- Summaries describe graph *structure*, not the domain: *"This community
  represents a collaborative effort by the c360.semsource organization, focusing
  on web-based entities related to workspace and chunk data."*
- The synthesized answer was fluent and partly wrong — it credited state
  persistence to "the Module Descriptor Class"; the corpus uses
  `DefaultModuleStateManager`.

`code_search` answered the same question with `IModule.start`,
`AbstractModule.state`, and the `ModuleState` enum: precise and checkable.

**Guidance: on a code corpus at this model size, the deterministic tools beat
tier 2.** Do not present tier 2 as the quality tier until clustering granularity
is addressed. The `qwen3-0.6b` model size is a plausible contributor to summary
quality, but the 82% cluster is a clustering problem no model size fixes.

## What this does NOT establish

Stated so the numbers are not over-read:

- One corpus, one language, one module. Java only; no Go/TypeScript/Python baseline.
- One machine (Mac mini, Docker), one run per milestone — no variance measured.
- One model size (`qwen3-0.6b`). Summary quality is **not** attributed to model
  size; see the A/B below.
- Tier 1 vs tier 2 answer quality was compared on a single question.
- No measurement of the full 1,932-file repo.

## Planned A/B: seminstruct 8b vs Gemini 2.5 Flash Lite

**Not yet run.** Recorded so the setup is not re-derived — and so the known
Gemini pitfalls are not rediscovered the hard way.

### Use the config shape that already works

Gemini exposes more than one wire protocol — its native
`generateContent` API, an OpenAI-compatibility layer, and Vertex AI (different
host *and* different auth). Only the compatibility layer works with a
`provider: "openai"` endpoint, and it needs more than a URL.

SemSpec already runs Gemini this way (`semspec/configs/e2e-gemini.json`). Copy
its field set rather than the minimum:

```json
"endpoints": {
  "gemini-flash-lite": {
    "provider": "openai",
    "url": "https://generativelanguage.googleapis.com/v1beta/openai",
    "model": "gemini-2.5-flash-lite",
    "api_key_env": "GEMINI_API_KEY",
    "max_tokens": 1048576,
    "supports_tools": true,
    "tool_format": "openai",
    "stream": true,
    "reasoning_effort": "low"
  }
},
"capabilities": {
  "community_summary": { "preferred": ["gemini-flash-lite"] }
}
```

The model registry resolves the key from the environment via `api_key_env`
(`model/registry.go:287`), so this is a **config-only** change — no code.
`reasoning_effort` matters: the 2.5 models are reasoning models, and SemSpec pins
it low.

### The compatibility layer is STRICTER than OpenAI

This is the trap, and SemSpec has three bug reports to show for it
(`semspec/docs/bugs/gemini-*.md`). The lesson in their own words: *"OpenAI
silently accepts duplicates; Gemini rejects them."* A request that works against
OpenAI or a local llama-server can return a hard 400 against Gemini.

| SemSpec bug | Status | Shape |
| --- | --- | --- |
| `Duplicate function declaration found: graph_summary` | **OPEN** — blocked all Gemini calls | Tool declaration |
| `Duplicate function declaration found: submit_work` | Fixed | Tool declaration |
| Planner omits `deliverable.goal`, 18+ retries | Fixed | Tool-schema adherence |

**All three are tool-calling failures, and our path does not call tools.**
Community summarization and answer synthesis issue a plain chat completion —
`ChatRequest{SystemPrompt, UserPrompt, MaxTokens, Temperature}`
(`processor/graph-query/answer.go:192`), with no `tools` array. So the known
blockers should not apply here. Verify that assumption on the first run rather
than trusting this paragraph.

### Do not run it yet

With **82% of entities in one community**, both models summarize the same
degenerate input. The A/B would measure prose style, not answer quality — and an
8b-vs-Flash-Lite result read off that would be quoted later as though it meant
something. Fix clustering granularity first.

Keep it out of the retrieval scorecard regardless: grading synthesized prose
needs a judge, and a judge drifts between runs.
