## ADDED Requirements

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
