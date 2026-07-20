# retrieval-ranking Specification

## Purpose
NL retrieval ranks what a developer means first: production code above its tests
(the code.artifact.test presence marker at -2.0 salience, the demotion
complement of the exported boost) and canonical documentation above planning
artifacts (openspec/** and node_modules are excluded from the default docs
corpus). All via governed salience and corpus scoping, no bespoke ranker
(audit 2026-07-19).

## Requirements

### Requirement: Production code outranks its tests

NL code retrieval SHALL rank production symbols above test and generated symbols for equivalent
relevance, via governed salience on a demotion marker (not result filtering), so a paraphrase
query for a behavior surfaces its implementation before its tests.

#### Scenario: Sanitizer paraphrase query

- **WHEN** code_search is asked "turn an arbitrary string into a safe entity id segment"
- **THEN** `SanitizeInstance` (the implementation) appears in the results ranked above any of its
  `_test.go` tests

#### Scenario: Tests remain reachable

- **WHEN** a query names a test explicitly or structural tools resolve a test symbol
- **THEN** test entities are still returned (demoted, not hidden)

### Requirement: Canonical docs outrank archived planning artifacts

Doc retrieval SHALL answer product questions from canonical documentation (README, docs/**) rather
than archived planning artifacts; archived OpenSpec changes are excluded from the default docs
corpus or demoted below canonical docs.

#### Scenario: Language-support question

- **WHEN** doc_context is asked which languages the ast source supports
- **THEN** the answer's cited passages come from canonical docs (README/ROADMAP), not from
  `openspec/changes/archive/**`

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
asserted. The passage corpus MUST NOT regress previously correct answers, and residuals
attributed to whole-file embedding dilution MUST be re-graded against the passage corpus.

#### Scenario: Graded interrogation is re-run

- **WHEN** the graded interrogation set is run against a stack ingesting passages
- **THEN** the dilution-attributed residual questions are re-graded
- **AND** no previously correct answer becomes incorrect

#### Scenario: Corpus growth is measured, not assumed

- **WHEN** a repository is ingested with passage chunking enabled
- **THEN** entity count and time-to-ready are recorded against the whole-file baseline for the same
  repository
