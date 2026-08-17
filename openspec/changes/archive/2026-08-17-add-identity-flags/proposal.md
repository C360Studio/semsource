# `semsource add` learns explicit identity: `--project` / `--version`

Tracks [#189](https://github.com/C360Studio/semsource/issues/189).

## Why

`SourceEntry` supports the version-intelligence pair — `project` (explicit
project slug) and `version` (revision scoping) — and those fields are
load-bearing product surface: they power `code_changes` version diffs
("same project at two versions") and the standalone-source-merges-with-a-
submodule-pin dedup the quickstart demonstrates. But `semsource add`'s
non-interactive parsers for `ast` and `repo` expose neither flag, so
explicit identity is config-file-only. The obvious command for the
documented story — `semsource add ast --path ./depA-1.9.0 --project depA
--version 1.9.0` — does not exist. (Found resolving the onboarding-quickstart
design's open question against the real CLI; filed rather than grown there.)

## What Changes

- `semsource add ast` and `semsource add repo` gain `--project` and
  `--version` string flags, passing through to the existing `SourceEntry`
  fields (same validation path as config-file values; omitted flags leave
  today's derivation behavior untouched).
- README's Managing Sources gains the version-intelligence example; the
  Config File section's `project`+`version` explanation points at the CLI
  form; the quickstart's multi-repo prose notes the flags (the marked
  config-file block stays — it is the teaching artifact for multi-source
  shape, and the doc-driven e2e step tables stay untouched).

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `cli-onboarding`: adds a requirement — non-interactive source registration
  can declare explicit `project`/`version` identity, with omission preserving
  automatic derivation.

## Impact

- `cli/add.go` (two parsers), `cli` tests, README, `docs/QUICKSTART.md`
  prose. No config-schema, engine, or wire changes — the fields already
  exist end to end (repo expansion copies them onto expanded entries).

## Non-goals

- No flags for `git`/`docs`/`config` sources (`version` is documented for
  ast and expanded repo entries; other types keep config-file-only until a
  need appears).
- No new validation: flag values feed the exact path config-file values do.
