# retrieval-ranking Specification

## Purpose
NL retrieval ranks what a developer means first: production code above its tests
(the code.artifact.test presence marker at -2.0 salience, the demotion
complement of the exported boost) and canonical documentation above planning
artifacts (openspec/** and node_modules are excluded from the default docs
corpus), and a canonical value above the section that overrides it. All via
governed salience, corpus scoping, and the passage text SemSource emits — no
bespoke ranker (audit 2026-07-19).

Salience is a tie-breaker, not a ranking lever: the substrate weights it
deliberately small against resolve order, so for natural-language doc queries
ranking is essentially the embedding's own cosine order. What SemSource controls
is therefore the body text that becomes that embedding (2026-07-20).

Retrieval claims here are measured against the graded interrogation set, so this
capability also governs what makes a graded verdict trustworthy: it must reflect
retrieval rather than grader error, and must not depend on a question's position
in the run.

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

### Requirement: A node with no retrievable body is never offered as evidence

Retrieval SHALL NOT present any node with an empty or absent body as substantive evidence, whatever its entity class. The existing parent-document guard covers one such class; the obligation is general, because the property that makes a node useless as a citation is the missing body, not the kind of thing it is.

#### Scenario: A body-less config dependency entity competes on a docs query

- **WHEN** an NL docs query is scoped over a corpus that includes config dependency entities
  carrying identity but no body
- **THEN** no such entity is returned as the top-ranked evidence node
- **AND** a passage carrying content ranks above it

#### Scenario: The guard does not empty a result set

- **WHEN** every candidate for a query lacks a retrievable body
- **THEN** the answer is honest about having no evidence rather than silently returning nothing
  distinguishable from a miss

### Requirement: A graded verdict reflects retrieval and never grader error

The graded interrogation set SHALL produce a verdict only from evidence the matcher actually evaluated. A matcher that fails to execute against its own literal MUST NOT be recorded as a retrieval failure, and a question whose literals the grader cannot evaluate MUST fail validation before it is ever scored.

#### Scenario: An expected value begins with a hyphen

- **WHEN** a question's expected or forbidden literal begins with a character the matcher could
  interpret as an option, such as `-p 8083:8083`
- **THEN** the matcher evaluates it as content
- **AND** the resulting verdict reflects whether retrieval returned it

#### Scenario: Question validation gates grader evaluability

- **WHEN** the question set is validated
- **THEN** every literal is checked by feeding it through the same matcher the grader uses against
  a string known to contain it and a string known not to
- **AND** a literal that cannot distinguish those two cases fails validation

### Requirement: A graded verdict does not depend on a question's position in the run

A question's verdict SHALL be a property of retrieval, not of what was asked before it. Where the platform does not yet make that true, the instrument MUST detect and report the instability rather than concealing it behind a single sampled call.

#### Scenario: The same question is asked at different positions

- **WHEN** one question is scored first in a run and again last in the same run against an
  unchanged stack and corpus
- **THEN** both runs record the same verdict
- **OR** the disagreement is itself reported as a distinct outcome, never resolved silently to
  either the passing or the failing result

#### Scenario: Instability is visible in the summary

- **WHEN** repeated calls for one question disagree
- **THEN** the run reports that question as unstable, with each distinct outcome retained
- **AND** the unstable count is surfaced alongside the score rather than folded into it

### Requirement: Evidence arguing for the wrong answer is graded distinctly from absent evidence

The graded set SHALL distinguish top-ranked evidence that carries a confusable value but not the answer from evidence that simply misses. A result that argues for the wrong answer is a different and more expensive failure than one that returns nothing, and conflating them hides the failure that misleads a caller.

#### Scenario: The top node carries only the confusable twin

- **WHEN** the top-ranked node contains a question's forbidden confusable value and does not
  contain its expected value
- **THEN** the verdict names that state distinctly from a plain miss

#### Scenario: The distinction survives in the summary

- **WHEN** a run contains such a result
- **THEN** it is reported separately from misses, and separately from fabrication, so a reader can
  tell "found nothing" from "found the wrong thing" from "asserted something false"

### Requirement: A query for a canonical value prefers the section that defines it

NL doc retrieval SHALL rank the passage that defines a configuration value above a passage that overrides, works around, or troubleshoots it. Answering "what is the default X" with the section about overriding X is a plausible-looking wrong answer, and the top-ranked evidence is what a caller cites.

This MUST be achieved through what SemSource emits — the passage text that becomes the embedded body — and never by classifying prose as canonical or non-canonical, nor by weighting predicate salience past the substrate's stated tie-breaking role.

#### Scenario: The default and its workaround live in different sections

- **WHEN** an NL docs query asks for the default value of a setting whose document also documents a
  workaround value elsewhere
- **THEN** the top-ranked passage carries the default
- **AND** it does not carry the workaround value

#### Scenario: A fact is not diluted by the unrelated facts beside it

- **WHEN** a fact sits inside a homogeneous list of independent settings
- **THEN** it is retrievable on its own terms rather than competing as one item in a broader vector

#### Scenario: Ranking is not bought with prose classification

- **WHEN** the retrieval ordering is changed to satisfy this requirement
- **THEN** the mechanism is structural — derivable from the document bytes — and no component
  decides whether prose is canonical, recommended, or a workaround by reading it
