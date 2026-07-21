# Every shipped config routes LLM capabilities to an embeddings model

## Why

`graph-query` logs this on every start of the default profile:

```
"LLM query classifier enabled"  model=Snowflake/snowflake-arctic-embed-s
"LLM answer synthesis enabled"  model=Snowflake/snowflake-arctic-embed-s
```

That is nonsense on its face. `arctic-embed-s` is an embeddings model; it has no
`/v1/chat/completions` and cannot classify or synthesize anything.

**Mechanism.** `Registry.Resolve` (semstreams `model/registry.go:572-582`) returns
`defaults.model` for any capability not explicitly mapped. Every SemSource config sets
`defaults.model: semembed`. So every LLM capability nobody mapped silently resolves to
the embeddings endpoint.

| capability | mvp / tier‑1 | tier‑2 | consumer |
| --- | --- | --- | --- |
| `embedding` | semembed ✅ | semembed ✅ | graph-embedding |
| `community_summary` | semembed ⚠️ | seminstruct ✅ | graph-clustering |
| `query_classification` | semembed ❌ | **semembed ❌** | graph-query |
| `answer_synthesis` | semembed ❌ | **semembed ❌** | graph-query |
| `anomaly_review` | semembed ⚠️ | **semembed ❌** | graph-clustering |

**`tier2-semantic-instruct.json` is the case that matters.** Its entire purpose is adding a
generative model. `seminstruct` / `qwen3-0.6b` is configured and correctly mapped for
`community_summary` — and the two capabilities most obviously needing it still fall through
to `semembed`.

**This defeats a deliberate design.** The consuming components degrade gracefully when a
capability is unconfigured: `graph-query` falls back to a keyword-only classifier and a
`TemplateAnswerSynthesizer`, and its own comment states the intent — *"No
query_classification capability configured — keyword-only is fine"*. `defaults.model`
converts **"unconfigured, degrade safely"** into **"configured wrong, fail at call time"**.

Verified: with no `defaults.model`, `Resolve` returns `""`, `GetEndpoint` returns nil,
`ResolveEndpoint` errors, and the component takes its designed fallback. Removing the
default *is* the fix, not a workaround.

**Why now.** Nothing is broken in the shipped path today — `doc_context` / `code_context`
go through `pkg/fusion`, not graph-query's GraphRAG path, and clustering is off by default
(`enableClustering := false`). That is precisely the problem: these routes are wholly
unexercised, so a config that reads as configured has never been shown to work. The defect
was found by reading a startup log during unrelated work, which is not a control.

## What Changes

- **Drop `defaults.model`** from `configs/mvp.json` and `configs/tiers/tier1-semantic.json`.
  Unmapped LLM capabilities then degrade to keyword-only and template synthesis, as the
  components intend. `embedding` is explicitly mapped in both, so it is unaffected.
- **Map the LLM capabilities in `tier2-semantic-instruct.json`** — `query_classification`,
  `answer_synthesis`, `anomaly_review` — to `seminstruct`, which is already configured there.
- **Add a role-compatibility test over every shipped config.** For each config in `configs/`
  and `configs/tiers/`, assert every capability resolves to an endpoint whose role matches:
  embedding capabilities to an embedding endpoint, LLM capabilities to a generative endpoint
  or to nothing at all. **This is the actual fix** — the first two items correct two
  instances; this makes the class impossible.
- **Document the degradation contract** in `configs/tiers/README.md`: an unmapped LLM
  capability is a supported state that degrades, not an omission to be papered over with a
  default.
- **File an upstream ask.** `defaults.model` silently defeats a deliberate degradation path,
  and there is no way for a producer to express "this capability has no model, use your
  fallback" other than by leaving the default unset — which is easy to get wrong and
  impossible to detect from the config alone.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `runtime-configuration`: today it requires that SemSource has one runtime configuration and
  that ID-shaped values are validated at load. Add the model registry to what configuration
  correctness means — a capability must resolve to an endpoint that can actually serve it,
  and an unmapped LLM capability must degrade rather than silently bind to whatever the
  default happens to be.

## Impact

- `configs/mvp.json`, `configs/tiers/tier1-semantic.json`,
  `configs/tiers/tier2-semantic-instruct.json` — registry blocks only.
- A new test over the shipped configs (`config/` package).
- `configs/tiers/README.md` — the degradation contract.
- `docs/upstream/semstreams-asks.md` — the `defaults.model` ask.
- **No product code changes expected.** If the role-compatibility test proves otherwise, the
  design says so rather than widening quietly.

## Non-goals

- **Making the GraphRAG path work end to end.** This change makes the configuration honest;
  proving `query_classification` and `answer_synthesis` actually serve a useful answer against
  `seminstruct` is separate work with its own measurement.
- **Wiring an LLM into the default profile.** The MVP stays embeddings-only. `doc_context`
  returns evidence and the calling agent reasons over it (ADR-0004); that division is
  deliberate and unchanged.
- **Changing `Registry.Resolve`.** Substrate. The outcome there is a GitHub issue and an entry
  in `docs/upstream/semstreams-asks.md`, never a PR.
- **Re-tuning retrieval.** Nothing here touches ranking, chunking, or the scorecard.

## Consumers

Operators choosing a tier, and anyone running `tier2-semantic-instruct.json` expecting the
instruct model to be used for instruct work. No agent-facing contract changes: `code_context`
/ `doc_context` over MCP and HTTP are unaffected, because they never reach these capabilities.
