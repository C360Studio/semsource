## MODIFIED Requirements

### Requirement: Splitting is deterministic and respects document structure

The splitter SHALL be a pure function of the document bytes: identical input MUST always yield identical passage boundaries, independent of machine, run order, or wall-clock time. Passage boundaries MUST fall on structural boundaries — heading, paragraph, sentence, a key-group boundary inside a homogeneous key/value block, or a fence boundary where a fenced block is isolated from the prose around it — except where a single sentence exceeds the ceiling. A fenced code block MUST NOT be split across passages **unless** it is a homogeneous key/value list, which MAY be divided on key-group boundaries at any size.

A fenced block is a homogeneous key/value list when its non-blank lines are predominantly `KEY=VALUE` assignments. Such a block is divided by grouping consecutive lines on the key's leading token up to the first underscore, and only when the block yields at least three distinct groups. The resulting groups are emitted individually even when they fall below the floor: the below-floor merge operates on whole sections before subdivision, so it never reaches spans created inside one.

A fenced block that is not a homogeneous key/value list SHALL be isolated as its own passage when the section's non-fence content is at least the chunk size floor — that is, when there is enough surrounding prose to dilute it. Isolation never divides the block; it only decides where the block's passage begins and ends. A section whose non-fence content falls below the floor is left un-isolated, because there is nothing substantial to separate the block from.

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

#### Scenario: A fenced block surrounded by substantial prose is isolated

- **WHEN** a section holds a fenced block that is not a homogeneous key/value list
- **AND** the section's non-fence content is at least the chunk size floor
- **THEN** the block becomes its own passage, whole
- **AND** the surrounding prose tiles alongside as its own passage or passages

#### Scenario: A fenced block with little surrounding prose is not isolated

- **WHEN** a section's non-fence content falls below the chunk size floor
- **THEN** the fenced block is not isolated
- **AND** the section splits — or does not — exactly as it would on size alone

#### Scenario: Isolation never divides a continuous construct

- **WHEN** a fenced block holding one continuous construct — a function body, a JSON document, a
  multi-line pipeline, a here-document — is isolated
- **THEN** the block is emitted as a single passage, never divided internally

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

#### Scenario: Dividing or isolating preserves the document byte for byte

- **WHEN** a key/value block is divided into groups, or a fenced block is isolated from its prose
- **THEN** the passage bodies still tile the document with no gap, no overlap and no duplicated
  text, including the section heading and the fence markers

### Requirement: Passages are named and citable

Every passage entity SHALL carry `dc.terms.title` qualified by its parent document's title and by
the passage's full heading ancestry — every heading level enclosing the passage's section, outermost
first — because the title is the identity text retrieval ranks by, and the ancestry is what keeps
two same-named facts under different parent sections distinguishable to the embedding. Leading
ancestry components that repeat the document title SHALL be dropped rather than stuttered. Every
passage SHALL carry `DocChunkIndex` with its ordinal. A passage under a heading SHALL additionally
carry `DocSection` with the immediate heading text — not the ancestry — because the section anchor
the docs lens exposes as the passage's locator fragment derives from it and must match the document
verbatim.

#### Scenario: A passage under a heading is labelled

- **WHEN** a passage derived from a headed section is returned in a relations listing
- **THEN** its label identifies both the parent document and the section

#### Scenario: A nested section's title carries its ancestry

- **WHEN** a passage derives from a section nested under a higher-level heading
- **THEN** its `dc.terms.title` names the enclosing heading or headings ahead of the section's own,
  outermost first

#### Scenario: A section that repeats the document title is not stuttered

- **WHEN** a document's first heading repeats the document's own title
- **THEN** passage titles drop the repeated component rather than qualifying the title with itself

#### Scenario: Passages from different documents share a heading

- **WHEN** passages from two different documents both derive from a section with identical heading
  text
- **THEN** their labels are distinguishable

#### Scenario: A headingless passage is labelled

- **WHEN** a passage carries no heading
- **THEN** it still carries a `dc.terms.title` and is name-resolvable

#### Scenario: A cited passage deep-links to its section

- **WHEN** a passage derived from a headed section is cited in a fusion answer
- **THEN** its locator carries the section anchor as the fragment
- **AND** the anchor derives from the immediate heading text, unaffected by the title's ancestry
