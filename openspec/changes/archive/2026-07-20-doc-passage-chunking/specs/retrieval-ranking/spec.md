## ADDED Requirements

### Requirement: A body-less parent document does not become lexical noise

A parent document entity carrying a title but no body SHALL NOT displace passage entities that
carry actual content in NL retrieval results. Titled, empty-bodied nodes MUST NOT be returned as
substantive evidence when a passage of the same document answers the query.

#### Scenario: A query matches a document's title and its passage content

- **WHEN** an NL docs query matches both a parent document's title and one of its passages
- **THEN** the passage carrying the content ranks above the body-less parent

#### Scenario: A body-less node is not offered as evidence

- **WHEN** a parent document entity is returned in a docs answer
- **THEN** it is not presented as a passage of evidence with an empty or absent body

### Requirement: A result set does not return both a document and its own passages as separate evidence

Retrieval results SHALL NOT present a parent document entity and passage entities of that same
document as independent evidence nodes for one query. The passage is the unit of evidence; the
parent is navigational context.

#### Scenario: Several passages of one document match

- **WHEN** multiple passages of the same document match one query
- **THEN** the passages are returned as evidence
- **AND** their parent document is not additionally returned as a competing evidence node

#### Scenario: Passage results remain attributable

- **WHEN** passages of a document are returned as evidence
- **THEN** each identifies the document it came from

### Requirement: Passage-level retrieval measurably improves answer precision

Passage-scoped retrieval SHALL be verified against the graded interrogation set rather than
asserted. The passage corpus MUST NOT regress previously correct answers, and the residuals
attributed to whole-file embedding dilution MUST be re-graded after the change.

#### Scenario: Graded interrogation is re-run

- **WHEN** the graded interrogation set is run against a stack ingesting passages
- **THEN** the dilution-attributed residual questions are re-graded
- **AND** no previously correct answer becomes incorrect

#### Scenario: Corpus growth is measured, not assumed

- **WHEN** a repository is ingested with passage chunking enabled
- **THEN** entity count and time-to-ready are recorded against the pre-change baseline for the same
  repository
