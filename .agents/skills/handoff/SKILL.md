---
name: handoff
description: >-
  Checkpoint current repository work for continuation or transfer across Claude, Codex, and Gemini.
  Reconciles live git/test/hold state and emits the 5-field pickup prompt.
---

# Checkpoint and Hand Off Work

Read [the shared protocol](../../protocol.md) first. A handoff records current truth in shared homes (GitHub/Gitea PR/issue comments and the structured `.ateam/state.json` snapshot) without creating competing prose handoff files in the repo tree.

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
