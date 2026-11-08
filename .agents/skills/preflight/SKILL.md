---
name: preflight
description: >-
  Select and execute Taskfile verification gates for the current diff and record reproducible evidence.
  Use before implementation pushes, handoffs, or concluding a task.
---

# Verify Changes (Preflight)

Read [the shared protocol](../../protocol.md) first. One `Taskfile.yml` entrypoint serves local verification, MCP (`ateam_task`), and CI.

## 1. Select Gates by Diff Scope

Inspect `git status --short` and `git diff --stat`, run `task test:spec` for test/spec changes (syntax and reference existence, not semantic coverage), run `task mutate:check` for non-trivial refusals/branches, and run `task verify` (via `ateam_task` or CLI) before any push or completion.

## 2. Record Evidence

Record the exact command, exit status, tested `HEAD` SHA, and the 6-line testing block (`Decision`, `Rule`, `Inputs`, `Run`, `Mutation`, `Not covered`) when touching parsers/laws/histories, ending with Ponytail's 1–2 line skipped/risk summary.
