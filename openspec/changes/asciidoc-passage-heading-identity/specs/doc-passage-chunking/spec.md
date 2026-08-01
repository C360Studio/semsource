## ADDED Requirements

### Requirement: Heading recognition covers every ingested markup format

A passage's heading and heading ancestry SHALL be derived using the heading syntax of the
document's own format. A document whose format expresses section titles SHALL yield passages
carrying those titles; a format with no heading syntax SHALL yield passages with no heading, which
remains a valid state.

Format SHALL be supplied by the caller, which resolves it from the document's extension, rather than
inferred from the bytes. Inference is not safe here: markdown's setext headings underline text with
`=`, which is the same character AsciiDoc uses as a section prefix, so byte-sniffing can misread one
format as the other.

Recognising a format's headings SHALL NOT imply interpreting the rest of its markup. Attribute
entries, block titles, includes, and conditionals remain pass-through text.

#### Scenario: An AsciiDoc section title becomes passage identity

- **GIVEN** an AsciiDoc document whose sections are introduced by `=`-prefixed titles
- **WHEN** it is split into passages
- **THEN** its passages carry the section title as their heading, and the enclosing titles as their
  heading ancestry, exactly as the equivalent markdown document does

#### Scenario: Equivalent documents in two formats agree

- **GIVEN** the same content expressed once in markdown and once in AsciiDoc
- **WHEN** both are split
- **THEN** the passages carry the same headings and the same heading ancestry

#### Scenario: A format without headings still produces passages

- **GIVEN** a plain-text document with no heading syntax
- **WHEN** it is split
- **THEN** passages are produced and carry no heading, and that is not an error

## MODIFIED Requirements

### Requirement: Splitting is deterministic and respects document structure

The splitter SHALL be a pure function of the document bytes **and the document's format**: identical
input MUST always yield identical passage boundaries, independent of machine, run order, or
wall-clock time. Format is an input rather than a hidden dependency precisely so the function stays
pure and reproducible. Passage boundaries MUST fall on structural boundaries — heading, paragraph, sentence, a key-group boundary inside a homogeneous key/value block, or a fence boundary where a fenced block is isolated from the prose around it — except where a single sentence exceeds the ceiling. A fenced code block MUST NOT be split across passages **unless** it is a homogeneous key/value list, which MAY be divided on key-group boundaries at any size.

A fenced block is a homogeneous key/value list when its non-blank lines are predominantly `KEY=VALUE` assignments. Such a block is divided by grouping consecutive lines on the key's leading token up to the first underscore, and only when the block yields at least three distinct groups. The resulting groups are emitted individually even when they fall below the floor: the below-floor merge operates on whole sections before subdivision, so it never reaches spans created inside one.

A fenced block that is not a homogeneous key/value list SHALL be isolated as its own passage when the section's non-fence content is at least the chunk size floor — that is, when there is enough surrounding prose to dilute it. Isolation never divides the block; it only decides where the block's passage begins and ends. A section whose non-fence content falls below the floor is left un-isolated, because there is nothing substantial to separate the block from.

#### Scenario: Splitting is reproducible

- **WHEN** the same document bytes are split twice in separate processes
- **THEN** the resulting passage boundaries and bodies are byte-identical

#### Scenario: Markdown splitting is unchanged by format awareness

- **WHEN** a markdown document is split after heading recognition becomes format-aware
- **THEN** its passage boundaries, bodies, headings, and ancestry are identical to before
