## ADDED Requirements

### Requirement: Every byte of an ingested document is retrievable

SemSource SHALL index documents as passages such that no portion of an ingested document is
excluded from the semantic index because of its position in the file. No single indexed body SHALL
exceed the substrate's embedding truncation limit, so no passage is ever silently truncated.

#### Scenario: A document larger than the truncation limit is ingested

- **WHEN** a document whose body exceeds the substrate embedding truncation limit is ingested
- **THEN** it is represented by more than one passage entity
- **AND** every passage entity's stored body is at or below the chunk size ceiling
- **AND** the concatenation of the passage bodies in ordinal order reconstructs the document's
  indexed content with no gap

#### Scenario: Content near the end of a long document is retrievable

- **WHEN** a distinctive phrase appearing only after the truncation limit in a long document is
  queried through the docs lens
- **THEN** the passage containing that phrase is returned
- **AND** the answer is not a miss

#### Scenario: A short document is not fragmented needlessly

- **WHEN** a document smaller than the chunk size floor is ingested
- **THEN** it produces exactly one passage entity

### Requirement: A document body is never silently unindexed

The verbatim body store SHALL be mandatory for document ingestion: when it cannot be created,
startup MUST fail loudly rather than degrade to an un-offloaded, un-embedded corpus. Passage bodies
MUST NOT be carried inline in triples as a fallback.

#### Scenario: The body store is unavailable at startup

- **WHEN** the doc source cannot create its verbatim body store
- **THEN** startup fails with an explicit error naming the unavailable store
- **AND** the component does not report itself healthy or ready

#### Scenario: A passage body is offloaded, never inlined

- **WHEN** a passage entity is published
- **THEN** its body is addressed by a content-addressed handle in the body store
- **AND** no triple carries the passage body inline

### Requirement: Passage identity is deterministic, total, and collision-free

Every passage SHALL receive a six-part entity ID of the form
`{org}.semsource.web.{system}.chunk.{path-slug}-{ordinal}`, constructed through `entityid.Build`.
The ID SHALL be derivable from the parent document path and the passage ordinal alone, MUST NOT
incorporate content, timestamps, or insertion order, and MUST be assigned to every passage
including passages that carry no heading.

#### Scenario: The same document ingests twice

- **WHEN** an unchanged document is ingested twice
- **THEN** both passes produce byte-identical passage entity IDs

#### Scenario: A document contains repeated headings

- **WHEN** a document contains two sections with identical heading text
- **THEN** each produces a distinct passage entity ID

#### Scenario: A document has content before its first heading

- **WHEN** a document begins with prose above any heading
- **THEN** that prose is assigned a passage entity ID like any other passage

#### Scenario: A heading is renamed

- **WHEN** a section's heading text changes but its position in the document does not
- **THEN** the passage retains its existing entity ID
- **AND** no orphaned passage entity is created

### Requirement: The parent document entity remains the stable navigational node

The file-level document entity SHALL retain its existing entity ID, `dc.terms.title`,
`DocFilePath`, `DocMimeType`, `DocFileHash`, and provenance, and SHALL additionally carry
`DocChunkCount` stating how many passages it currently has. It SHALL NOT carry a body handle
(`DocBodyStore`/`DocBodyKey`) or a body `StorageRef`.

#### Scenario: A chunked document is ingested

- **WHEN** a document is ingested as passages
- **THEN** the parent entity carries `DocChunkCount` equal to the number of passage entities emitted
- **AND** the parent entity carries no `DocBodyStore` or `DocBodyKey` triple
- **AND** each passage entity carries its own body handle and `StorageRef`

#### Scenario: Doc identity survives an edit

- **WHEN** a document's content changes but its path does not
- **THEN** the parent entity ID is unchanged
- **AND** `DocFileHash` and `DocChunkCount` reflect the new content

### Requirement: Passages link to their parent document

Each passage entity SHALL carry a `code.structure.belongs` triple naming its parent document
entity, with the object marked as an entity reference. The docs lens SHALL declare this predicate
so a passage resolves to its parent and a parent resolves to its passages.

#### Scenario: A passage is returned as a seed

- **WHEN** a passage entity is the seed of a docs-lens relations query
- **THEN** its parent document is reachable under the parent role

#### Scenario: A document is returned as a seed

- **WHEN** a parent document entity is the seed of a docs-lens relations query
- **THEN** its passage entities are reachable under the chunk role

#### Scenario: Containment does not inflate impact

- **WHEN** an impact or paths walk runs over doc-domain entities
- **THEN** the passage-containment predicate does not participate in that walk

### Requirement: Splitting is deterministic and respects document structure

The splitter SHALL be a pure function of the document bytes: identical input MUST always yield
identical passage boundaries, independent of machine, run order, or wall-clock time. Passage
boundaries MUST fall on structural boundaries — heading, paragraph, or sentence — except where a
single sentence exceeds the ceiling, and a fenced code block MUST NOT be split across passages.

#### Scenario: Splitting is reproducible

- **WHEN** the same document bytes are split twice in separate processes
- **THEN** the resulting passage boundaries and bodies are byte-identical

#### Scenario: An oversized section is subdivided

- **WHEN** a single heading's section exceeds the chunk size ceiling
- **THEN** it is subdivided on paragraph boundaries
- **AND** no resulting passage exceeds the ceiling

#### Scenario: A fenced code block spans a would-be boundary

- **WHEN** a fenced code block straddles a candidate split point
- **THEN** the code block is kept whole within one passage

#### Scenario: Trivial sections do not each become a passage

- **WHEN** a document contains consecutive headings whose sections fall below the chunk size floor
- **THEN** those sections are merged into a shared passage rather than emitted individually

### Requirement: Passages that no longer exist do not serve as current

Passages that no longer exist SHALL be marked with the governed staleness marker
`entity.lifecycle.stale` when a document is re-ingested with fewer passages than before. This
MUST hold both on the ingest fast path and through the asynchronous lifecycle pass, so a crash
between publishing the parent and marking the tail does not leave phantoms indistinguishable from
facts. Passages SHALL NOT be deleted; the graph is retention-first.

#### Scenario: A document shrinks

- **WHEN** a document previously ingested as ten passages is re-ingested as seven
- **THEN** the passage entities at ordinals seven through nine carry `entity.lifecycle.stale`
- **AND** those entities are still present in the graph

#### Scenario: The lifecycle pass runs after an interrupted ingest

- **WHEN** the parent's `DocChunkCount` is lower than the highest ordinal of its live passage
  entities
- **THEN** the lifecycle pass marks every passage whose ordinal is at or above `DocChunkCount`

#### Scenario: A document regrows

- **WHEN** a shrunken document is re-ingested at or above its earlier passage count
- **THEN** the previously marked passage entities have the staleness marker cleared
- **AND** they carry the new content for their ordinal

#### Scenario: A live passage is never marked

- **WHEN** a document is re-ingested with the same or more passages
- **THEN** no passage entity below the new `DocChunkCount` carries the staleness marker

### Requirement: Passages are named and citable

Every passage entity SHALL carry `dc.terms.title` qualified by its parent document's title, and
SHALL carry `DocChunkIndex` with its ordinal. A passage under a heading SHALL additionally carry
`DocSection` with the heading text, and the docs lens SHALL expose a section anchor as the passage's
locator fragment.

#### Scenario: A passage under a heading is labelled

- **WHEN** a passage derived from a headed section is returned in a relations listing
- **THEN** its label identifies both the parent document and the section

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
