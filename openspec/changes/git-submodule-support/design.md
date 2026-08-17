# Design — git submodule support

See `proposal.md` for motivation and `specs/` for the behavior contracts.

## Context

Verified current state (2026-08-17):

- A `repo` meta-source expands at spawn time into four component specs — git,
  ast, docs, config (`config.ExpandRepoSources` → `sourcespawn.buildSpecs`).
  The clone itself happens later, at git-source runtime (`workspace.EnsureRepo`
  from `handler/git/handler.go:348`); ast/doc/cfg instances point at the
  expected checkout path and gate on `IsRepoReady`/`IsPathReady`. So at
  expansion time a remote repo's `.gitmodules` and gitlink SHAs are **not yet
  readable** — any expansion design that needs them at spawn time would move
  the clone into the Add/boot path.
- Dynamic post-spawn expansion already has precedent: multi-branch repos
  register a `BranchWatcherRef` that discovers branches at runtime and spawns
  per-branch entries.
- `WatchPathConfig` (ast) carries org/project/version per entry; explicit
  `project` overrides the path-derived system slug, and `version` appends
  version scoping to the system segment (`astComponentConfig`,
  `internal/sourcespawn/components.go:83-129`).
- doc-source and cfgfile-source carry only `org` + `paths`; their system
  segment and instance names derive from the path
  (`SystemSlug(paths[0])`, `components.go:208,231`). There is no project
  override surface today.
- `IsRepoReady` already treats a `.git` FILE (gitlink) as a complete checkout.
- Instance names are deterministic and `sourcespawn.Add` is idempotent by
  instance name — two writers to the same name overwrite, they don't coexist.

## Goals / Non-Goals

**Goals:**

- Race-free correctness: submodule files must never be attributable to the
  parent scope, even transiently during seeding.
- Canonical identity that dedups across consumer repos without coordination.
- All three handler families (ast, docs, config) scope submodule content
  correctly — not just code.
- No clone work moves into the spawn/Add path (a monster monorepo clone must
  not block boot or an add_source reply).

**Non-Goals:**

- No cross-host submodule auth story beyond the existing single `GitToken`
  (a submodule needing different credentials fails the clone loudly).
- No `public.*` namespace auto-adoption for well-known open-source submodules
  — submodule entities stay in the configured org's namespace under existing
  namespace policy.
- No handling for hand-authored ast `watch_paths` pointed directly at a repo
  with submodules beyond boundary skipping (see D5) — full expansion applies
  to repo/git sources, which own the git context.

## Decisions

### D1. Discovery and expansion happen at runtime, in git-source, after the checkout materializes

The git-source instance — the component that owns the clone/pull — probes the
checkout after every successful `EnsureRepo` (initial and each poll cycle):
parse `.gitmodules`, read gitlink SHAs and materialization state
(`workspace.ListSubmodules`, new: `git submodule status --recursive` +
`git config -f .gitmodules`), then spawn/refresh per-submodule ast/doc/cfg
entries through the existing `sourcespawn.Add` path with deterministic names.
The probe result is carried on git-source's status (the loudness signal).

*Why not spawn-time expansion in `ExpandRepoSources`?* The checkout doesn't
exist yet for remote repos; cloning at expansion time moves minutes of work
into boot/Add replies. *Why not inside ast-source's `ResolveWatchPaths`?*
docs/config would stay mis-scoped, ast-source would become git-aware, and
the loudness signal would have no owner for unmaterialized (empty) trees that
ast never walks. The BranchWatcher pattern already established runtime
discovery → spawn as the house shape for "the repo tells us what sources
exist."

Submodule-derived entries point INTO the parent's checkout (a submodule has
no clone of its own); there is no per-submodule git-source instance.

### D2. Identity: project = URL slug, version = 12-hex gitlink SHA

- `project` = `workspace.URLToSlug` of the submodule's **resolved** URL
  (relative `.gitmodules` URLs like `../sibling` resolve against the parent's
  remote before slugging — required for cross-consumer determinism).
- `version` = first 12 hex chars of the gitlink SHA, fixed truncation (not
  `git rev-parse --short`, whose width varies by repo). Precedent: commit
  entities use short SHAs; config entities use `sha256[:6]`.
- `org` = the parent source's org (sovereign namespace policy unchanged).

Same (URL, SHA) linked anywhere in the org → byte-identical entity IDs →
governed-graph merge does the dedup. Both values ride existing surfaces
(`WatchPathConfig.Project`/`Version` slugification), so IDs stay valid
6-part NATS-KV-safe IDs by construction. This is a permanent identity
decision — record as a one-page ADR (tasks).

### D3. Entity identity is canonical; instance identity is parent-scoped

