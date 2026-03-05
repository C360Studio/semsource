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
| **Reactive Engine** | State detection, triggers, multi-step coordination | Conditions, actions, step sequence, loop limits, timeouts | Actual work execution |
| **Component** | Execute single units of work | Execution mechanics, internal state | Workflow awareness, caller context |

## Quick Decision

| Pattern | Use |
|---------|-----|
| Condition X met --> fire action Y (no retry) | Single-trigger reactive rule |
| A --> B --> A --> B... (max N times, with timeouts) | Reactive workflow |
| Execute LLM call, process tools, write files | Component |

## The 5 Rules

1. **Rules trigger, they don't orchestrate** -- A rule fires one action, not a sequence.
   - Anti-pattern: Rule A sets `step=1`, Rule B watches for `step=1` and sets `step=2`...
   - Fix: Use a workflow for multi-step sequences.

2. **Workflows coordinate, they don't execute** -- Workflows spawn components, not inline logic.
   - Anti-pattern: Workflow step contains 100 lines of processing logic.
   - Fix: Move processing into a component, have workflow publish to trigger it.

3. **Components are workflow-agnostic** -- Components don't know their caller.
   - Anti-pattern: Component checks `workflow_id` to decide behavior.
   - Fix: Pass behavior differences as configuration, not caller identity.

4. **State ownership is exclusive** -- Only one layer owns any piece of state.

   | State | Owner |
   |-------|-------|
   | Trigger conditions | Rules |
   | Step progress, iteration count | Workflow |
   | Execution state (pending tools, loop phase) | Component |
   | Domain entities | Knowledge graph (ENTITY_STATES) |

5. **If it needs a loop limit, it's a workflow** -- Simple handoffs use rules; cycles use workflows.

## Anti-Patterns

- Rule chains that build up state across multiple firings (should be workflow)
- Workflows with inline processing logic (belongs in components)
- Components checking workflow context to decide behavior (should be caller-agnostic)
- Both rules and workflows tracking the same state (exclusive ownership violated)

## State Storage Boundaries

| Category | Storage | In Knowledge Graph? |
|----------|---------|---------------------|
| Domain entities | `ENTITY_STATES` KV | Yes (Graphable) |
| Operational results | Component-specific KV | No |
| Events/work items | JetStream streams | No |

Do NOT write operational results to ENTITY_STATES -- it pollutes the knowledge graph.

## Reactive Rule Example (single trigger)

```go
reactive.NewRule("architect-complete-spawn-editor").
    WatchKV("AGENT_LOOPS", "LOOP_*").
    When("architect role", func(ctx *reactive.RuleContext) bool {
        return ctx.State.(*AgentLoopState).Role == "architect"
    }).
    When("completed", func(ctx *reactive.RuleContext) bool {
        return ctx.State.(*AgentLoopState).Outcome == "success"
    }).
    Publish("agent.task.editor", ...).
    Build()
```

## Workflow Example (loop with limit)

```go
reactive.NewWorkflow("review-fix-cycle").
    WithStateBucket("REVIEW_FIX_STATE").
    WithMaxIterations(3).
    WithTimeout(300 * time.Second).
    AddRule(/* request-review rule */).
    AddRule(/* fix-issues rule */).
    AddRule(/* max-iterations-exceeded rule */).
    MustBuild()
```

Read `docs/concepts/14-orchestration-layers.md` for full documentation.
