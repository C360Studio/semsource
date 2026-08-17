# Git submodule support

Tracks [#185](https://github.com/C360Studio/semsource/issues/185).

## Why

A monorepo with linked git submodules currently ingests **incompletely and
silently**: remote clones run `git clone --depth 1` with no
`--recurse-submodules` (`workspace/workspace.go`), so every submodule directory
arrives empty and its code is simply absent from the graph with no warning —
the inverse of the no-silent-entity-loss posture, applied to inputs. Even a
locally initialized submodule is walked as a plain directory: its symbols are
attributed to the parent watch path's org/project/version, and the submodule's
pinned commit (the gitlink SHA) appears nowhere in provenance. An early
adopter's monorepo with several submodules makes this a v1 blocker, and its
identity decisions gate the #184 multi-repo onboarding story.

## What Changes

- **Completeness**: remote repo clones recurse submodules
  (`--recurse-submodules --shallow-submodules`); pulls keep submodule working
  trees in sync with the parent's gitlinks. Proposed default: **on**, with a
  per-source opt-out (silent absence is the bug; design confirms the flag
  shape).
- **Loudness**: a watched tree whose `.gitmodules` names a submodule whose
  directory is empty/uninitialized is detected and surfaced on the source
  status surfaces — never silently missing code.
- **Identity**: `.gitmodules` entries auto-expand into per-submodule scoped
  watch paths — each submodule gets its own `project` and a `version` derived
  from the gitlink SHA, riding the existing `WatchPathConfig.Version`
  entity-ID scoping surface. A shared submodule therefore dedups to the same
  entity IDs across every consumer repo pinned at the same SHA, instead of
  each consumer's copy forking identity under its parent's `{system}` segment.
  The parent's own watch path excludes submodule directories.
- **Provenance**: the gitlink SHA is recorded for submodule-derived entities
  (via the version scoping above), so "which pinned commit produced this
  symbol" is answerable.

## Capabilities

### New Capabilities

- `git-submodule-ingestion`: submodule-aware workspace and identity behavior —
  recursive clone/pull semantics, uninitialized-submodule detection on status,
  `.gitmodules` → scoped-watch-path expansion (own project, gitlink-SHA
  version), parent-tree exclusion of submodule dirs, and shared-submodule
  identity dedup.

### Modified Capabilities

- `ast-source-configuration`: the "AST sources use only watch paths"
  requirement currently demands complete, explicitly-configured entries used
  "without precedence or conversion logic". Submodule auto-expansion derives
  additional scoped entries from `.gitmodules` at resolution time; the
  requirement gains that derivation as sanctioned behavior (strict rejection of
  the six legacy top-level keys is unchanged).

## Impact

- **Code**: `workspace/workspace.go` (clone/pull flags, submodule sync,
  readiness probes — `IsRepoReady` already accepts gitlink `.git` files);
  `.gitmodules` parsing; watch-path expansion in the repo-source intake path
  and/or `processor/ast-source` resolution (`ResolveWatchPaths`); source
  status composition for the uninitialized-submodule signal.
- **Identity**: entity IDs for submodule code move from the parent's system
  segment to per-submodule `{project}` + gitlink-SHA-versioned scoping —
  **BREAKING** for any graph already containing submodule code under parent
  identity (accepted: fresh-reseed posture is established, and today's
  submodule ingestion is accidental).
- **Consumers**: SemSpec, SemDragon, and SemOps read the governed graph and
  gain complete, correctly-attributed monorepo coverage; #184 onboarding
  documents the resulting multi-repo config story. No semstreams substrate
  changes — expansion, identity, and status are all SemSource-owned.
- **Test fixture** (exists, verified 2026-08-17): `C360Studio/semdev-test`
  links `C360Studio/semdev-test-sub` twice — root `semdev-test-sub` @
  `b1256521` (v2, has `greeter.Farewell`) and `nested/semdev-test-sub` @
  `b191a7bf` (v1, no `Farewell`) — exercising dual-pin version scoping,
  shared-submodule dedup, nested paths, and (via plain clone) the
  empty-directory loudness case. Both repos are public; history is
  append-only.

## Non-goals

- No submodule **write** operations: SemSource never inits, updates, or
  advances pins in user repos it did not clone itself; it ingests what the
  gitlinks say.
- No `.gitmodules` editing, pin-freshness advice, or dependency-update
  tooling (SemOps/product territory).
- No git-LFS or partial-clone support changes.
- No substrate work: governed-graph merge/dedup mechanics stay in semstreams;
  SemSource only constructs IDs that merge correctly.
- Nested submodules (a submodule's own `.gitmodules`) are cloned by
  `--recurse-submodules`, but identity expansion beyond depth 1 is decided in
  design — not promised here.
