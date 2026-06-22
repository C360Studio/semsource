# SemSource OpenSpec

SemSource uses OpenSpec change sets for large migrations that cross dependency,
runtime, graph-contract, and consumer-integration boundaries. The documents are
human-readable planning contracts: implementation tasks, tests, demos, and
SemStreams or SemOps follow-ups should trace back to explicit requirements.

## Layout

- `project.md`: standing project context and conventions.
- `changes/<change-id>/proposal.md`: why the change exists, what changes, and
  expected impact.
- `changes/<change-id>/design.md`: design decisions, rollout, and open
  questions.
- `changes/<change-id>/tasks.md`: implementation checklist in dependency order.
- `changes/<change-id>/specs/*/spec.md`: capability requirements and scenarios.

When a change is accepted and implemented, durable requirements can be promoted
into baseline specs under `openspec/specs`.
