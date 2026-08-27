---
name: orchestration-check
description: Determine whether logic belongs in a reactive rule, workflow, or component. Use when adding orchestration logic, designing multi-step processes, or reviewing boundary violations.
argument-hint: [pattern or logic being evaluated]
---

# Orchestration Layer Check

## What pattern are you evaluating?

$ARGUMENTS

## The Two Layers

| Layer | Responsibility | Owns | Does NOT Own |
|-------|---------------|------|--------------|
| **Rule engine** | State detection, triggers, bounded action sequencing | Conditions, actions, match state, iteration caps | Actual work execution or semantic judgment |
| **Lifecycle harness** | Declared phase discipline for named entities | Phase graph, transition validation, operator-writable state contract | Work execution or hidden private storage |
| **Component** | Execute single units of work | Execution mechanics, internal state, output emission | Workflow awareness, caller context |

## Quick Decision

| Pattern | Use |
|---------|-----|
| Condition X met --> fire action Y (no retry) | Single-trigger reactive rule |
| A --> B --> A --> B... (bounded loop over named state) | Coordinated rule set over a lifecycle-managed entity |
| Execute LLM call, process tools, write files | Component |

## The 5 Rules

1. **Rules trigger, they don't execute work** -- A rule fires bounded actions, not business logic.
   - Anti-pattern: Rule A sets `step=1`, Rule B watches for `step=1` and sets `step=2`...
   - Fix: Put durable progress on a lifecycle-managed entity and let coordinated rules fire components.

2. **Lifecycle coordinates entity phase, it doesn't execute** -- Lifecycle validates transitions; components do work.
   - Anti-pattern: lifecycle_transition carries business logic or hidden processing.
   - Fix: Move processing into a component; transition only declared entity state.

3. **Components are workflow-agnostic** -- Components don't know their caller.
   - Anti-pattern: Component checks `workflow_id` to decide behavior.
   - Fix: Pass behavior differences as configuration, not caller identity.

4. **State ownership is exclusive** -- Only one layer owns any piece of state.

   | State | Owner |
   |-------|-------|
   | Trigger conditions | Rules |
   | Rule match/iteration counters | Rule engine |
   | Phase/progress of a named entity | Lifecycle-managed graph entity |
   | Execution state (pending tools, loop phase) | Component |
   | Domain and lifecycle entities | Knowledge graph (ENTITY_STATES) |

5. **If it has operator-visible phase/progress, model it explicitly** -- Simple handoffs use rules; durable multi-step progress uses lifecycle-managed entities.

## Anti-Patterns

- Rule chains that build up ad hoc step state instead of using a lifecycle entity
- Lifecycle actions with inline processing logic (belongs in components)
- Components checking workflow context to decide behavior (should be caller-agnostic)
- Rules, lifecycle, and components tracking the same state (exclusive ownership violated)

## State Storage Boundaries

| Category | Storage | In Knowledge Graph? |
|----------|---------|---------------------|
| Domain and lifecycle entities | `ENTITY_STATES` KV | Yes (Graphable) |
| Operational execution artifacts | Component-specific KV / ObjectStore | No, except ref triples |
| Events/work items | JetStream streams | No |

Do NOT write opaque execution artifacts to ENTITY_STATES -- it pollutes the knowledge graph.
Lifecycle phase, parent/child refs, audit source, and operator-visible progress are graph facts when they describe a named entity.

## Reactive Rule Example (single trigger)

```json
{
  "id": "architect-complete-spawn-editor",
  "type": "expression",
  "entity": {
    "pattern": "acme.*.delivery.*.review.*",
    "watch_buckets": ["ENTITY_STATES"]
  },
  "conditions": [
    {"field": "review.lifecycle.phase", "operator": "eq", "value": "architect_complete"}
  ],
  "on_enter": [
    {"type": "publish_agent", "role": "editor"}
  ]
}
```

The current rule processor has a typed `ENTITY_STATES` evaluator; it does not watch
or decode component-owned operational KV such as `AGENT_LOOPS`. A future
operational-state rule path requires its own typed decoder/evaluator contract.

## Lifecycle Rule Set Example (loop with limit)

```go
// Sketch: rules watch a lifecycle-managed review entity in ENTITY_STATES.
// Review output and fix artifacts stay in component stores/ObjectStore refs.
Rule "request-review": when review.phase == "ready" -> publish_agent reviewer
Rule "request-fix": when review.phase == "needs_fix" -> publish_agent fixer
Rule "max-iterations": when state.iteration >= 3 -> lifecycle_fail(reason)
```

Read `docs/concepts/14-orchestration-layers.md` for full documentation.
