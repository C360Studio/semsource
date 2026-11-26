## MODIFIED Requirements

### Requirement: Fusion tools disclose where Tier 1 seeds came from

At Tier 1 the fusion tools (`code_search`, `code_context`, `doc_context`) SHALL seed natural-language
questions only through the substrate's merged search, with no SemSource-owned search path, and SHALL
return the fusion response's `provenance`. For a natural-language question they SHALL also return the
fusion response's `merge` unchanged (`honoured`, `detail`, and `reason` and `code` when not honoured), so
`provenance: lexical` beside `merge.honoured: true` is an applied merge with no embedding-list seed and beside
`merge.honoured: false` is a word-list fallback. A symbol or prefix question SHALL carry no `merge`. Their
descriptions SHALL state that `similarity` is the merged rank score (at most 2/61) only when the merge was
applied, and the word score otherwise, and SHALL name `merge.honoured`.

#### Scenario: Merged seeds at Tier 1

- **GIVEN** a Tier 1 composition whose embedding provider answers
- **WHEN** an agent asks `code_search` a natural-language question whose answer includes an embedding-list seed
- **THEN** the reply reports `provenance: embedding` and `merge.honoured: true`

#### Scenario: Provider fails during a fusion question

- **GIVEN** a Tier 1 composition whose provider does not answer
- **WHEN** an agent asks `code_search` a natural-language question with `include_scores`
- **THEN** the reply reports `provenance: lexical` and each `similarity` is the word score, not a merged rank score
- **AND** the reply's `merge` is not honoured and carries the provider failure's reason and code

#### Scenario: Tier 0 unchanged

- **GIVEN** the default Tier 0 composition
- **WHEN** an agent asks a fusion tool a natural-language question
- **THEN** the seeds are the word list and the reply reports `provenance: lexical` with no `merge`

### Requirement: Tier 1 acceptance needs a reachable, identified provider

Tier 1 qualification SHALL record the embedding provider's image digest, build SHA, served model id
and revision, model artifact hash, dimensions and the query instruction applied (by the provider via `input_type`, or a product prefix), and SHALL use one provider instance for
every compared run. A run whose provider was configured but unreachable SHALL be reported as
unavailable evidence, never as a pass. Readiness SHALL report the semantic index ready only while
graph-lexical and graph-embedding both report their index caught up, and SHALL then report the lower
of their indexed revisions, unknown when either is unknown. Readiness is index coverage,
not provider liveness: each answer reports liveness itself, through `merge.honoured` on
`graph_search` and on the fusion tools.

#### Scenario: Provider missing during a qualification run

- **GIVEN** the neural lane or the Tier 1 scorecard arm is configured with a provider that does not answer
- **WHEN** the run completes
- **THEN** it is recorded as unavailable, not graded

#### Scenario: Embeddings fail while the index is built

- **GIVEN** a Tier 1 composition whose provider does not answer while entities are embedded
- **WHEN** a client reads readiness
- **THEN** the semantic index is not ready, because graph-embedding's failed entities hold its index back

#### Scenario: Both halves ready at different revisions

- **GIVEN** a Tier 1 composition whose word index has indexed revision 100 and whose graph-embedding has indexed revision 90, both ready
- **WHEN** a client reads the semantic index on `/source-manifest/status`, the capabilities document or MCP `source_status`
- **THEN** it reports `indexed_revision: 90`, so a write after revision 90 is not read as covered

#### Scenario: Provider goes down after the index caught up

- **GIVEN** a Tier 1 composition whose index has caught up and whose provider then stops answering
- **WHEN** an agent calls `graph_search`
- **THEN** readiness may still report the semantic index ready, and the answer reports `merge.honoured: false` with the word list

#### Scenario: Graded against the floors

- **GIVEN** a reachable, identified provider
- **WHEN** the Tier 1 scorecard arm runs on the #230 corpora
- **THEN** its result is compared with the #230 Tier 1 floor and the alpha.6 Tier 0 run, band by band
