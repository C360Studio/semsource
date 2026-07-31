## MODIFIED Requirements

### Requirement: A query for a canonical value prefers the section that defines it

NL doc retrieval SHALL rank the passage that defines a configuration value above a passage that overrides, works around, or troubleshoots it. Answering "what is the default X" with the section about overriding X is a plausible-looking wrong answer, and the top-ranked evidence is what a caller cites.

This MUST be achieved through what SemSource emits — the passage text that becomes the embedded body — and never by classifying prose as canonical or non-canonical, nor by weighting predicate salience past the substrate's stated tie-breaking role.

#### Scenario: The default and its workaround live in different sections

- **WHEN** an NL docs query asks for the default value of a setting whose document also documents a
  workaround value elsewhere
- **THEN** the top-ranked passage carries the default
- **AND** it does not carry the workaround value

#### Scenario: A fact is not diluted by the unrelated facts beside it

- **WHEN** a fact sits inside a fenced block of independent peer entries — settings, commands, or
  flag declarations alike
- **THEN** it is retrievable on its own terms rather than competing as one item in a broader vector

#### Scenario: Ranking is not bought with prose classification

- **WHEN** the retrieval ordering is changed to satisfy this requirement
- **THEN** the mechanism is structural — derivable from the document bytes — and no component
  decides whether prose is canonical, recommended, or a workaround by reading it

## ADDED Requirements

### Requirement: A fact does not lose its passage as the prose around it grows

A passage carrying a specific, citable fact SHALL remain retrievable for that fact as unrelated prose accumulates in its section. Documentation growth is ordinary and continuous; a retrieval guarantee that holds only for a document's current wording is not a guarantee.

Where a passage's fact is diluted past the point of retrieval by surrounding prose, the remedy MUST be structural — dividing the passage so the fact stands on its own terms — and MUST NOT be an edit to the documentation that restores the score while leaving the mechanism in place.

#### Scenario: Prose accumulates around a fact in a fenced block

- **GIVEN** a fact inside a fenced block of independent peer entries, retrievable as the top-ranked
  passage for a query naming it
- **WHEN** unrelated prose is added to the same section, leaving it below the chunk size ceiling
- **THEN** the fact is still the top-ranked passage for that query

#### Scenario: A graded regression is not repaired by rewording the corpus

- **WHEN** a graded question regresses because its answer passage was diluted
- **THEN** the change that repairs it alters the splitter, not the document
- **AND** the repair is demonstrated on the unmodified document

### Requirement: A candidate change to emitted body text is scored offline before a live A/B

Because ranking for NL doc queries is essentially the embedding's own cosine order, a change to the body text SemSource emits SHALL be evaluated offline — splitting with the real splitter and embedding through the deployed embedder — against a named distractor, before it is promoted to a live A/B.

The offline instrument SHALL report the signed margin between the intended answer and its distractor, and SHALL be admissible only where it has reproduced a known live ordering, including the sign of that margin. An instrument that has not predicted a real outcome measures a proxy, and MUST NOT be reported as evidence that a candidate rule works.

The instrument SHALL live in the repository rather than in a session workspace, so a later change can re-run it against the same corpus.

#### Scenario: A candidate split rule is proposed

- **WHEN** a change alters how passages are divided
- **THEN** the offline harness reports the answer-versus-distractor margin before and after the rule
- **AND** a rule that does not move the margin in the intended direction does not proceed to a live
  A/B

#### Scenario: The instrument's own credibility is established

- **WHEN** the offline harness is used as evidence
- **THEN** it has reproduced at least one known live ordering, including the sign of the margin
- **AND** the corpus, embedder, and query prefix used are recorded with the result

#### Scenario: An offline prediction disagrees with the live result

- **WHEN** the harness predicts an ordering that the live stack contradicts
- **THEN** the live result stands and the harness is treated as incomplete
- **AND** the disagreement is recorded rather than resolved in the harness's favour
