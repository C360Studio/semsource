# Forward a repo source's index_interval to its ast child

Approved-by:

Track: small (one config pass-through; no identity, payload, or exported-surface change).

## Why

Dogfood finding F7 (#400). `config.ExpandRepoSources` builds the child `ast` entry without `IndexInterval` in both
the single-branch path (`config/expand.go:153`) and the per-branch path (`expandBranchSources`, `:317`). The spawn
path then falls back to `60s` whenever `watch: true` (`internal/sourcespawn/components.go:114-117`), and
`semsource validate` passes the config. A declared `index_interval: 30m` on a repo source ran every 60 s on the mini.
No spec requirement covers `index_interval` today, so nothing said it must survive expansion.

## What Changes

- Both expansion paths copy the repo entry's `IndexInterval` onto the `ast` child.
- An ADDED requirement in `ast-source-configuration`: a repo source's `index_interval` governs its ast child, and an
  unset one leaves the spawn default in force.
- A test covers both paths, set and unset; it fails on main.

## Decisions you are making by approving

- **`index_interval` on a repo source applies to its ast child only.** The git poll and docs children are unchanged;
  they have no reindex interval today.
- **No refusal for the old behaviour.** A config that declared `index_interval` on a repo source now gets the interval
  it declared; nothing that booted before is refused.

## Out of scope

- The idle reindex churn itself (#401).
- `index_interval` on any other source type.
