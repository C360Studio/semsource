# Design: boot-time source discovery

## Context

- **Boot order** (`cmd/semsource/run.go`): `loadAndExpandConfig` (branch discovery,
  `config/expand.go:154`), then `startConfigManager` (retained arbitration), then
  `createServiceManager`. The service manager reads `configMgr.GetConfig()` (`run.go:404`), and
  `StartAll` seals the composition.
- **Arbitration** (SemEngine `config/doc.go:102-117`): the file reaches the bucket only on first
  boot or when its version (`<generation>.<config_revision>.0`) is newer. Otherwise the bucket's
  components replace the file's.
- **Writes before sealing** (`config/doc.go:135-140`): `PutComponentToKV` and
  `DeleteComponentFromKV` update `GetConfig` before returning. A write made before
  `createServiceManager` is therefore in this boot's composition.
- **Branches** expand to `{git,ast,doc,cfgfile}-source-<BranchScopedSlug>` via `sourcespawn.Build`.
- **`BranchWatcher.Discover` is not read-only.** It creates worktrees (`EnsureWorktree`); only
  `ListBranches` plus `MatchBranches` are side-effect free.
- **Submodules:** `internal/subwatch` builds parent-scoped components per materialized submodule
  (project = URL slug, version = 12-hex gitlink SHA) through the same Put/Delete store.
- **A remote multi-branch source with no checkout yet** reaches `ListBranches` on a missing
  path, which returns an error. `loadAndExpandConfig` turns that into a boot error. This is from
  reading the code, not measured; task 2.1 confirms it.

## Goals / Non-Goals

Goals: discovery takes effect at the boot that finds it, on any store; runtime is read-only and
reports `restart_required`; failures are typed status. Non-goals are in `proposal.md`.

## Decisions

### D1. Where discovery enters the composition

- **A. Reconcile into the config manager before sealing (chosen).** After `startConfigManager`,
  Put the discovery-owned components that are missing and Delete the stale ones, then build the
  service manager. This works whatever arbitration chose, and needs no version games. Cost: one
  new boot step. Every Put and Delete is retained for the next boot too, which is the desired
  behaviour.
- **B. Fold the discovered set into the file version.** Rejected: a set digest is not monotonic,
  and it would make every file edit win. **C. Require a `config_revision` bump.** Rejected: a new
  branch would need a hand edit, which defeats discovery.

### D2. Which components discovery owns

A component is discovery-owned only if its name is one discovery derives from a parent source
in the current effective configuration:

- a branch component name for one of that parent's patterns' branches, or
- a subwatch name for one of that parent's submodules.

Reconcile never reads, puts or deletes any other name. When a parent leaves the configuration,
its derived names are deleted at the next boot, together with the parent.

Rejected: a marker field in the component config, because retained entries written earlier lack it.

### D3. Runtime removals win over rediscovery (owner question Q1)

A discovery-owned component that an operator removed at run time is held retained-disabled
(#271). Reconcile SHALL NOT re-enable it, so the next boot keeps it removed. Re-adding it takes an
explicit add.

Recommended: otherwise every restart silently undoes an operator's removal.

### D4. Unsupported or failed discovery

Boot never fails because of discovery. Each parent source gets a typed discovery state:

| state | when | effect |
|---|---|---|
| `ok` | listing succeeded | children reconciled |
| `awaiting_checkout` | remote source with no checkout yet | parent only; the runtime check flips `restart_required` once the clone exists |
| `listing_failed` | git error | previously admitted children are kept, not deleted; `error` holds the message |
| `capped` | `max_branches` reached | count of matches not admitted |

`listing_failed` keeps children: deleting on a transient error drops live branches.

### D5. Runtime check

One goroutine per parent with discovery enabled, on `branch_poll_interval` (default 15s):

- **Branches:** `ListBranches` plus `MatchBranches`.
- **Submodules:** `ListSubmodules` on the checkout.

It compares the result with the boot-admitted set held in memory, and publishes this block on
the manifest status:

```
discovery: { restart_required, sources: [{ source, state, error?,
  pending: [{ kind: branch|submodule, name, change: added|removed|changed }] }] }
```

It writes nothing, creates no worktrees, and runs under source-manifest's Start context.

### D6. Dead code

`startBranchWatchers`, `startSubmoduleWatchers`, `publishBranchComponents`,
`removeBranchComponents` and subwatch's `Run` loop are deleted. subwatch keeps its build logic,
which D1 calls at boot. The two `capability: blocked` warnings are deleted with them.

## Risks / Trade-offs

- **Retained growth** is bounded: stale derived names are deleted each boot, and a recreated
  branch gets the same names and IDs. **Boot latency:** one worktree per new branch, bounded by
  `max_branches`.
- **Entities from deleted branches stay in the graph** until #215. Status lists the removal as
  pending, so nothing claims they are gone.
- **SemEngine #646** (keys a newer file lacks return on the next boot): a discovery-owned name
  that reconcile deleted could reappear after a newer-file push. Reconcile runs on every boot and
  deletes it again, so the effect is bounded to one boot step.

## Migration Plan

Additive: explicit `branch:` sources and `submodules: false` are unchanged; the first boot after
upgrade keeps admitted branch names and adds submodule components.

## Open Questions

- **Q1 (owner):** D3, whether a runtime removal beats rediscovery. Recommended: yes.
- **Q2:** whether `listing_failed` should also set `restart_required` when the boot set may be
  stale. Recommended: no; report the state, and let `restart_required` mean "a restart would
  change the set".
