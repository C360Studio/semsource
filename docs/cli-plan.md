# CLI Implementation Plan & Status

## Overview

SemSource CLI provides an interactive setup wizard and subcommand structure to help users configure and manage source ingestion without manually editing JSON config files.

## Phases

### Phase 0: YAML to JSON Migration -- COMPLETE

- All config structs use `json:"..."` tags
- Loader uses `encoding/json` with `DisallowUnknownFields()`
- Default config path is `semsource.json`
- Example config at `configs/semsource-example.json`
- `yaml.v3` retained in go.mod only for `source/parser/markdown.go` frontmatter

### Phase 1: CLI Subcommands & Wizard -- COMPLETE

**Subcommands implemented:**
- `semsource init` -- Interactive setup wizard, creates `semsource.json`
- `semsource run` -- Start the ingestion engine
- `semsource add` -- Add a source (interactive or flag-based)
- `semsource remove` -- Remove a source (interactive or `--index N`)
- `semsource sources` -- List configured sources in a table
- `semsource validate` -- Check config without starting
- `semsource version` -- Print version

**Packages created:**
- `cli/` -- Terminal I/O (`Term`), wizard registry (`SourceWizard` interface), all source wizards, add/validate/sources commands
- `cmd/semsource/` -- Subcommand dispatch via `os.Args` switch + `flag.NewFlagSet`

**Source wizards registered:**
| Wizard | TypeKey | Status |
|--------|---------|--------|
| AST | `ast` | Available |
| Git | `git` | Available |
| Docs | `docs` | Available |
| Config | `config` | Available |
| URL | `url` | Available |
| Video | `video` | Coming soon (placeholder) |

### Phase 2: Post-Review Fixes -- COMPLETE

Must-fix issues from go-reviewer (all resolved):

1. **EOF infinite loop in `Term.Select`** (`cli/term.go`) -- Fixed: EOF detection returns error instead
   of looping.
2. **Git wizard allows empty URL** (`cli/wizard_git.go`) -- Fixed: prompt loops until non-empty input.
3. **Doc/Config wizards allow zero paths** -- Fixed: `MultiLine` requires at least one path.
4. **URL wizard allows zero URLs** -- Fixed: requires at least one URL.

Should-fix issues (all resolved):

1. Dead `autoRun()` removed -- `run_v2.go` was renamed to `run.go`; `runV2Cmd` renamed to `runCmd`.
   The dead code path no longer exists.
2. Unused `_ string` parameter in `parseGlobalFlag` removed (`cmd/semsource/main.go`).
3. Bare `semsource` auto-runs when `semsource.json` exists -- this is the intended behavior and is
   confirmed working.

### Phase 3: Enhancements -- FUTURE

- Evaluate `charmbracelet/huh` for richer interactive forms
- Video streaming source wizard (when handler is implemented)
- Config migration command (`semsource migrate`)
- Shell completions

## Architecture Notes

### Extensibility Pattern

New source types are added by:
1. Creating a file `cli/wizard_<type>.go`
2. Implementing `SourceWizard` interface (Name, TypeKey, Description, Available, Prompts)
3. Calling `RegisterSourceWizard()` in `init()`

The `Available() (bool, string)` method allows "coming soon" placeholders (e.g., video).

### Testing Strategy

- `Term` wraps `io.Reader`/`io.Writer` for fully testable terminal I/O
- All wizard prompts are testable via piped string input
- No external dependencies (stdlib only)
