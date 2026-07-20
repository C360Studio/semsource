## MODIFIED Requirements

### Requirement: Splitting is deterministic and respects document structure

The splitter SHALL be a pure function of the document bytes: identical input MUST always yield identical passage boundaries, independent of machine, run order, or wall-clock time. Passage boundaries MUST fall on structural boundaries — heading, paragraph, sentence, or a key-group boundary inside a homogeneous key/value block — except where a single sentence exceeds the ceiling. A fenced code block MUST NOT be split across passages **unless** it is a homogeneous key/value list, which MAY be divided on key-group boundaries at any size.

A fenced block is a homogeneous key/value list when its non-blank lines are predominantly `KEY=VALUE` assignments. Such a block is divided by grouping consecutive lines on the key's leading token up to the first underscore, and only when the block yields at least three distinct groups. The resulting groups are exempt from the below-floor merge, which would otherwise reassemble the block.

#### Scenario: Splitting is reproducible

- **WHEN** the same document bytes are split twice in separate processes
- **THEN** the resulting passage boundaries and bodies are byte-identical

#### Scenario: An oversized section is subdivided

- **WHEN** a single heading's section exceeds the chunk size ceiling
- **THEN** it is subdivided on paragraph boundaries
- **AND** no resulting passage exceeds the ceiling

#### Scenario: A fenced code block spans a would-be boundary

- **WHEN** a fenced code block of ordinary code straddles a candidate split point
- **THEN** the code block is kept whole within one passage

#### Scenario: A homogeneous key/value block is divided by key group, under the ceiling

- **WHEN** a fenced block of `KEY=VALUE` lines yields at least three distinct leading-token groups
- **AND** the block is smaller than the chunk size ceiling
- **THEN** it is divided into one passage per key group
- **AND** keys sharing a leading token stay in the same passage

#### Scenario: A small or uniform key/value block is left whole

- **WHEN** a fenced `KEY=VALUE` block yields fewer than three distinct leading-token groups
- **THEN** it is kept whole, because splitting it would add passages without separating facts

#### Scenario: Key groups survive the below-floor merge

- **WHEN** dividing a key/value block produces groups smaller than the chunk size floor
- **THEN** those groups are still emitted as separate passages rather than merged back together

#### Scenario: Trivial sections do not each become a passage

- **WHEN** a document contains consecutive headings whose sections fall below the chunk size floor
- **THEN** those sections are merged into a shared passage rather than emitted individually

#### Scenario: Dividing a block preserves the document byte for byte

- **WHEN** a key/value block is divided into groups
- **THEN** the passage bodies still tile the document with no gap, no overlap and no duplicated
  text, including the section heading and the fence markers
