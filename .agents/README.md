# SemSource Agent Configuration

The files here are the tracked, platform-neutral authority. `.claude/` and `.codex/` hold **thin
adapters** that name exactly one canonical file and carry none of its content. Two copies of a
behavioral contract drift, and the one that drifts is always the one you are not looking at.

Modeled on the same layout in semstreams (`.agents/README.md` there), so someone moving between
the repos finds the same shape. Where we diverge, we say so and why.

## Roles

| Role | Canonical | Claude | Codex |
| --- | --- | --- | --- |
| Component reviewer | `contracts/go-component-reviewer.md` | `.claude/agents/go-component-reviewer.md` | `.codex/agents/go-component-reviewer.toml` |
| Graph event reviewer | `contracts/graph-event-reviewer.md` | `.claude/agents/graph-event-reviewer.md` | `.codex/agents/graph-event-reviewer.toml` |

Both roles are **read-only**. They report findings with file:line references and a severity; they do
not implement fixes unless the user starts a separate authorized task.

## Spawning them is the default, not a request

Spawning a role agent when the work matches its scope is the **default execution path for nontrivial
work in this repository — no user permission needed.** There is no "don't spawn agents unless asked"
rule here. An owner session that changed a component and did not run `go-component-reviewer` has
skipped a step, not saved one.

Route by what the diff touches:

| The change touches | Spawn |
| --- | --- |
| A component's structure — config, factory, ports, payload registration, lifecycle | `go-component-reviewer` |
| Entity ID construction, event semantics, federation merge, watch correctness | `graph-event-reviewer` |
| Both | Both, concurrently — they read disjoint failure classes |
| Neither | Neither. A reviewer spawned outside its scope returns noise, and noise is how a real finding gets ignored. |

The owner session keeps the decision. These roles are read-only by construction: they return findings,
never commits, and a finding is an input to your judgment rather than a verdict on it.

Massively-parallel `Workflow` orchestration remains opt-in and is a separate question — that
restriction does not extend to these role agents.

We still do **not** adopt semstreams' architect/developer/reviewer role triad as spawnable agents.
Those roles carry a framework's design authority; SemSource is a consumer, and our two specialists are
scoped to the failure classes that are actually ours. The `architect.md`, `developer.md`, and
`reviewer.md` files under `contracts/` are a different thing: **ateam's generated base contracts**
(added by `ateam init`, drift-checked by `ateam doctor --check`). They state the rules every run of that
kind follows — independence, model/effort records, the `reviewed-by:` grammar — and are read by the
`design` and `review-pr` skills and by each specialist, which opens with a pointer to its base. They
have no `.claude/agents/` or `.codex/agents/` adapter; do not edit their generated paragraphs.

## Shared decision skills

Five decision heuristics live in `skills/`; every upstream skill in `github.com/c360studio/semengine` has a
disposition recorded in `upstream.manifest`. The distinction matters, and getting it backwards is how a
false fact spreads:

| Skill | Mode | Why |
| --- | --- | --- |
| `kv-or-stream` | **vendored** | KV vs Stream is framework truth SemEngine owns (`KV_<bucket>`, `ENTITY_STATES` history=1, `pkg/graphview`). |
| `entity-or-bucket` | **vendored** | Framework storage-placement and operational-state ownership rules (`GRAPH_STATUS` info-only per ADR-109, `COMMUNITY_INDEX` prefix grouping, `graph/kvcatalog`). |
| `new-payload` | **forked** | Both repos register explicitly at bootstrap (ADR-103); this recipe preserves SemSource package paths and bootstrap while adopting registry floors/contracts and production decoder tests. |
| `query-pattern` | **forked** | Upstream's copy says there is no canonical graph MCP surface yet, which is true of the framework. We ship one (`processor/mcp-gateway`), so MCP is a real option here alongside `graph.QueryVerbs()` and `min_revision`. |
| `orchestration-check` | **local (retained)** | Retained from SemStreams (`54cd290a`) until SemEngine Change 6 ports `processor/rule` and re-publishes upstream rule/lifecycle guidance. |

**vendored** — our copy must match the pinned upstream (`github.com/c360studio/semengine`) byte for byte. Do not edit it; if it is wrong,
it is wrong upstream, and the fix is a GitHub issue against `C360Studio/semengine`.

**forked** — ours by right. The manifest records the upstream digest we last reconciled against, so an
upstream edit fails the check and forces a conscious re-read rather than silent divergence.

**excluded** — `semengine-status` tracks SemEngine's internal package-port backlog and does not define
SemSource's workflow. Its exclusion records the exact digest and fails on upstream change, requiring
another scope review. An excluded workflow must not also exist as a local skill. (`design`, `handoff`,
`pickup`, `ponytail`, `preflight`, and `review-pr` are `ateam`-managed across both repos and checked by
`task ateam:doctor`.) Generic vendored skills reference framework docs: resolve those references in the
pinned module from `go mod download -json github.com/c360studio/semengine@<version>`, not as SemSource-owned policies.

The remaining `.claude/skills/` entries (the `openspec-*` workflow) are Claude-workflow tooling and
are platform-specific by design — do not mirror them.

## Checking it

```bash
task agents:check     # or: ./scripts/check-agent-sync.sh
task ateam:doctor     # pinned ateam doctor --check: AGENTS.md size, rules-index parity, hooks, skill adapters
```

This is the scripted form of what SemEngine documents as a manual read-only procedure. It fails, with
a named reason, when:

1. a **vendored** skill drifts from the pinned upstream;
2. **upstream changes** a skill we deliberately forked;
3. upstream ships a shared decision skill we track under no disposition;
4. an adapter stops naming its canonical file, grows a body, or gains a write tool;
5. a Claude agent has no frontmatter — which makes it **undiscoverable**, the exact state both of ours
   shipped in before this check existed, while CLAUDE.md advertised them;
6. a specialist contract stops opening with a pointer to its ateam base contract, or `CLAUDE.md`
   stops being an `@AGENTS.md` import;
7. `upstream.manifest` records a SemEngine version other than the one `go.mod` pins (once `semengine` is a direct `go.mod` requirement), which would mean
   the check was comparing against the wrong source.

## When the semengine pin moves

Bumping `github.com/c360studio/semengine` will fail `task agents:check` until the shared skills are
reconciled — that is the point. For each skill the check names: re-copy the vendored ones, read the
upstream diff for the forked or excluded ones and decide what carries over, then record the new digests and the
new `upstream_version` in `upstream.manifest`.
