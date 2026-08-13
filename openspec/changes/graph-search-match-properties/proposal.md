# Proposal: graph-search-match-properties

## Why

Config-entity property values (a dependency's version, kind, scope, configuration) are
unreachable across the entire MCP tool surface in one call: `graph_search` match rendering
carries only id/type/label, and no other tool reads config-entity properties (#166, the
surviving half of #142). On a Gradle corpus an agent cannot answer "what version of
spring-core does this build use" even though the graph holds the triple — the substrate's
common response path already returns the full entity triples, and the gateway discards
everything except `dc.terms.title`.

## What Changes

- `graph_search` matches gain a bounded `properties` map, populated from an explicit
  allowlist of value-bearing predicates, ONLY when the substrate response already carries
  entity triples (the entities path). No hydration round-trips; the digest and ID-only
  paths are unchanged (they carry no triples to render).
- The allowlist is small and value-shaped (config family values plus artifact version);
  per-match property count and per-value length are capped, honoring the standing
  anti-bloat decision in `graphSearch` (a 38-hit query once returned 197 KB of raw
  triples — 11x code_search; a search verb ranks, it does not transfer the corpus).
- `questions-osh.json` bumps to version 2 with a config band (name-level and value-level
  questions) as the empirical gate: value questions must FAIL on the current build and
  PASS with the change. Version 2 is a new comparability domain per scorecard rules.
- Context-bytes growth is measured (scorecard arm B before/after on both corpora) and
  recorded in the change; the MCP schema budget gate (#161) is unaffected (no schema
  change — `properties` is response-shape, not input-schema).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `graphrag-access`: the "Results are citable" surface grows a sibling requirement —
  matches carry a bounded, allowlisted set of the entity's own property values when the
  substrate response includes the entity's triples, so value-shaped facts are answerable
  in one call without transferring the corpus. Nothing is invented: values are the
  substrate's own triple objects, and absence (digest/ID-only paths) stays absence.

## Impact

- `processor/mcp-gateway/graph_matches.go` (rendering + allowlist), `graph_tools_test.go`
  / new unit tests (properties present on the entities path, absent elsewhere, caps
  enforced).
- `scripts/scorecard/questions-osh.json` v2 + README config-band note (currently says a
  config band is "authorable at name level only" — this change makes value-level
  authorable; both checkers re-verified against the OSH pin).
- No NATS contract, payload, or config change; no semstreams surface touched
  (substrate-owned rule holds — rendering only reads what the response already carries).
- GitHub: closes #166; updates the scorecard README's composition-band "inadmissible
  shapes" note (cross-source joins become partially admissible at name+value level).
