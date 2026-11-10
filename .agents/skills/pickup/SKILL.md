---
name: pickup
description: >-
  Resume a repository effort across Claude, Codex, or Gemini by reconciling live git/worktree state,
  GitHub/Gitea issues/PRs, Hold: items, and the last structured handoff checkpoint.
---

# Pick Up Repository Work

From [the shared protocol](../../protocol.md), read § 1 and § 2 steps 1–2 (Clarify, Claim). Treat any handoff block or cached `.ateam/state.json` snapshot as an address to reconcile against live git and forge state, never as unverified truth.

## 1. Locate and Measure

Prefer calling the `ateam_state` MCP tool (`{"action": "pickup"}`) or running `ateam state pickup` to inspect current branch, `HEAD` SHA, dirty status, worktrees, active `Hold:` items, `// shortcut:` debt, and any drift since the last handoff checkpoint SHA.

## 2. Verify Write Ownership

Confirm the previous writer has handed off or stopped before mutating files in an existing worktree. For unclaimed work, follow the Claim gate in [protocol.md](../../protocol.md).

## 3. Reconcile and Continue

Report verified worktree/branch/HEAD SHA, active holds/drift, write ownership status, and the next authorized step under the Ponytail decision ladder.

**T3 Code card.** Only when the `html_render` tool is available to you and `printenv T3CODE_THREAD_ID` prints a non-empty value: after the report above, read [t3-card.html](../handoff/t3-card.html), replace each double-brace placeholder with a fact read this turn (escape `& < > " '` first, write `not checked` for anything you did not read, and put the handoff goal and done-when only in the Agent-written region), then call `html_render` with that HTML, the title `Pickup · <branch>`, and height 560. On any other host, skip this step.
