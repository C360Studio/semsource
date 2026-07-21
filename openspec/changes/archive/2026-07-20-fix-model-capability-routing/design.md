# Design — making capability routing honest

## Context

`proposal.md` has the defect and the mechanism. Two things found while designing change what
this should build.

### The reasoning already exists in this codebase — it was just not applied everywhere

`config.requireCapability` (`config/config.go:283`) carries this comment:

> We require an explicit capability rather than accepting `reg.Resolve`'s fallback to
> `defaults.model`: that fallback would silently route, say, `community_summary` to the
> embedding endpoint (`defaults.model` is typically the embedder), **which starts cleanly and
> then produces garbage**. Demanding the operator declare the capability is the honest
> fail-fast.

That is precisely this defect, correctly diagnosed, already in the tree. It is enforced for
exactly two capabilities — `embedding` when `embedder_type: "http"`, and `community_summary`
when `clustering_llm` — because those are the two a *selected tier* needs. Nothing checks the
capabilities a tier does not need but the registry silently binds anyway.

So this is not a new principle to introduce. It is an existing principle with a gap, and the
design should close the gap in the place the principle already lives rather than inventing a
second mechanism beside it.

### The framework is not at fault, and the docs say so

`semstreams docs/operations/05-model-registry.md` documents `defaults.model` as a catch-all
"used when a capability resolves nowhere else", tells operators to "bind the capability
explicitly rather than reshaping `defaults.model`", and marks `semembed` as an
"HTTP embedder, **not** chat completions — different protocol". There is no upstream ask here;
we set a documented catch-all to an endpoint documented as a different protocol.

## Goals / Non-Goals

**Goals:**

- No shipped config binds a capability to an endpoint that cannot serve it.
- A misrouted capability is caught mechanically, not by reading a startup log.
- An unbound LLM capability degrades as the framework documents, rather than being papered
  over by a catch-all.

**Non-Goals:**

- Making the GraphRAG path work end to end.
- Wiring an LLM into the default profile; the MVP stays embeddings-only (ADR-0004).
- An upstream ask — see above.
- Changing `Registry.Resolve`. Substrate, and correct as documented.

## Decisions

### D1 — Classify endpoints by role, declared by us, from what the endpoint is

The check needs to know that `semembed` serves embeddings and `seminstruct` serves chat.
Nothing in `EndpointConfig` states this: it has `provider`, `url`, `model`, `query_prefix`,
`max_tokens`. `provider: "openai"` is true of both.

The signal used is **`query_prefix`**, which only an asymmetric retrieval embedder needs, plus
the endpoint's model name. Both are already present and neither is invented for this check.

*Alternative rejected:* infer from URL or port (`:8081` vs `:8083`). Positional and would break
the moment someone reverse-proxies or renames.

*Alternative rejected:* add a `role` field to our config schema. It duplicates a substrate type
we do not own, and a field an operator must remember to set is a field they will forget — which
is how this defect happened.

*Alternative considered and deferred:* probe the endpoint at startup (`GET /v1/models`). It
would be authoritative rather than inferred, but turns config validation into a network
operation and makes `semsource validate` fail when a service is merely down. Recorded as an
open question, not adopted.

### D2 — Enforce in `validateModelRegistry`, extending the existing rule

The check lands in `config.validateModelRegistry`, beside `requireCapability`, and fails at
config load — so `semsource validate` and `semsource run` both catch it before any component
starts, consistent with how every other identity- and tier-shaped config error is handled here.

The new rule is the complement of the existing one:

- **Existing:** a capability a selected tier *needs* must be explicitly declared.
- **New:** a capability that *resolves at all* must resolve to an endpoint that can serve it.

Both are one idea — a capability's binding must be real — so they belong in one function.

### D3 — Reject the misroute; do not silently unbind it

When an LLM capability resolves to an embedding endpoint, fail with an error naming the
capability, the endpoint, and the two ways out: bind it to a generative endpoint, or drop
`defaults.model` so it degrades as documented.

*Alternative rejected:* treat the misroute as "unbound" and quietly degrade. It would fix the
runtime behaviour and leave the config lying about what it does — the same class of problem
one layer up. An operator who wrote `defaults.model: semembed` deliberately deserves to be
told it cannot serve `answer_synthesis`, not to have it ignored.

### D4 — Drop `defaults.model` rather than binding LLM capabilities in mvp/tier‑1

