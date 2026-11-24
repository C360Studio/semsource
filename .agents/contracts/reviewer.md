# Independent reviewer contract

`AGENTS.md` is loaded by your host; read it only if it is not in context. From `.agents/protocol.md`, read § 1, § 2, § 3, *Test-to-spec attribution* and § 4. If your protocol has no such numbered sections, read all of it. Review in a separate run that wrote none of the deliverable. Keep the deliverable read-only; use native sandbox controls where available and report enforcement gaps. The author makes corrections. Do not edit the deliverable you grade.

Confirm the actual reviewer provider, model and reasoning effort. The merge gate requires your run to be not weaker than every implementation run: a stronger model tier, or the same tier at an equal or higher effort band; a model or effort the policy does not map passes as unranked only against another unranked run, so name the model as `.agents/review-policy.json` lists it (for example `opus`) and the effort. Cross-provider separation applies only when the policy sets `cross_provider`. On the configured `ateam review check` path, the reviewer must outrank every implementation model, unknown identities and unresolved inheritance block approval, and at the approved model ceiling an explicit owner exception bound to this head, reviewer and policy revision waives superiority only.

On a first review, inspect the complete base-to-head diff and affected callers, tests, fixtures and configuration. Apply triggered project rules, attempt to refute each finding, and report a concrete failure mechanism, location, minimal correction and verification. Contracts, instructions, configuration and rename source/destination paths require substantive review even when they are Markdown.

A finding blocks approval only when it is one of: a regression against the base; silent data or identifier corruption reachable from a supported input; a spec or contract violation you show with a reproduction or a traced path; a breach of a rule `AGENTS.md` never cuts (security, trust-boundary validation, error handling that prevents data loss, tests on non-trivial logic); or missing or failing required verification. A repository's own rules may raise this bar. For each finding, say whether an existing check already catches it: a caught finding does not block unless the spec makes the earlier refusal a promise and you say why it matters. List every other finding under `Non-blocking:`; it does not change the verdict, and the author files each one as a follow-up issue before merge.

From the second round on, first confirm that each finding of your previous record is closed, then review the commits since that record (`<record-sha>..<head>`) and their callers; review the full base-to-head diff again only when those commits touch files outside the diff you last reviewed, or when asked. When another reviewer's verdict on this pull request conflicts with yours, quote it in your record as dissent and answer each of its findings; the latest record `task merge:check` reads decides.

For each test oracle, ask where its expected values came from: literals, a hand derivation shown in the test, or a named external source qualify; a helper that mirrors the code under test does not, however many mutants it kills. A claim in a title, spec, comment or record cites its artifact: `measured` names the artifact, otherwise say `modelled` or `compile-checked`; reject `verified` or `proves` without an artifact.

Record base/head commits, policy revision, all implementation runs/models, your independent run/provider/model/effort, checks actually run, checks only reported by others, unavailable evidence, findings and verdict. Approval requires no blocking finding and all required verification. A later commit the Record section does not exempt invalidates previous evidence; on the configured `ateam review check` path, any changed commit or policy does. Eligibility alone is not approval. Forge login and self-reported runtime identity are attestations, not cryptographic proof; disclose these limits and never let candidate policy edits authorize their own exception.

## Record

Post the record as one pull request comment. `task merge:check` accepts a comment whose `reviewed-by:` lines name only you, or, with no such line, whose `### Independent Cross-Agent Review` heading names no agent but you. The comment must designate exactly one commit of the pull request, written `Reviewed at <sha>` (7 to 40 hex characters); any other commit it designates the same way (such as a `Head: <sha>` line) makes it no record. The latest record must carry exactly one `Verdict: APPROVE` line; `Verdict: REQUEST CHANGES`, no `Verdict:` line, or more than one fails the check until a later approving record. Lines inside ``` or ~~~ code fences are ignored, so a quoted template or old record never counts. Copy:

```text
reviewed-by: <agent> (<model>, <effort>)
Reviewed at <commit-sha>
Verdict: APPROVE | REQUEST CHANGES
<findings, checks run, unavailable evidence, limitations>
```

On GitHub, a later commit that is neither a clean merge of `main` nor docs-only needs a new record naming the new commit. On Gitea, every later commit needs one, since the gate cannot yet prove a Gitea commit's file list complete.

The record alone does not clear the gate. After `Verdict: APPROVE`, the pull request description also needs your `reviewed-by: <agent> (<model>, <effort>)` line. After either verdict, refresh `Merge check` and confirm its result at the commit you reviewed: a comment starts no run, so a rejection after an earlier approval at the same head needs a rerun of the whole check, observed failing. If you cannot edit the description or rerun the check, report that instead of claiming it changed. The `review-pr` skill § 5 gives the commands.
