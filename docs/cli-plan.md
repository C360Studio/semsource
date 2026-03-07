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

### Phase 1: CLI Subcommands & Wizard -- COMPLETE (pending reviewer fixes)

**Subcommands implemented:**
- `semsource init` -- Interactive setup wizard, creates `semsource.json`
- `semsource run` -- Start the ingestion engine
- `semsource add` -- Add a source (interactive or flag-based)
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

### Phase 2: Post-Review Fixes -- TODO

Must-fix issues from go-reviewer:

1. **EOF infinite loop in `Term.Select`** (`cli/term.go:90-101`) -- `readLine()` returns "" on EOF, causing Select to loop forever. Fix: detect EOF and return 0 or error.
2. **Git wizard allows empty URL** (`cli/wizard_git.go:19`) -- Prompt with empty default accepts empty input, producing invalid config. Fix: loop until non-empty.
3. **Doc/Config wizards allow zero paths** -- `MultiLine` can return empty slice, producing invalid config. Fix: require at least one path.
4. **URL wizard allows zero URLs** -- Same issue as doc/config. Fix: require at least one URL.

Should-fix issues:

1. Remove dead `autoRun()` in `cmd/semsource/run.go:76-81`
2. Remove unused `_ string` parameter in `parseGlobalFlag` (`cmd/semsource/main.go:149`)
3. Consider whether bare `semsource` should auto-run (currently starts engine if `semsource.json` exists)

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