Two ways to satisfy D3 in the embeddings-only configs: bind every LLM capability to something
generative (there is nothing to bind to), or remove the catch-all. The second is right and is
the documentation's own advice.

`embedding` is explicitly declared in both configs, so removing `defaults.model` changes
nothing about the path that is actually exercised. Verified: `Resolve` then returns `""`,
`GetEndpoint` returns nil, `ResolveEndpoint` errors, and the component takes the documented
unbound path — keyword-only classifier, `TemplateAnswerSynthesizer`.

### D5 — In tier‑2, bind only what tier‑2 actually runs

Tier‑2 has `seminstruct`, so the misroute is fixed by binding rather than unbinding. But bind
deliberately, not reflexively:

- `query_classification`, `answer_synthesis` — bind to `seminstruct`. These are graph-query's,
  and tier‑2 exists to make exactly this work.
- `anomaly_review` — **do not bind.** Its consumer is graph-inference's ReviewWorker via
  graph-clustering, and SemSource ships clustering **off** (`enableClustering := false`). The
  upstream table says it "falls through to the `community_summary` endpoint (legacy
  piggyback)", but no such piggyback exists in `Registry.Resolve` — `buildChain` reads only the
  capability's own `preferred`/`fallback`. So today it lands on `defaults.model`, and once that
  is gone it is cleanly unbound. Binding a capability nothing runs would be inventing coverage.

The proposal listed `anomaly_review` among the three to bind. This narrows that, because the
check made the question concrete.

### D6 — The test asserts the property, over every shipped config, by discovery

A table test that enumerates configs by hand would pass forever while a new config ships
unchecked. Instead: **glob `configs/*.json` and `configs/tiers/*.json`**, load each through the
real loader, and assert the role property. A new config is covered the day it lands, without
anyone remembering to add it.

This is the item the proposal calls the actual fix. D4 and D5 correct today's instances; D6 is
what makes the class impossible.

## Risks / Trade-offs

- **[Role inference is heuristic]** → It is, and D1 says so. **Corrected during
  implementation: the inference is only sound in ONE direction, and the design originally
  assumed both.** A positive signal (`query_prefix`, or a known embedding family in the model
  name) is strong evidence an endpoint serves embeddings; its *absence* is not evidence of the
  reverse, because an unrecognised embedder looks identical to a chat endpoint. Checking that
  `embedding` resolves to something recognisably an embedder rejected `model: "arctic-s"` — a
  real embedding model — in this package's own fixtures. That direction is dropped: a chat
  endpoint bound to `embedding` fails loudly at first use, whereas the defect this change
  exists for is silent and deferred. Only the sound direction is enforced.
- **[Dropping `defaults.model` changes behaviour for an operator who relied on it]** → It
  changes behaviour from "chat requests to an embeddings endpoint" to "documented graceful
  degradation". Both were unexercised. Called out in the tiers README so the intent is on the
  record.
- **[Validation rejects a config that used to load]** → Only if that config routes a capability
  to an endpoint that cannot serve it, which never worked. Failing at `semsource validate` is
  the point.
- **[This makes tier-2's LLM path look tested when it is not]** → Real risk. Binding the
  capabilities makes the config *honest*, not *proven*. The proposal's first non-goal says so;
  the tiers README must not imply otherwise.

## Migration Plan

Config-only. No graph rebuild, no reindex, no consumer contract change. An operator with a
custom config that misroutes a capability will get a validation error naming the fix — that is
the intended breaking surface, and it is caught at `semsource validate` before anything starts.

## Open Questions

- **Should the role check probe endpoints at startup instead of inferring?** `GET /v1/models`
  would be authoritative, but makes config validation depend on a running service and would
  fail `semsource validate` when a service is merely down. Inference is chosen for now
  precisely because it works offline; revisit if inference ever misclassifies a real endpoint.
- **Should `defaults.model` be disallowed outright in shipped configs?** D3 rejects the
  misroute, which is narrower. A blanket ban would be simpler to reason about but would remove
  a legitimate framework feature we may want when a genuinely general-purpose endpoint exists.
- **Does anything actually exercise `query_classification` / `answer_synthesis`?** They live on
  graph-query's GraphRAG path (`graphrag.go`), which `doc_context` / `code_context` never
  reach. Worth establishing before tier‑2 claims these work — but that is the proposal's
  non-goal, and it needs its own measurement.
