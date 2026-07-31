## MODIFIED Requirements

### Requirement: Splitting is deterministic and respects document structure

The splitter SHALL be a pure function of the document bytes: identical input MUST always yield identical passage boundaries, independent of machine, run order, or wall-clock time. Passage boundaries MUST fall on structural boundaries — heading, paragraph, sentence, or an entry-group boundary inside a fenced block of independent peer entries — except where a single sentence exceeds the ceiling. A fenced code block MUST NOT be split across passages **unless** its lines are independent peers, in which case it MAY be divided on entry-group boundaries at any size.

A fenced block holds independent peer entries when its non-blank, non-comment lines are predominantly self-contained units that do not depend on one another for meaning — `KEY=VALUE` assignments, whole shell commands, or single-line flag or option declarations. Such a block is divided by grouping consecutive lines on their leading token, and only when the block yields at least three distinct groups. A block whose lines form a continuous construct — a function body, a multi-line pipeline, a here-document, a data structure spanning lines — is NOT a peer block and MUST be kept whole, because its parts are not independently meaningful.

The resulting groups are emitted individually even when they fall below the floor: the below-floor merge operates on whole sections before subdivision, so it never reaches spans created inside one.

#### Scenario: Splitting is reproducible

- **WHEN** the same document bytes are split twice in separate processes
- **THEN** the resulting passage boundaries and bodies are byte-identical

#### Scenario: An oversized section is subdivided

- **WHEN** a single heading's section exceeds the chunk size ceiling
- **THEN** it is subdivided on paragraph boundaries
- **AND** no resulting passage exceeds the ceiling

#### Scenario: A fenced block of continuous code spans a would-be boundary

- **WHEN** a fenced code block whose lines form one continuous construct straddles a candidate split point
- **THEN** the code block is kept whole within one passage

#### Scenario: A block of independent commands is divided, under the ceiling

- **WHEN** a fenced block contains at least three self-contained shell commands
- **AND** the block is smaller than the chunk size ceiling
- **THEN** it is divided into one passage per command group
- **AND** a command that continues across lines stays whole within its group

#### Scenario: A homogeneous key/value block is divided by key group, under the ceiling

- **WHEN** a fenced block of `KEY=VALUE` lines yields at least three distinct leading-token groups
- **AND** the block is smaller than the chunk size ceiling
- **THEN** it is divided into one passage per key group
- **AND** keys sharing a leading token stay in the same passage

#### Scenario: A small or uniform peer block is left whole

- **WHEN** a fenced peer block yields fewer than three distinct leading-token groups
- **THEN** it is kept whole, because splitting it would add passages without separating facts

#### Scenario: Entry groups survive the below-floor merge

- **WHEN** dividing a peer block produces groups smaller than the chunk size floor
- **THEN** those groups are still emitted as separate passages rather than merged back together

#### Scenario: Trivial sections do not each become a passage

- **WHEN** a document contains consecutive headings whose sections fall below the chunk size floor
- **THEN** those sections are merged into a shared passage rather than emitted individually

#### Scenario: Dividing a block preserves the document byte for byte

- **WHEN** a peer block is divided into groups
- **THEN** the passage bodies still tile the document with no gap, no overlap and no duplicated
  text, including the section heading and the fence markers
