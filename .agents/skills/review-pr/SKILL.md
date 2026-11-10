---
name: review-pr
description: >-
  Review one pull request as an independent, read-only reviewer and post the record comment
  merge-check reads. The manual trigger for a cross-agent review.
---

# Review a Pull Request

Read [the reviewer contract](../../contracts/reviewer.md) first; it names the protocol sections a review needs. Review in a separate run that wrote none of the change. The deliverable stays read-only; the author makes corrections.

## 1. Pick the PR

Use the PR the user named. Otherwise list the open, non-draft PRs whose `implemented-by:` line does not name you and whose comments hold no record (`reviewed-by:` with `Reviewed at <sha>`) for the current head: `gh pr list --state open --json number,title,isDraft,headRefOid,body`, then `gh pr view <n> --comments` (`tea` on Gitea). One candidate: review it and say which. Zero or several: ask which PR to review, then stop.

## 2. State the Problem and Your Own Approach

Read the PR description and the issue it closes or addresses. State the problem in one sentence. Before opening the diff, decide how you would solve it: the files you would touch, the shape of the change, and what you would not build. The diff must not anchor you.

## 3. Review

Check out the PR head detached at `.worktrees/<agent>-review-pr-<n>` (`git worktree add --detach .worktrees/<agent>-review-pr-<n> <head-sha>`), never in the author's worktree. When a host launch already started you in that checkout at the head, use it. Read the complete base-to-head diff and the callers, tests, fixtures, and configuration it touches. Compare it with your approach: a difference is a finding only when the PR's choice is worse. Pragmatic, idiomatic, simple yet thorough code beats complex or clever code (the Ponytail ladder). For each finding, try to refute it, then report the failure mechanism, location, minimal correction, and how to verify. On a code PR, run `task verify` in the checkout and record the result verbatim.

## 4. Post the Record

Post one PR comment in the contract grammar (`gh pr comment <n> --body-file <file>`):

```text
reviewed-by: <agent> (<model>, <effort>)
Reviewed at <head-sha>
Verdict: APPROVE | REQUEST CHANGES
<findings, checks run, unavailable evidence, limitations>
```

Name your provider, model as `.agents/review-policy.json` lists it (for example `opus`), and reasoning effort. Remove the review checkout (`git worktree remove .worktrees/<agent>-review-pr-<n>`) once the comment is posted. No sync of yours delivers the record: each checkout keeps its own `.ateam/state.json`, and the record reaches the developer when their checkout runs `ateam state sync`. When the launch spec gave you a `return_to`, run `ateam review notify --record <file> --return-to <id> <n>`: it sends only `review record for PR #<n> landed at <sha>; run ateam state sync, then ateam state pickup`, or prints the host tool to call, never your findings. End your conclusion with 1–2 lines naming what you skipped or did not check and any residual risk.

## 5. Clear the Merge Check

Where `Merge check` runs from ateam's `merge-check.yml`, a probe skips it while a code PR has no `reviewed-by:` description line and no passing `Merge check` already stands at its head, whether or not a record exists. GitHub shows a skipped job under its raw `name:` expression, never as `Merge check`, so until the line is written the required check is absent, not red (where the gate runs without that probe, it failed when the PR was marked ready). Once the line exists, or a pass stands at the head, every run judges in full. After either verdict, refresh the check and confirm its result at the commit you reviewed; if you cannot edit the description or rerun the check, say so instead of reporting that it changed.

- After a first `Verdict: REQUEST CHANGES`, add no line and stop: the gate passes only a latest record with exactly one `Verdict: APPROVE` line, so the PR cannot merge, and while it has no line and no pass at its head the probe skips `Merge check`, so the required check stays absent, not red.
- After `Verdict: APPROVE`, the gate also needs your `reviewed-by: <agent> (<model>, <effort>)` line in the PR description: add it if it is missing (`gh pr view <n> --json body -q .body`, then `gh pr edit <n> --body-file <file>`). Where `Merge check` runs on the `edited` event, that edit, made after the record comment, runs it at the head.
- When nothing is edited, because the description already carried your line or because your `Verdict: REQUEST CHANGES` follows an earlier approval at the same head, a comment starts no run and the earlier result stands. Rerun the whole latest run for the head, not `--failed`, which skips a run that passed; the probe and the check read the forge when they run, so the rerun judges your new record: `gh run list --branch <head-branch> --workflow merge-check.yml --limit 1 --json databaseId,headSha`, confirm its `headSha` is the commit you reviewed, wait for it (`gh run watch <run> --exit-status || true`), then `gh run rerun <run>` and `gh run watch <run> --exit-status`. After a rejection it must fail; an earlier pass is no evidence the rejection was applied. In a repository that runs the gate inside `ci.yml`, rerun that run's merge-check job (`gh run rerun --job <job-id>`). On Gitea, rerun it from the Actions page, and say so in your reply.

Do not update the branch to clear it.
