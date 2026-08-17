# Design — add identity flags

Small change; three decisions worth recording.

## D1. Flag names mirror the JSON keys exactly

`--project` / `--version` ↔ `project` / `version`. No short forms, no
aliases — the CLI flag IS the config key, so the README's config reference
doubles as flag documentation.

## D2. `ast` and `repo` only

`SourceEntry.Version` is documented for ast and expanded-repo entries;
`config/expand.go` already copies `Project`/`Version` from a `repo` entry
onto its expanded code entries. `git`/`docs`/`config` sources keep
config-file-only identity until a need appears (the doc/cfg `project`
override from beta.8 remains reachable by file).

## D3. No new validation

Flag values enter `SourceEntry` and flow through the identical
`config.LoadConfig`/`Validate` path a hand-edited file takes. Inventing
flag-side validation would fork the rules; `semsource validate` remains the
single checker.
