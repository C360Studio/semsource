# Operator config revision

Approved-by:

## Why

Any edit to `semsource.json` after first boot is silently ignored on a retained store (#266). Measured on
main `4ca2072`: boot with one source, `semsource add` a second, restart — the manifest still lists one
source and the log says "Synced configuration from KV". The same happens to an `embedder_type` change.

The cause is by design upstream and a gap here. SemEngine's config manager lets the file replace the
store only when the file's version is newer; at an equal version the store wins. `semsource.json` has no
version. `cmd/semsource/run.go` stamps the generated composition with a compile-time constant
(`1.0.0` in dd8d1cb, `2.0.0` in e63270c, `3.0.0` in 3604a9c) that only a release can change, so an
operator can never make their file newest. Nothing warns when the file and the store disagree.

## What Changes

- `semsource.json` gains an optional top-level `config_revision` (integer, default `0`).
- The composition version becomes `<generation>.<config_revision>.0`. The generation is the existing
  constant as an integer (`3`); a release bumps it when the generated composition changes. With
  `config_revision` absent the version is `3.0.0`, identical to today, so retained stores see no change.
- `semsource add` and `semsource remove` increment `config_revision` when they write the file, so a
  CLI change takes effect on the next boot of a retained store.
- After a boot where the file is newest, SemSource deletes retained component keys the file's composition
  lacks. SemEngine's push overlays and deletes nothing, so a removed source otherwise returns one boot later.
- At boot, when the retained store is in effect and its services, components, model registry or NATS
  settings differ from what the file generates, SemSource logs a warning naming the entries. Entries that
  exist only in the retained store are named separately, with the note that a newer file would drop them.
- `semsource init` over an existing file writes that file's revision plus one.
- A test fingerprints the composition generated from a fixed configuration and fails when it changes
  without a generation bump.
- README documents the field and how to change configuration on an existing deployment.

## Non-goals

- Bumping the store's version when a runtime change is accepted (SemEngine ask recorded on #266).
- A generation bump for #222. Every generation-3 `semengine_config_*` store was written by #222's
  composition, because #222 renamed the bucket.
- Writing runtime-accepted sources back to `semsource.json`. Open owner question on #266: a file that
  becomes newest replaces the store, so runtime-added sources absent from the file are dropped.
- An environment override for the revision.

## Impact

- `config`, `cli`, `cmd/semsource`, README. No entity identity, payload, or wire change.
- Compatible with "Removal overrides an original file-defined source": the same JSON at the same revision
  still loses to the retained store.
- Operators who hand-edit `semsource.json` must bump `config_revision`; the new warning tells them.
