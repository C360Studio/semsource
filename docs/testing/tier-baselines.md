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

## Clustering edge synthesis, measured on a multi-repo corpus

The 82% cluster above was measured on a **single repo**, where every entity shares one `system`
segment. That made system-peer edge synthesis degenerate by construction, so the obvious question
was whether a multi-repo graph — where `system` actually varies — behaves differently. It does not
behave *better*; it fails a different way.

### The corpus

Three repos, one source root each, so `system` varies. osh-core is scoped to `sensorhub-core` +
README, matching the single-repo baseline above, so the added repos are the only new variable.

| repo | contributes | entities |
| --- | --- | --- |
| `opensensorhub/osh-core` | 499 `.java`, 8 `.md` | 8,725 (68%) |
| `opengeospatial/ogcapi-connected-systems` | 798 `.adoc`, 19 `.md` | 2,994 (23%) |
| `meshtastic/firmware` | 40 `.py`, 37 `.md` | 981 (8%) |
| | | **12,798 total** |

**Parser-coverage skew, stated plainly:** Meshtastic is a C/C++ codebase and we parse neither, so
its **1,299 `.c/.cpp/.h` files produce zero entities**. This is a *multi-system* corpus, not a
balanced polyglot one — 68% Java symbols, 23% AsciiDoc passages, 8% incidental. It answers the
question about the `system` segment; it does not establish anything about C/C++ retrieval.

> **Superseded for the corpus, not for the result (2026-08-01).** C and C++ parsers have since
> landed, so those 1,299 files are no longer invisible: the same Meshtastic tree now yields
> **32,312 `cpp` entities**, and a MAVLink tree added alongside it yields **12,104 `c`** — both
> measured on a live stack. The clustering numbers above are left exactly as measured, because they
> were taken on the corpus as it stood; they are not restated for the new shape. What this does mean
> is that the corpus is no longer 68% Java — a re-measurement of clustering on a C/C++-heavy graph
> is unmeasured work, not a footnote, and the "one blob per repo" finding should not be assumed to
> carry over unchanged.

A second wrinkle: 21 systems appear, not 3. The extra 18 (52 entities at most, ~0.6% combined) come
from source roots resolving to a nested directory base name. So `system` is not exactly `repo` even
when configured one root per repo.

### The result — one blob per repo

Level 0 of the community hierarchy, one variable changed per run, entity count and community count
both stable before reading:

| `include_system_peers` | communities | largest community | single-system communities |
| --- | --- | --- | --- |
| `true` (substrate default), max 15 | 14 | **8,823 — 66.1%** | 13 of 14 |
| `true`, `max_system_peers: 3` | 22 | 6,067 — 47.4% | 21 of 22 |
| **`false`** (what tier 2 now ships) | 19 | 6,069 — 47.4% | 14 of 19 |

With the default, **every one of osh-core's 8,725 entities lands in one community, and every one of
ogcapi's 2,994 in another.** Multi-repo does not rescue the default — it partitions the collapse
along repo lines, which a `system` filter would give without running label propagation at all.

Capping peers to 3 recovers the entire benefit of turning them off, so the harm is in the *number*
of synthesized peers rather than in the concept.

### Two honest limits on this result

- **It does not reproduce the single-repo "largest 93".** There, turning peers off left almost no
  edges. Here the remaining 6,069-entity community is held together by **sibling** synthesis
  (same 5-part type prefix — every `…java.osh-core.class.*`), which both arms leave at its default.
  Off is an improvement and the right default; it is not a fix for clustering granularity.