Two parents linking the same submodule at the same SHA must produce identical
entity IDs but must NOT share a component instance (same deterministic name →
`sourcespawn.Add` overwrite → one parent's path wins, the other's watch/
lifecycle silently vanishes, and removing one parent would tear down the
other's instance). Instance names therefore include the parent scope, e.g.
`ast-source-<subproject>-<ver12>-via-<parentproject>`, while the
`watch_paths` entry carries only the canonical project/version. Duplicate
publishes from two live instances at the same SHA are byte-identical and
idempotent in the governed graph.

### D4. Recursive expansion with a depth cap

The probe inventories submodules **recursively**; every depth gets its own
scoped entries (its gitlink is deterministic given the parent pin, so
identity stays deterministic at all depths). Depth is capped at 10; paths
beyond the cap are surfaced on status, never silently dropped (no-silent-caps
posture). This also keeps D5 sound: without recursive expansion, boundary
skipping would orphan depth-2 trees entirely.

### D5. Parent walks skip git-boundary subtrees intrinsically

The shared walk layer used by ast/doc/cfg sources skips any subdirectory that
is itself a git boundary (contains a `.git` entry — file or directory).
This is what makes exclusion race-free: correctness never depends on the
parent's config being updated before its first walk, because the boundary is
detected from the tree itself at walk time. Deliberate side effect, sanctioned:
an unrelated nested git repo inside a watch path is also skipped — the same
bug class (foreign code blending into the parent's scope). Walkers report
skipped boundaries in their status detail so a hand-configured ast source
still isn't silent. Unmaterialized submodule dirs are empty and yield no
files regardless; their loudness comes from D1's probe.

### D6. Opt-out: `submodules` (\*bool, default true) on repo/git sources

Follows the `WatchEnabled *bool` precedent for default-true booleans. When
false: `EnsureRepo` does not recurse, no expansion spawns, and the probe (a
local read of `.gitmodules`, no network) still reports declared submodule
paths as `excluded_by_config` — distinguishable from unexpected emptiness,
per the loudness spec.

### D7. Clone/pull mechanics

- Clone: `--recurse-submodules --shallow-submodules` (parent stays
  `--depth 1`; gitlink SHAs live in the tree object, so depth 1 suffices).
- A shallow submodule fetch FAILS when the pinned SHA is not the remote
  branch tip (a well-known git behavior, server-dependent). Fallback: on
  failure, retry that submodule with a full fetch
  (`git submodule update --init --recursive` without depth). The fixture's
  v1 pin is deliberately a non-tip SHA and exercises exactly this path.
- Pull: after the parent syncs, `git submodule update --init --recursive`
  brings trees to the (possibly moved) gitlinks; the probe then re-runs.

### D8. A moved gitlink is a version transition, not an in-place mutation

Version is part of the instance name (existing pattern), so a moved gitlink
spawns the new-version instances and removes the old-version instances via
`sourcespawn.Remove` (ordering: spawn new after the tree syncs, then remove
old). Graph-side supersession of the old version's entities follows the
existing versioned-source-supersession contract via the version registration
surface — nothing new on the graph side.

### D9. doc-source and cfgfile-source gain an optional `project` override

Today their system segments are path-derived, which would fork doc/config
identity per consumer checkout path. Add an optional `project` config field
(same override semantics ast already has: explicit project replaces the
path-derived slug; absent = byte-identical to today). Spawned submodule
entries set it to the canonical submodule project. Version scoping stays
code-only (the spec contracts version scoping for code entities; config
entities already content-hash their instance segment).

## Risks / Trade-offs

- [Shallow fetch of a non-tip pin fails on some servers] → D7 fallback to
  full fetch per submodule; fixture covers it.
- [Instance blowup on a monorepo with many submodules × 3 handler families]
  → acceptable at v1 scale (components are light; per-file/symbol guards
  already bound work). If a real corpus hurts, batch multiple submodules
  into one component config per family later — an instance-topology change,
  not an identity change.
- [Submodule cycles / pathological nesting] → depth cap 10 + loud status
  (D4). Cyclic gitlinks would already break the clone step loudly.
- [Two parents at different SHAs of one submodule] → distinct version scopes
  by construction (D2); both live side by side; supersession only relates
  versions within one registration lineage.
- [.gitmodules declares a path with no gitlink in the tree (stale entry)] →
  probe reports it as declared-but-absent on status; no spawn.
- [Same-host private submodules] → parent's `GitToken` flows to recursive
  clone (same `applyAuth` mechanism). Cross-host credentials are a loud
  clone failure, out of scope (Non-Goals).

## Migration Plan

**BREAKING** identity change for any graph that previously ingested
initialized submodule code under parent scope (accidental behavior). Standard
posture applies: `docker compose down -v` + reseed; the graph re-derives from
source. No shims, no dual-publish window. Rollback = redeploy previous tag +
fresh reseed the same way.

## Open Questions

- Whether doc/config entities should later also carry gitlink-version scoping
  (would require a version surface on those components; deferrable — spec
  contracts code-only today, and config entities content-hash already).
- Whether a future change should offer `public.*` namespace opt-in for
  well-known open-source submodules for cross-org dedup (existing namespace
  policy question, not submodule-specific).
