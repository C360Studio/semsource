---
name: query-pattern
description: Choose the right query access pattern (GraphQL, MCP, NATS Direct) for a use case. Use when designing query APIs, adding new access points, or choosing between gateway types.
argument-hint: [access scenario or caller description]
---

# Query Access Pattern Selection

## What is the access scenario?

$ARGUMENTS

## Three Access Patterns

| Pattern | Best For | Key Property |
|---------|----------|-------------|
| **GraphQL** | External apps, web frontends, data exploration | Schema-validated, field selection, introspection |
| **MCP** | AI agents, LLMs, automated reasoning | Bounded capabilities, full audit trail, structured tools |
| **NATS Direct** | Internal services, low-latency paths | No gateway overhead, lowest latency |

## Quick Decision

```
Who is calling?

  External app / web frontend  --> GraphQL
  AI agent / LLM               --> MCP
  Internal service              --> NATS Direct
  Multiple caller types         --> Combine patterns (see below)
```

## Decision Matrix

| Factor | GraphQL | MCP | NATS Direct |
|--------|---------|-----|-------------|
| Latency | Higher (HTTP) | Higher (HTTP) | Lowest (direct) |
| Schema control | Strong (SDL) | Strong (tool schemas) | Per-component |
| Auditability | Good (query logs) | Excellent (tool call audit) | Manual |
| Field selection | Yes (client picks fields) | Yes (tool parameters) | No (full response) |
| External access | Yes | Yes | No (internal only) |
| Discovery | Schema introspection | Tool list enumeration | Capability queries |
| NL query support | Yes (query classification) | Yes (query classification) | No |

## Common Combinations

| System Type | Recommended Pattern |
|------------|---------------------|
| Web app backend | GraphQL (user-facing) + NATS Direct (background jobs) |
| AI-powered system | MCP (agent access) + GraphQL (dashboard/monitoring) |
| Microservice mesh | NATS Direct (service-to-service) + GraphQL (external API) |
| Full platform | All three: NATS internal, GraphQL external, MCP for agents |

## Key Points

- All three patterns read from the same underlying knowledge graph
- Consistency is eventually consistent regardless of access pattern
- GraphQL and MCP both include natural language query classification
- MCP wraps GraphQL capabilities with bounded tool definitions and structured audit
- GraphRAG (community search) and PathRAG (structural traversal) are available through GraphQL and MCP, not NATS Direct currently

## GraphRAG vs PathRAG

When choosing query strategy (applicable to GraphQL and MCP):

| Pattern | Use When | Returns |
|---------|----------|---------|
| **GraphRAG** | Discovery, Q&A, "what do we know about X?" | Community-scoped results with summaries |
| **PathRAG** | Impact analysis, dependencies, "what's affected by X?" | Bounded traversal from known entity |

Read `docs/concepts/11-query-access.md` for full documentation.
Read `docs/concepts/09-graphrag-pattern.md` for GraphRAG details.
Read `docs/concepts/10-pathrag-pattern.md` for PathRAG details.
