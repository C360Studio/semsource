# Delta: retrieval-ranking

## ADDED Requirements

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
