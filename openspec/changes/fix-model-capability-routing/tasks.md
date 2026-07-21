# Tasks — making capability routing honest

Config and validation only. No graph rebuild, no reindex, no consumer contract change.

## 1. Classify endpoint role

- [x] 1.1 Add role classification in `config/`: an endpoint is embeddings-serving when it
      carries `query_prefix` or its model name marks it as an embedding model; generative
      otherwise (design D1)
- [x] 1.2 Unit-test the classifier against the two endpoints actually shipped (`semembed` with
      `query_prefix`, `seminstruct`/`qwen3-0.6b` without) plus an endpoint with neither signal
- [x] 1.3 Record in the code comment that this is inference, not a probe, and why —
      `semsource validate` must work offline, so it cannot depend on a running service

## 2. Reject a misroute at config load

- [x] 2.1 Extend `config.validateModelRegistry` with the role check, beside `requireCapability`
      where the same principle already lives (design D2)
- [x] 2.2 Check every capability that resolves at all, not only the two a selected tier needs —
      that gap is the defect
- [x] 2.3 Error names the capability, the endpoint it resolved to, and **both** remedies: bind
      it to something that can serve it, or leave it unbound to degrade (design D3)
- [x] 2.4 Reject rather than silently unbind — quietly degrading fixes runtime behaviour and
      leaves the config asserting something untrue
- [x] 2.5 Test that `semsource validate` AND `semsource run` both fail, before any component
      starts

## 3. Fix the shipped configs

- [x] 3.1 Drop `defaults.model` from `configs/mvp.json` and `configs/tiers/tier1-semantic.json`
      (design D4). `embedding` is explicitly declared in both, so the exercised path is
      unaffected
- [x] 3.2 Bind `query_classification` and `answer_synthesis` → `seminstruct` in
      `configs/tiers/tier2-semantic-instruct.json` (design D5)
- [x] 3.3 Do **not** bind `anomaly_review` — its consumer runs only under clustering, which
      ships off. Binding it would invent coverage for something nothing runs
- [x] 3.4 Confirm every config still loads: `semsource validate --config <each>` passes for all
      of them

## 4. Make the class impossible

- [x] 4.1 Add the role-compatibility test that **globs** `configs/*.json` and
      `configs/tiers/*.json` rather than enumerating them (design D6) — a hand-written list
      passes forever while a new config ships unchecked
- [x] 4.2 Load each through the real loader, so the test exercises the shipped path and not a
      reimplementation of it
- [x] 4.3 Prove the test catches the bug it exists for: with `defaults.model: semembed`
      restored, it must fail on `query_classification`/`answer_synthesis`
- [x] 4.4 Assert the corollary too — an unbound LLM capability passes, so the test cannot be
      satisfied by simply binding everything

## 5. Document the degradation contract

- [x] 5.1 In `configs/tiers/README.md`, state that an unmapped LLM capability is a **supported
      state that degrades** (keyword-only classifier, template synthesis), not an omission to
      paper over with a catch-all
- [x] 5.2 Note that `defaults.model` is a documented framework feature being deliberately left
      unset here, and why — upstream's own guidance is to bind capabilities explicitly rather
      than reshape the default
- [x] 5.3 State plainly that tier-2 binding these capabilities makes the configuration
      **honest, not proven**: the GraphRAG path they serve is still unexercised. The README must
      not imply otherwise

## 6. Gates

- [x] 6.1 `task lint` clean — revive warnings fail CI, pinned v1.15.0
- [x] 6.2 `go test ./...` and `go test -race ./config/`
- [x] 6.3 `openspec validate --all`
- [ ] 6.4 Bring up the default profile and confirm the misleading startup log is gone — no
      "LLM query classifier enabled" or "LLM answer synthesis enabled" naming an embedding
      model. That log line is what surfaced this, so it is the honest end-to-end check
