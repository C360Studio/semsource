# Delta: fusion-gateway

## ADDED Requirements

### Requirement: Impact seeds resolve exactly

Impact (and context) symbol resolution SHALL be exact-match on symbol names; case-insensitive
lookalikes SHALL NOT enter the closure. When a name is genuinely ambiguous across packages, each
candidate SHALL be surfaced as an explicitly-labeled seed, never silently merged into one count.

#### Scenario: Case lookalikes excluded

- **WHEN** code_impact is asked about `SystemSlug` while unexported `systemSlug` helpers exist in
  other packages
- **THEN** the lookalikes contribute nothing to the closure and only the exact-match seed(s) are
  reported

### Requirement: Impact names its dependents

The impact response SHALL name at least the direct dependents of the seed (bounded and
truncation-labeled), in addition to counts, so "what depends on this" is answerable without a
follow-up query per node. If dependent hydration requires framework support, the framework ask is
recorded upstream and counts remain honest in the interim.

#### Scenario: Dependents listed

- **WHEN** code_impact returns a non-zero closure for a symbol
- **THEN** the response names the direct dependents (up to the documented bound, with truncation
  labeled)
