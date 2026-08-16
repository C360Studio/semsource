# Design — guard-minified-asset-ingestion

## Context

See proposal.md — Why. The mechanics that matter: `parseFileWithWatcher`
(processor/ast-source/component.go) is the single choke point every parsed file
passes through — routing table first (`pw.routes[ext]`), then
`parser.ParseFile` under `pw.parseMu`. Symbol publishing happens later, per
file, in `publishParseResult`. The seed loop (`runSeed`) already owns the
"Initial index complete" summary line. The pre-publish liveness counters
(`files_parsed`, PR #174) increment on successful parse.

## Goals / Non-Goals

- **Goal**: keep vendored minified assets out of the symbol graph at the
  cheapest possible point (before tree-sitter ever runs), with a cap backstop
  and aggregate visibility.
- **Non-goal**: a general vendored-path heuristic (e.g. skipping
  `src/main/resources/**`). Path guessing misclassifies real code; the two
  content-anchored rules cover the measured problem.
- **Non-goal**: retroactive graph cleanup / RETRACT of junk already published
  by older builds. Fresh-storage posture (beta.161) makes reseed the cleanup.
- **Non-goal**: doc/config/url handlers — only the AST source extracts
  symbols; nothing else has this failure mode.

## Decisions

1. **Guard sits in `parseFileWithWatcher`, after route resolution, before
   `ParseFile`.** Alternative — inside each language parser — rejected: seven
   parsers would each need the rule, and the guard's point is to avoid the
   parse entirely (a 3.5 MB single-line file is real tree-sitter cost).
   Alternative — in `parseDirectory`'s walk — rejected: watch events and
   periodic reindex enter through `parseFileWithWatcher` too, and the guard
   must hold for them equally.

2. **Detection = name rule OR content probe.** Name rule: case-insensitive
   suffixes `.min.js`, `-min.js`, `.min.css`, `-min.css` on the base name.
   Content probe (only when the name rule misses): read up to the first 64 KiB;
   minified iff `bytes_read / max(lines_in_read, 1) > 512`. Hand-written and
   generated-but-real code measures well under 200 bytes/line on the corpora we
   score (Go/Java/TS/Python); minified plotly measures in the tens of
   thousands. 64 KiB bounds the extra read for the common (non-minified) case
   to one page-cache hit; the parser re-reads anyway, and correctness beats
   saving one read. Alternative — full-file read for the probe — rejected: the
   probe exists to be cheap, and 64 KiB of a minified bundle is decisive.
   Threshold 512 chosen at ~4x hand-written worst case rather than closer to
   it: a false "minified" verdict silently unindexes real code, the worse
   failure. CSS files route to no parser today, so the `.css` name rules are
   inert until a CSS parser exists — kept anyway (they cost nothing and the
   spec names the convention).

3. **File entity still published.** The minified skip returns a synthetic
   one-entity ParseResult (the file container), not nil: the file exists,
   supports navigation/completeness, and keeps `files_parsed` semantics
   (the file WAS handled). The parser normally builds the file entity during
   parse; the guard constructs it the same way (`ast.NewCodeEntity(...,
   TypeFile, ...)`) without parsing.

4. **Cap enforced on the ParseResult, not in the parser.** After `ParseFile`
   returns, if symbol entities (non-file) exceed `max_symbols_per_file`, strip
   them, keep file-level entities, WARN once with path+count. Config:
   `max_symbols_per_file` on the component config, default **5000**, `0` =
   disabled, negative rejected by `Validate()`. Default rationale: the largest
   legitimate file measured on our corpora (generated serializers included) is
   in the low hundreds of symbols; 5000 leaves an order of magnitude of
   headroom while still catching the ~10–15k-per-file plotly members.

5. **Aggregation via two component atomics** (`filesWithheld`,
   `symbolsWithheld`), summarized in `runSeed` right after "Initial index
   complete" — one line, only when nonzero. Watch-event guard hits log at
   Debug per file (rare, and there is no "end of watch" to aggregate to).
   Not added to the status report: this is seed diagnostics, not readiness,
   and the status surface's contract stays as-is.

## Risks / Trade-offs

- [False positive unindexes real code] → threshold at 4x hand-written worst
  case; name rule only fires on conventional minified suffixes; spec scenario
  pins "hand-written code is not misclassified"; cap disable value documented.
- [False negative (pretty-printed vendored code)] → out of scope by decision;
  the cap catches the pathological tail; #175 stays closable on the measured
  corpus.
- [Content probe misreads short files] → probe only runs when the name rule
  misses AND the file is large enough to matter (< 4 KiB files skip the probe
  entirely — a minified file that small cannot flood anything).
- [Behavior change for anyone who wanted minified symbols] → `0` cap disables
  the backstop, but detection has no off switch in this slice; if a real
  consumer surfaces, a config flag is a one-line follow-up.

## Migration Plan

Ship in the next tag; no data migration (fresh-storage posture). Rollback =
revert the commit; re-seed restores prior behavior. The OSH acceptance
comparison (proposal — Impact) is the deploy gate.

## Open Questions

- None blocking. Threshold constants live in one place with the measured
  numbers cited, so retuning is a constant edit, not a design change.
