## ADDED Requirements

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
