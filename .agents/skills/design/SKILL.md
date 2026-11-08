---
name: design
description: >-
  Draft design.md for an OpenSpec change on the design track: file:line context, options with trade-offs,
  the adopter seam, and owner questions. Read-only on code; drafts and never rules or implements.
---

# Draft a Design

Read [the shared protocol](../../protocol.md) and [the architect contract](../../contracts/architect.md) first. Use this skill only on the `design` track (protocol § 2 Clarify); a `quick` change has no `design.md`. You draft text; the owner rules and a developer implements.

## 1. Inventory What Exists

Open every file the change touches and record each claim as `file:line`, opened in this session, never remembered. Run `git log -S '<symbol>'` before calling anything dead.

## 2. Write design.md in OpenSpec's Sections

Write `openspec/changes/<id>/design.md` with the `spec-driven` sections (Context, Goals / Non-Goals, Decisions, Risks / Trade-offs, Migration Plan, Open Questions) and follow the `design` rules in `openspec/config.yaml` when present:

- **Context:** the `file:line` inventory from step 1.
- **Decisions:** each names at least two options with cost, blast radius, and what each forecloses, plus the adopter seam: what a developer who never opened this file must know, and what happens if they do nothing.
- **Open Questions:** mark each one that needs an owner ruling. A question that would change specs, approach, or tasks is resolved before tasks.

Stay under 120 lines; `task spec:check` warns past the limit.

## 3. Tie Tasks to Premises

Give each task in `tasks.md` its `file:line` premise and its acceptance check. Leave the first task's `Hold: OPEN-Q proposal awaits owner approval` in place: one owner edit approves `proposal.md` and `design.md` together. Stop there and hand off.
