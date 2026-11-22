## ADDED Requirements

### Requirement: Tier 1 is an explicit opt-in neural composition

SemSource SHALL compose graph-embedding and set graph-query `embedding_enabled` only when the
configuration sets `graph.semantic_search: true`. A configuration that sets it SHALL be refused at
validation unless `model_registry` resolves the `embedding` capability to an embedding endpoint and
`graph.lexical_search` is on. The default composition SHALL stay Tier 0, and enabling Tier 1 SHALL
advance the composition generation.

#### Scenario: Tier 1 configured with an embedder

- **GIVEN** `graph.semantic_search: true` and a registry whose `embedding` capability resolves to semembed
- **WHEN** SemSource composes the graph subsystem
- **THEN** graph-embedding is registered through `RegisterWithEmbedder` with an `httpembedder` built from that endpoint
- **AND** graph-query runs with `embedding_enabled: true`

#### Scenario: Tier 1 configured without an embedder

- **GIVEN** `graph.semantic_search: true` and no registry entry that resolves `embedding` to an embedder
- **WHEN** the configuration is validated
- **THEN** validation fails naming the missing embedding capability, and SemSource does not boot as Tier 0

#### Scenario: Default composition is unchanged

- **GIVEN** a configuration that does not set `graph.semantic_search`
- **WHEN** SemSource composes the graph subsystem
- **THEN** no graph-embedding is composed and the Tier 0 composition fingerprints are unchanged

### Requirement: graph_search discloses the merged answer at Tier 1

At Tier 1 the MCP `graph_search` tool SHALL report each match's merged reciprocal-rank score as
`relevance`, and its `word_rank` and `vector_rank` when present. It SHALL report whether the merge
was honoured. A reply whose merge was not honoured SHALL be disclosed as such, never presented as
merged. The tools-list SHALL stay within its byte budget.

#### Scenario: Provider answers

- **GIVEN** a Tier 1 composition whose embedding provider answers
- **WHEN** an agent calls `graph_search`
- **THEN** matches carry the merged relevance and their word and vector ranks, and the reply reports the merge honoured

#### Scenario: Provider fails mid-query

- **GIVEN** a Tier 1 composition whose embedding provider is unreachable
- **WHEN** an agent calls `graph_search`
- **THEN** the answer is the word list and the reply reports the merge not honoured

### Requirement: Fusion tools disclose where Tier 1 seeds came from

At Tier 1 the fusion tools (`code_search`, `code_context`, `doc_context`) SHALL seed natural-language
questions only through the substrate's merged search, with no SemSource-owned search path, and SHALL
return the fusion response's `provenance`. Their descriptions SHALL state that `similarity` is on the
merged scale at Tier 1.

#### Scenario: Merged seeds at Tier 1

- **GIVEN** a Tier 1 composition whose embedding provider answers
- **WHEN** an agent asks `code_search` a natural-language question whose answer includes an embedding-list seed
- **THEN** the reply reports `provenance: embedding`

#### Scenario: Tier 0 unchanged

- **GIVEN** the default Tier 0 composition
- **WHEN** an agent asks a fusion tool a natural-language question
- **THEN** the seeds are the word list and the reply reports `provenance: lexical`

### Requirement: Tier 1 acceptance needs a reachable, identified provider

Tier 1 qualification SHALL record the embedding provider's image digest, build SHA, served model id
and revision, model artifact hash, dimensions and the query instruction applied (by the provider via `input_type`, or a product prefix), and SHALL use one provider instance for
every compared run. A run whose provider was configured but unreachable SHALL be reported as
unavailable evidence, never as a pass. Readiness SHALL NOT report the semantic index ready while the
provider is unreachable.

#### Scenario: Provider missing during a qualification run

- **GIVEN** the neural lane or the Tier 1 scorecard arm is configured with a provider that does not answer
- **WHEN** the run completes
- **THEN** it is recorded as unavailable, not graded, and readiness never reported the semantic index ready

#### Scenario: Graded against the floors

- **GIVEN** a reachable, identified provider
- **WHEN** the Tier 1 scorecard arm runs on the #230 corpora
- **THEN** its result is compared with the #230 Tier 1 floor and the alpha.6 Tier 0 run, band by band
