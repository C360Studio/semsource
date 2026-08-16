# asset-ingestion-guards Specification

## Purpose

Which files the AST source deliberately does NOT extract symbols from —
minified assets and per-file symbol-cap breaches — what those files get
instead (their file entity only), and how the withholding is surfaced.
Measured into existence by semsource#175: one vendored minified JS family was
45% of an entire corpus's publishes. A junk entity is worse than an unindexed
vendored asset.

## Requirements

### Requirement: Minified assets are indexed as files, never as symbols

A file the AST source identifies as a minified asset SHALL be represented in
the graph by its file entity only. No symbol entities (functions, variables,
classes, methods) SHALL be extracted from it, and the source SHALL NOT spend a
full parse on content it will not extract from.

Identification SHALL combine a name rule (conventional minified suffixes such
as `.min.js`, `-min.js`, `.min.css`, `-min.css`) with a content-shape rule
(line structure far outside hand-written code, e.g. extreme average line
length), so that a renamed minified blob is still caught and an ordinarily
long hand-written file is not.

The rationale is the standing identity instinct made spec: a junk entity is
worse than an unindexed vendored asset. One vendored `plotly-latest-min.js`
family measured at ~34.9k single-character symbol entities — 45% of an entire
corpus's publishes (semsource#175).

#### Scenario: A conventionally named minified asset

- **GIVEN** a watch path containing `plotly-latest-min.js`
- **WHEN** the initial seed indexes the path
- **THEN** the graph contains the file entity for `plotly-latest-min.js`
- **AND** contains zero symbol entities whose source location is that file

#### Scenario: A renamed minified blob is caught by shape

- **GIVEN** a minified JavaScript file whose name carries no minified suffix
  (e.g. `bundle.js` with multi-thousand-character lines)
- **WHEN** the file is indexed
- **THEN** it is treated as a minified asset (file entity only)

#### Scenario: Hand-written code is not misclassified

- **GIVEN** ordinary hand-written source files, including generated-but-real
  code with many symbols and occasional long lines
- **WHEN** the path is indexed
- **THEN** their symbols are extracted exactly as before the guard existed

### Requirement: A per-file symbol cap backstops detection, loudly

The AST source SHALL enforce a configurable per-file symbol cap. When one
file's parse yields more symbol entities than the cap, the source SHALL keep
the file-level entity, SHALL NOT publish that file's symbol entities, and
SHALL report the withholding at the default log level with the file path and
the symbol count — once per file, not once per symbol.

The cap SHALL be configurable per AST source instance with a default that
ordinary code never reaches (thousands), and a documented disable value for
operators who want everything. Validation SHALL reject a negative cap.

#### Scenario: A pathological file breaches the cap

- **GIVEN** a file that slips past minified detection and parses into more
  symbol entities than the configured cap
- **WHEN** the file is indexed
- **THEN** its file entity is published, none of its symbol entities are,
  and one default-level log entry names the path and the withheld count

#### Scenario: The cap is inert for ordinary code

- **GIVEN** the default cap and a corpus of hand-written source
- **WHEN** the corpus is indexed
- **THEN** no file triggers the cap and the published entity set is identical
  to the pre-guard behavior

#### Scenario: The cap can be disabled explicitly

- **GIVEN** a configuration that sets the documented disable value
- **WHEN** a pathological file is indexed
- **THEN** all its symbol entities are published (the operator asked for
  everything) and detection-by-name/shape still applies independently

### Requirement: Withholding is summarized, not sprayed

Guard activity SHALL aggregate: the initial seed SHALL emit one summary at its
completion stating how many files were withheld from symbol extraction —
split by cause (minified detection vs cap breach) — and the symbol count
withheld by cap breaches. A skipped minified file's symbol count is
unknowable by design (the file is never parsed) and SHALL NOT be estimated.
Per-file guard detail beyond the cap's single WARN SHALL sit below the
default level. A seed that withheld nothing SHALL emit no guard summary at
all.

This is the ADR-0011 rule applied to the guard: control volume by aggregating,
never by lowering the severity of the one line that matters.

#### Scenario: Seed-end summary on a corpus with vendored assets

- **GIVEN** a corpus containing minified assets
- **WHEN** the initial seed completes
- **THEN** exactly one summary line reports, at the default level, the
  withheld file counts by cause and the cap-withheld symbol total

#### Scenario: Clean corpus, silent guard

- **GIVEN** a corpus with no minified assets and no cap breaches
- **WHEN** the initial seed completes
- **THEN** no guard summary line is emitted
