## ADDED Requirements

### Requirement: Doc evidence is passage-scoped

A `doc_context` answer SHALL return passage bodies as evidence rather than whole file bodies. A
returned evidence node MUST correspond to one passage of one document, and its body MUST be the
verbatim stored passage resolved through the passage's own body handle — the gateway MUST NOT slice,
trim, or window a document body itself, consistent with the substrate's pre-sliced-handle contract.

#### Scenario: A query matches part of a long document

- **WHEN** a `doc_context` query matches content inside a document larger than one passage
- **THEN** the evidence node is the matching passage, not the whole document
- **AND** its body is byte-identical to the stored passage body

#### Scenario: A whole document is not returned as one node

- **WHEN** a `doc_context` query resolves against a multi-passage document
- **THEN** no single evidence node carries the concatenated document body

#### Scenario: The gateway performs no body arithmetic

- **WHEN** any docs-lens answer is assembled
- **THEN** each node body is returned exactly as hydrated from its handle
- **AND** no line, offset, or character math is applied to it by SemSource

### Requirement: The docs lens declares passage containment edges

The docs lens SHALL declare the passage-containment predicate in its edge specification with both an
outgoing role resolving a passage to its parent document and an incoming role resolving a document
to its passages. The declaration MUST restrict the edge to the relations facet, so passage
containment does not participate in impact or paths walks.

#### Scenario: Passage expands to its document

- **WHEN** a docs-lens relations query is seeded on a passage entity
- **THEN** the parent document is present under the declared outgoing role

#### Scenario: Document expands to its passages

- **WHEN** a docs-lens relations query is seeded on a parent document entity
- **THEN** its passage entities are present under the declared incoming role

#### Scenario: Containment stays out of the impact walk

- **WHEN** an impact or paths facet is computed for a doc-domain seed
- **THEN** the passage-containment predicate contributes no edges to that walk

#### Scenario: Passage neighbours render with usable references

- **WHEN** a passage appears as a neighbour in a relations listing
- **THEN** its reference carries a name identifying its document and section, and a path
