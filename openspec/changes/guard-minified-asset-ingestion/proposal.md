# Guard minified-asset ingestion

## Why

The beta.161 OSH acceptance run (2026-08-16) measured what one vendored minified
asset family does to the graph: `plotly-latest-min.js` and friends under
`sensorhub-webui-core` parse into **~34.9k junk entities** (31,837
`javascript.var` + 3,033 `javascript.function`, mostly single-character minified
identifiers) — **45% of all 77,802 publishes on that corpus**. The flood is the
entire historical publish plateau (~5 minutes of the seed), it breached the
GRAPH stream's 256 MiB ceiling (semsource#178), and every junk entity that
lands costs index writes, embedding time, and consumer trust. Filed as
semsource#175 and confirmed a release blocker: trash in the graph is not
acceptable.

The spec instinct already exists for edges — "a wrong edge is worse than a
missing one." The same holds here: a junk entity is worse than an unindexed
vendored asset.

## What Changes

- **Minified-asset detection at parse routing time.** A file that is minified
  by name (`*.min.js`, `*-min.js`, `*.min.css`, `*-min.css`) or by content
  shape (average line length far beyond hand-written code) is indexed as its
  FILE entity only: no symbol extraction, and no tree-sitter parse at all (a
  3.5 MB single-line file is also pure parser waste).
- **Per-file symbol cap as a loud backstop.** A parse result whose symbol count
  exceeds a configurable cap (default 5,000; 0 disables) keeps its file-level
  entity and drops the symbol entities with one WARN naming the path and count.
  A 30k-symbol "file" is never signal, whatever slipped past detection.
- **One seed-end summary, not per-file spam.** Skipped/capped files aggregate
  into a single summary line at the end of the initial index (count + total
  symbols withheld), with per-file detail at Debug — the ADR-0011 rule (control
  volume by aggregating, never by lowering severity).
- No RETRACT/cleanup migration: the beta.161 fresh-storage posture means a
  reseed re-derives the graph without the junk.

## Capabilities

### New Capabilities

- `asset-ingestion-guards`: which files the AST source deliberately does NOT
  extract symbols from (minified assets, symbol-cap breaches), what those files
  get instead (file entity only), and how the withholding is surfaced
  (aggregate summary, loud cap WARN).

### Modified Capabilities

<!-- none: the new knob (max_symbols_per_file) ships inside the new capability's
     requirements; ast-source-configuration's existing requirements (watch-path
     shape, language validation, entry points) are unchanged. -->

## Impact

- `processor/ast-source`: detection before `parser.ParseFile` in the routing
  path (`parseFileWithWatcher`), cap enforcement on the parse result, skip
  counters + seed-end summary in `runSeed`; config gains `max_symbols_per_file`
  (component-level, schema-tagged, validated).
- `source/ast`: none — parsers are untouched; the guard sits above them.
- Scorecard/acceptance: measured on the pinned OSH corpus
  (`corpus-osh.sh` @ `235c0eab`): raw publishes ~77.8k → ~43k, publish plateau
  eliminated, zero `maximum bytes exceeded` failures, `beta161-osh-v2`
  question set stays 11/13 (the two reds are semstreams#823, unrelated).
- Issues: closes semsource#175; de-knife-edges semsource#178; shrinks the
  blast radius of semsource#176's WARN flood on this corpus.
