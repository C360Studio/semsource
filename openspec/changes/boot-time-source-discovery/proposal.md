# Discover branches and submodules at boot, and say when a restart would find more

Approved-by:

Track: design (adds spec Requirements, touches the boot path, and changes what the retained
configuration holds).

## Why

#216: beta.9 discovered matching branches and git submodules at run time and started a
component set for each. SemEngine ADR-094 seals composition at boot, so on the SemEngine pin
both are off, with log warnings (`cmd/semsource/run.go:209-218`). The owner ruled on 2026-11-17
for **Change**: all discovery happens at boot, and status says when a restart would find more
([ruling](https://github.com/C360Studio/semsource/issues/216#issuecomment-6940397331)).

Reading main shows two more gaps:

- **Branches are expanded into the file-generated configuration** (`config/expand.go:154`). On a
  retained store at an equal version, the retained configuration wins (#266), so a branch that
  appears between boots is never admitted unless the operator bumps `config_revision`.
- **Submodules are never expanded.** Expansion exists only in the runtime `internal/subwatch`,
  which is no longer started.

## What Changes

- **Boot reconciliation.** After the config manager starts and before the service manager is
  built, SemSource reconciles discovery-owned components into the effective configuration. It
  adds components for branches and materialized submodules found now, and deletes discovery-owned
  components whose branch or submodule is gone. Composition seals over the result, so discovery
  takes effect at the boot that finds it, whatever the retained arbitration decided.
- **Submodules expand at boot** whenever the parent checkout exists, reusing subwatch's
  identity rules: project = submodule URL slug, version = 12-hex gitlink SHA.
- **A read-only runtime check** compares what boot admitted with what a restart would find, and
  reports it on `/source-manifest/status` and `source_status` as a `discovery` block with
  `restart_required`. It never writes configuration, never creates worktrees, and never starts a
  component.
- **Discovery failures are typed status, not log lines.** Covered failures: a remote repo with
  no checkout yet, a git listing failure, and a branch cap that was reached.
- **The dead runtime watchers are removed:** `startBranchWatchers`, `startSubmoduleWatchers`,
  `publishBranchComponents`, `removeBranchComponents`, and subwatch's `Run` loop.

## Non-goals

- No runtime component admission. A restart remains the only activation path (ADR-094).
- No retirement of entities from a deleted branch or submodule. They stay until #215's removal
  replay lands, the same as any source removed from configuration today.
- No remote fetch to discover branches. Discovery reads local refs, as beta.9 did.
- No per-branch submodule expansion for multi-branch sources (not covered at beta.9 either).

## Consumers

Operators and SemTeams or SemDragon through `source_status` and `/source-manifest/status`. The
`discovery` block is additive; no existing field changes.

## Impact

- Specs: new capability `source-discovery`.
- Code: `cmd/semsource/run.go` (boot reconcile, warnings removed), `config/expand.go`,
  `internal/subwatch`, `processor/source-manifest` (status block).
- Tests: real-process restart tests against local git fixtures (see `tasks.md`).
