# Design: Honest Readiness and Errors

## Context

Live-verified: `phase:"ready"` appears while sources are still seeding (aggregation counts a
source as arrived on first report, `processor/source-manifest/status.go:106`); HTTP status lacks
the `index`/`embedding` objects the README points consumers at; `source_status` omits those keys
entirely when a responder is down; the MCP gateway returns ADR-060 error envelopes as successful
tool text; `entity_count`/`total_entities` are monotone publish counters inflated ×4 within
minutes by the periodic reindex.

## Goals / Non-Goals

- Goals: every readiness/success signal on every surface (NATS, HTTP, MCP tool) is truthful,
  including during the seed window and during partial infrastructure failure.
- Non-goals: removal lifecycle (separate change), retraction/staleness, new readiness *features*.

## Decisions

- **D1 — phase from completion, not arrival**: aggregate `phase` = `seeding` until every
  configured source has reported initial-seed completion (sources already emit this transition;
  the aggregator tracks per-source phase ∈ {ingesting → watching/idle}); `ready` requires all
  sources past seed; any errored source ⇒ `degraded`. The seed-window test asserts `seeding` is
  observable between first report and last completion.
- **D2 — one status assembly**: extract the MCP `source_status` composition (status + index +
  embedding + note) into a shared function used by both the MCP tool and the HTTP
  `/source-manifest/status` handler. README's documented polling contract becomes true rather
  than re-documenting the gap. Alternative (docs-only fix) rejected: consumers (semspec) poll
  HTTP; parity is cheap and removes a whole class of drift.
- **D3 — explicit unavailability**: failed index/embedding sub-queries yield
  `{available: false, reason: "<error>"}` instead of key omission; the `note` text is corrected to
  scope its "miss = genuine absence" claim to `phase == ready && index.ready`.
- **D4 — ADR-060 mapping in the gateway**: replace bare `nc.Request` with a helper that inspects
  reply headers (X-Status) and, when absent, sniffs the ADR-060 envelope shape
  (`{"message": ...}` without `contract_version`); either maps to an MCP `isError` result carrying
  the envelope message. If the current client API cannot expose headers, the fallback sniff alone
  still closes the hole; a framework ask for first-class error propagation goes to
  docs/upstream/semstreams-asks.md.
- **D5 — distinct-entity counts**: each source tracks the set of entity IDs it has confirmed
  (bounded by corpus size; ast-source already holds per-file hash maps of similar scale);
  `entity_count`/`type_counts`/`total_entities` report set cardinality. Republishing an unchanged
  entity does not change counts. A separate `publish_total` remains available for throughput
  visibility (not in the headline fields).

## Risks / Trade-offs

- [Ready arrives later for consumers] — that is the fix; semspec heads-up in PR notes (timing
  change only, shape unchanged).
- [ID-set memory] bounded by entity count per source (~4k for this repo; ~100k worst envisioned)
  → acceptable; if a future source exceeds it, switch to a counting filter then.
- [Header API uncertainty] D4's envelope sniff is the safety net either way.

## Migration Plan

Ship together (single PR); consumers see: later-but-true `ready`, richer HTTP status (additive
fields), stable counts. Rollback = revert.

## Open Questions

- none blocking.
