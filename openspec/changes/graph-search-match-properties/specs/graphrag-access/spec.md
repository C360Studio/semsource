## ADDED Requirements

### Requirement: Matches carry bounded value properties when the substrate supplies triples

A `graph_search` match SHALL include a bounded `properties` map of the entity's own value-bearing facts — populated exclusively from an explicit allowlist of property predicates — whenever the substrate response already carries that entity's triples. Property values SHALL be the substrate's own triple objects, never derived or invented. The map SHALL be capped in entry count per match and in bytes per value, and a response path that carries no triples (digests, bare entity IDs) SHALL render matches without properties rather than fetching them — no additional substrate round-trips are permitted on behalf of match rendering.

#### Scenario: Config dependency values are answerable in one call

- **WHEN** `graph_search` matches a config dependency entity on a response path that carries the entity's triples
- **THEN** the match's `properties` include the allowlisted config values present on the entity (such as dependency version and kind), so a value question is answerable without a follow-up call

#### Scenario: Absence stays absence on triple-less paths

- **WHEN** the substrate response carries digests or bare entity IDs instead of entity triples
- **THEN** matches render without a `properties` map, and no substrate query is issued to fill it

#### Scenario: Rendering stays bounded

- **WHEN** a matched entity carries more allowlisted properties than the per-match cap, or a property value longer than the per-value byte cap
- **THEN** the rendered map is truncated to the caps deterministically, and the match otherwise renders normally

#### Scenario: Non-allowlisted predicates never render

- **WHEN** a matched entity carries triples whose predicates are not on the allowlist (relationship edges, bodies, timestamps)
- **THEN** none of them appear in `properties`, regardless of remaining cap headroom
