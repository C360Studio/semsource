# Fusion tools disclose the merge answer

Approved-by: Coby Leuschke (2026-11-26) sha256:c62fe6e8aca0
Track: small (documents and pins a field the substrate already returns; no code path changes).

## Why

#377, triaged into milestone D on 2026-11-25. SemEngine alpha.8 (semengine#723, our ask) added `Merge` to
`fusion.Response`, and it is set for natural-language resolves (`pkg/fusion/engine_lens.go:144-150`). The MCP gateway
returns the fusion response JSON verbatim (`processor/mcp-gateway/query_tools.go:124-150`). So `code_search`,
`code_context` and `doc_context` already return `merge` today. A probe of
`TestIntegration_FusionRepliesCarryMergedSearchProvenance` on main 2b4cd23 shows
`"merge":{"honoured":false,"detail":"merged","reason":"embed the query: provider answered 503","code":"internal"}` beside
`provenance: lexical`.

What is stale is the contract around it:
- The spec still says `provenance: lexical` cannot tell a word-list fallback from an applied merge with no embedding
  seed (tier1-consumer-qualification, "Fusion tools disclose where Tier 1 seeds came from" and the readiness
  requirement).
- The three tool descriptions say "`lexical` may be either (semengine#723)" (`processor/mcp-gateway/component.go:93,101,105`).
- No test asserts that `merge` is present, so a gateway that started re-encoding the response could drop it silently.

## What Changes

- MODIFIED requirement: at Tier 1 the fusion tools SHALL return the fusion response's `merge` for natural-language
  questions, and a symbol or prefix question carries none. The "cannot tell" caveat is removed from both requirements.
- The three tool descriptions point at `merge.honoured` instead of the caveat, within the 8,192-byte tools/list budget
  (`processor/mcp-gateway/schema_budget_test.go:18`).
- The provenance integration test asserts `merge` on each reply: honoured on a merged answer, not honoured with its
  reason and code when the provider fails, and absent at Tier 0.

## Decisions you are making by approving

- `merge` on the fusion tools becomes part of SemSource's consumer contract; dropping it later is a breaking change.
- The field is passed through exactly as SemEngine shapes it (`honoured`, `detail`, `reason`, `code`); SemSource does
  not rename or reshape it.

## Out of scope

- `graph_search`, which already surfaces `merge` and is unchanged.
- Any Tier 0 change: no merge runs there, so no `merge` appears.
