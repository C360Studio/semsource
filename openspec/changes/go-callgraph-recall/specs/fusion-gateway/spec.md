# Delta: fusion-gateway

## ADDED Requirements

### Requirement: Impact seeds resolve exactly

Symbol-mode resolution SHALL be byte-exact on the display name for the code lens's answer
verbs (context, callers, callees, impact); case-insensitive lookalikes SHALL NOT enter the seed
set or the closure, and zero exact matches SHALL yield the engine's ready+absent miss (with
near-name suggestions), never a lookalike answer. Search keeps case-folded recall — discovery
is where case-insensitivity is a feature. When a name is genuinely ambiguous across packages,
each exact-match candidate SHALL be surfaced as its own explicitly-located seed node, never
silently collapsed into one.

#### Scenario: Case lookalikes excluded

- **WHEN** code_impact is asked about `SystemSlug` while unexported `systemSlug` helpers exist in
  other packages
- **THEN** the lookalikes contribute nothing to the closure and only the exact-match seed(s) are
  reported

#### Scenario: Search recall unchanged

- **WHEN** code_search is asked the same query
- **THEN** case-folded recall still surfaces the lookalikes as candidates

### Requirement: Impact names its dependents

The impact response SHALL name at least the direct dependents of each resolved seed — via the
relations facet's reverse roles (caller, extended_by, implemented_by, referenced_by,
embedded_by), bounded per role by the engine's documented cap — in addition to closure counts,
so "what depends on this" is answerable without a follow-up query per node. Closure-level
truncation SHALL remain labeled by `impact.truncated`; per-role truncation markers are a
recorded framework ask, and counts remain honest in the interim.

#### Scenario: Dependents listed

- **WHEN** code_impact returns a non-zero closure for a symbol
- **THEN** the response names the direct dependents in the seed node's reverse-role relations
  (up to the documented per-role bound), and the closure counts carry the truncation label