- **Summary quality is untouched by any of this**
  ([semstreams#829](https://github.com/C360Studio/semstreams/issues/829)). Better clusters are not
  better answers.

### It also hits a hard ceiling — [semstreams#837](https://github.com/C360Studio/semstreams/issues/837)

On stock NATS (1 MiB `max_payload`) the **default configuration cannot complete a detection pass on
this corpus**: LPA builds a community whose serialized member list exceeds the limit,
`SaveCommunity` fails, and the whole pass is discarded — 2 communities holding 654 entities (4.4%),
with the component still reporting healthy.

Community records serialize at ~148 bytes/member, so ~7,070 members is the ceiling. Worth knowing
even with peers off: the largest record in that arm is 886,692 B — **84.6% of the 1 MiB limit** at
only 12,798 entities. Measurements above therefore ran with `max_payload: 33554432` on both arms.

### Reproducing it

Build the corpus with one directory per repo, point one source root at each (that is what makes
`system` vary), and run tier 2 over it:

```bash
mkdir -p /tmp/multi && cd /tmp/multi
git clone --depth 1 https://github.com/opensensorhub/osh-core            # keep sensorhub-core + README
git clone --depth 1 https://github.com/meshtastic/firmware               meshtastic-firmware
git clone --depth 1 https://github.com/opengeospatial/ogcapi-connected-systems
rm -rf ogcapi-connected-systems/swagger-ui ogcapi-connected-systems/redoc  # vendored minified bundles
```

Then read the result:

```bash
go run ./scripts/measure-communities -nats nats://localhost:4222 -label mine
```

It prints entities per system, the size distribution for one hierarchy level, and each community's
system mix. Two traps it exists to avoid: `COMMUNITY_INDEX` stores every level plus an
`entity.<level>.<id>` reverse index, so reading all keys double-counts entities once per level; and
a detection pass writes progressively, so an early read catches a partial result — wait for the key
count to hold steady.

## What this does NOT establish

Stated so the numbers are not over-read:

- One corpus, one language, one module. Java only; no Go/TypeScript/Python baseline.
- One machine (Mac mini, Docker), one run per milestone — no variance measured.
- One model size (`qwen3-0.6b`). Summary quality is **not** attributed to model
  size; see the A/B below.
- Tier 1 vs tier 2 answer quality was compared on a single question.
- No measurement of the full 1,932-file repo.

## Planned A/B: seminstruct 8b vs Gemini 2.5 Flash Lite

**Not yet run.** Recorded so the setup is not re-derived — and so the Gemini
wire-protocol traps are not rediscovered the hard way.

### Verify field names against the pinned SemStreams, not against a sibling

`model.EndpointConfig` at `v1.0.0-beta.159` accepts:

```
provider  url  model  query_prefix  max_tokens  max_output_tokens
supports_tools  tool_format  api_key_env  options  stream  reasoning_effort
input_price_per_*  output_price_per_*  requests_per_minute  max_concurrent
request_timeout  idle_conn_timeout  response_header_timeout
disable_keepalives  wire_backend
```

**Unknown fields fail loudly, at every level.** `config/loader.go` decodes with
`DisallowUnknownFields`, and it applies inside endpoints too — a bogus key under
`model_registry.endpoints.<name>` fails `semsource validate` with
`json: unknown field "..."`. That is the good outcome: a config copied from an
older SemStreams cannot silently lose a setting, it refuses to load. Run
`semsource validate` after any config transplant and believe it.

SemSpec has a working Gemini config (`semspec/configs/e2e-gemini.json`) but it is
**stale**: last touched 2026-04-04, and it pins SemStreams `v1.0.0-alpha.92`
against our `v1.0.0-beta.159`. Its field set still validates at beta.159, and
`semsource validate` is what proves that — not inspection.

### `wire_backend` is the Gemini-specific knob

Gemini has more than one wire protocol, and SemStreams models this per endpoint:

| `wire_backend` | Client |
| --- | --- |
| `""` / `"sdk"` (default) | `sashabaranov/go-openai` SDK |
| `"wire"` | Framework-owned `model/wire` (ADR-037) |
| `"responses"` | OpenAI Responses surface (ADR-051) |

**Gemini is ADR-037's named motivating case.** Its rationale: Gemini 3.x preview
requires `thought_signature` echo on multi-turn tool flows, the SDK's typed
`ToolCall` struct cannot carry the field, and Gemini 2.5 has a finite runway. The
field is per-endpoint precisely so operators "flip Gemini to `wire` first while
other providers stay on SDK".

Our tier-2 summary path **does** honor it — `graph/llm/openai_client.go:126`
threads `EndpointConfig.WireBackend` into the client. So `"wire"` is available to
us if the SDK path misbehaves.

### Starting config

```json
"endpoints": {
  "gemini-flash-lite": {
    "provider": "openai",
    "url": "https://generativelanguage.googleapis.com/v1beta/openai",
    "model": "gemini-2.5-flash-lite",
    "api_key_env": "GEMINI_API_KEY",
    "max_tokens": 1048576,
    "reasoning_effort": "low",
    "wire_backend": "sdk"
  }
},
"capabilities": {
  "community_summary": { "preferred": ["gemini-flash-lite"] }
}
```

Config-only — the registry resolves the key via `api_key_env`
(`model/registry.go:287`). `reasoning_effort` matters: the 2.5 models are
reasoning models. Start on `sdk`; move to `wire` if you hit protocol errors.

### The compatibility layer is stricter than OpenAI

SemSpec has three Gemini bug reports (`semspec/docs/bugs/gemini-*.md`), and the
lesson in their own words is *"OpenAI silently accepts duplicates; Gemini rejects
them."* A request that works against OpenAI or a local llama-server can hard-400
against Gemini.

| SemSpec bug | Status | Shape |
| --- | --- | --- |
| `Duplicate function declaration found: graph_summary` | **OPEN** — blocked all Gemini calls | Tool declaration |
| `Duplicate function declaration found: submit_work` | Fixed | Tool declaration |
| Planner omits `deliverable.goal`, 18+ retries | Fixed | Tool-schema adherence |

**All three are tool-calling failures, and our path does not call tools.**
Community summarization and answer synthesis issue a plain chat completion —
`ChatRequest{SystemPrompt, UserPrompt, MaxTokens, Temperature}`
(`processor/graph-query/answer.go:192`), no `tools` array. The known blockers
should not apply. Verify on the first run rather than trusting this paragraph.

### Do not run it yet

With **82% of entities in one community**, both models summarize the same
degenerate input. The A/B would measure prose style, not answer quality — and an
8b-vs-Flash-Lite result read off that would be quoted later as though it meant
something. Fix clustering granularity first.

Keep it out of the retrieval scorecard regardless: grading synthesized prose
needs a judge, and a judge drifts between runs.
