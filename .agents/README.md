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

We deliberately do **not** mirror semstreams' architect/developer/reviewer role triad. Those roles
carry a framework's design authority; SemSource is a consumer, and our two roles are scoped to the
failure classes that are actually ours — component structure and entity/event semantics. Adopting
the triad would be inventing authority we don't hold.

## Shared decision skills

Four decision heuristics live in `skills/`, each in one of two modes recorded in
`upstream.manifest`. The distinction matters, and getting it backwards is how a false fact spreads:

| Skill | Mode | Why |
| --- | --- | --- |
| `kv-or-stream` | **vendored** | KV vs Stream is framework truth semstreams owns. Our old fork still claimed KV history was an audit trail long after upstream corrected it — bounded per-bucket history is not a ledger. |
| `orchestration-check` | **vendored** | Also framework truth. Our old fork still named "reactive workflows", a model upstream retired in favor of lifecycle-managed entities. |
| `new-payload` | **forked** | SemSource registers payloads explicitly at bootstrap — `RegisterPayloads(reg)` wired into `buildPayloadRegistry()`. semstreams uses `init()` with blank imports. Vendoring theirs would break our convention. |
| `query-pattern` | **forked** | Upstream's copy says there is no canonical graph MCP surface yet, which is true of the framework. We ship one (`processor/mcp-gateway`), so MCP is a real option here. |

**vendored** — our copy must match the pinned upstream byte for byte. Do not edit it; if it is wrong,
it is wrong upstream, and the fix is an issue against semstreams (see `docs/upstream/semstreams-asks.md`).

**forked** — ours by right. The manifest records the upstream digest we last reconciled against, so an
upstream edit fails the check and forces a conscious re-read rather than silent divergence.

The remaining `.claude/skills/` entries (the `openspec-*` workflow) are Claude-workflow tooling and
are platform-specific by design — do not mirror them.

## Checking it

```bash
task agents:check     # or: ./scripts/check-agent-sync.sh
```

This is the scripted form of what semstreams documents as a manual read-only procedure. It fails, with
a named reason, when:

1. a **vendored** skill drifts from the pinned upstream;
2. **upstream changes** a skill we deliberately forked;
3. upstream ships a shared decision skill we track under neither mode;
4. an adapter stops naming its canonical file, grows a body, or gains a write tool;
5. a Claude agent has no frontmatter — which makes it **undiscoverable**, the exact state both of ours
   shipped in before this check existed, while CLAUDE.md advertised them;
6. `upstream.manifest` records a semstreams version other than the one `go.mod` pins, which would mean
   the check was comparing against the wrong source.

## When the semstreams pin moves

Bumping `github.com/c360studio/semstreams` will fail `task agents:check` until the shared skills are
reconciled — that is the point. For each skill the check names: re-copy the vendored ones, read the
upstream diff for the forked ones and decide what carries over, then record the new digests and the
new `upstream_version` in `upstream.manifest`.
