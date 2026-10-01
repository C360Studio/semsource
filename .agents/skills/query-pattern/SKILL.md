---
name: query-pattern
description: Choose a declared SemSource MCP/HTTP operation or a typed graph query contract for a caller.
argument-hint: [access scenario or caller description]
---

# SemSource query access patterns

This deliberate fork describes SemSource's shipped adapters. The framework has no canonical graph
MCP front door; SemSource owns `processor/mcp-gateway` and its bounded tool contracts. Do not infer a
capability from protocol choice, a registry import, or an advertised operation name.

## Select an implemented operation

| Caller | Surface | Contract source |
| --- | --- | --- |
| Agent | SemSource MCP tools | `processor/mcp-gateway` and `openspec/specs/mcp-gateway-contract` |
| Local HTTP consumer | SemSource code/doc context, source/status routes | Owning processor and its tests |
| Internal graph consumer | Named typed adapter or declared `graph.query.*` operation | Exact pinned operation request/response and readiness contract |
| UI | Configured graph HTTP gateway | Its implemented operation tests; do not assume a conformant GraphQL executor |
| Operator diagnosing state | Explicitly declared bucket or broker inspection | Owner's diagnostic contract; not a new application fallback |

For every caller, identify the operation's owner, source of answers, scope, result limit, hydration
behavior, freshness/readiness requirements, and error/partial-result contract. Use existing typed
adapters when the pin supplies them. SemSource's existing declared NATS operations remain supported
consumer contracts; raw KV reads must not silently replace a failed public query.

## Correctness before interface choice

- Exact entity state, materialized indexes, and neural/lexical search have different freshness signals.
  A source being ready does not establish that its indexes are ready. Check the signal the operation owns.
- Unavailable providers, transport errors, partial hydration, or `Deferred` are not absence findings.
- Apply source/taxonomy scope before result limits. Under ADR-102, taxonomy is segment four; a wildcard
  is not a literal prefix. Use complete admitted scopes, not broad search followed by post-limit filtering.
- Code/doc fusion and `graph.query.searchGraph` are distinct paths; qualify both when a consumer needs both.
- MCP does not automatically wrap GraphQL, provide durable audit, or admit generation. SemSource's
  semembed profile provides neural retrieval; optional generation has a separate qualification contract.

## Verify against the pinned implementation

Read the matching capability spec and producer/consumer tests. For framework operations, inspect the
module directory reported by `go list -m -f '{{.Dir}}' github.com/c360studio/semstreams`; ADR aspirations
may exceed behavior shipped at that pin. The SETUP 03A crosswalk is in
`docs/testing/setup-03a/compatibility.md` and its evidence must not be generalized to untested operations.
