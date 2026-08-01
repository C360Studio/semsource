## ADDED Requirements

### Requirement: A file resolves to exactly one parser, deterministically

Given a watch path's declared language set and a file, the parser that reads that file SHALL be a
total, deterministic function of those two inputs alone. The same file under the same declared
languages MUST resolve to the same parser on every run, in every process, regardless of map
iteration order, filesystem order, or the order languages appear in configuration.

Resolution SHALL NOT depend on the file's contents. A language whose syntax overlaps another's
cannot be told apart reliably by inspection, and a wrong guess silently changes the `{domain}`
segment of every entity in the file — which is part of its identity.

#### Scenario: An extension claimed by two declared languages

- **GIVEN** a watch path declaring two languages that both claim one file extension
- **WHEN** a file with that extension is parsed repeatedly
- **THEN** the same parser handles it every time
- **AND** the entities produced carry the same `{domain}` segment every time

#### Scenario: Resolution ignores file contents

- **GIVEN** two files with the same extension under the same declared languages, one containing
  syntax specific to the other language
- **WHEN** each is parsed
- **THEN** both are read by the same parser, chosen from the declared languages

#### Scenario: A file no declared language claims

- **WHEN** a file's extension is claimed by none of the watch path's declared languages
- **THEN** it is skipped, and no entity is produced for it

### Requirement: An unsupported language fails loudly rather than defaulting

Every configured language SHALL either resolve to a registered parser and its own domain, or fail.
A language that is unrecognized MUST NOT fall back to another language's parser, extensions, or
`{domain}` segment.

A silent fallback is worse than an error here: it publishes entities that claim to be a language
they are not, which is a graph-wide identity defect that no query can distinguish from the truth.

#### Scenario: A watch path declares a language with no parser

- **WHEN** a watch path declares a language for which no parser is registered
- **THEN** configuration is rejected with the offending language named
- **AND** no source begins ingesting with a substituted parser

#### Scenario: A registered parser has no domain mapping

- **WHEN** a parser is registered but its language is not mapped to a domain
- **THEN** the gap is detected rather than resolved to a default language
