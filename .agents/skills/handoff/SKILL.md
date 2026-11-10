---
name: handoff
description: >-
  Checkpoint current repository work for continuation or transfer across Claude, Codex, and Gemini.
  Reconciles live git/test/hold state and emits the 5-field pickup prompt.
---

# Checkpoint and Hand Off Work

From [the shared protocol](../../protocol.md), read § 1 and § 2 step 5 (Close). A handoff records current truth in shared homes (GitHub/Gitea PR/issue comments and the structured `.ateam/state.json` snapshot) without creating competing prose handoff files in the repo tree.

## 1. Reconcile the Checkpoint

Call the `ateam_state` MCP tool (`{"action": "handoff", ...}`) or run `ateam state handoff` with the 5 semantic fields:

- `goal`: PR/issue and the authorized outcome
- `addresses`: PR, worktree, branch, checkpoint SHA, and evidence locations
- `constraints`: Decision/hold pointers, local access limits, and write-ownership transfer status
- `done_when`: Acceptance criteria and remaining verification gates
- `checks_to_run`: Cheap reconciliation commands and remaining unrun validation

Pass `post` explicitly over MCP: `post:false` saves a local-only checkpoint, and `post:true` posts it as a PR comment when posting is authorized. An omitted `post` posts. The CLI posts unless given `--no-post`.

## 2. Return the 5-Field Pickup Prompt

Output the canonical 5-field block and instruct the next session to run `/pickup` (`ateam state pickup`):

```text
Goal: <PR/issue and authorized outcome>
Addresses: <PR, worktree, branch, checkpoint SHA, evidence locations>
Constraints: <Decision/hold pointers, local access limits, ownership transfer status>
Done when: <Acceptance criteria and remaining boundary>
Checks to run: <Reconciliation commands and remaining validation>
```

At a code PR's ready-for-review under the review policy's `review_launch: manual` (the default), this handoff is the stop: name the PR and the reviewer runs `ateam review request <n>` prints in `Done when`, so a person or a scheduled workflow can start the reviewer.

**T3 Code card.** Only when the `html_render` tool is available to you and `printenv T3CODE_THREAD_ID` prints a non-empty value: after the report above, read [t3-card.html](t3-card.html), replace each double-brace placeholder with a fact read this turn (escape `& < > " '` first, write `not checked` for anything you did not read, and put the handoff goal and done-when only in the Agent-written region), then call `html_render` with that HTML, the title `Handoff · <branch>`, and height 560. On any other host, skip this step.
