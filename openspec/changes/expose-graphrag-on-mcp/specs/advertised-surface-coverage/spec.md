## ADDED Requirements

### Requirement: Graph-query MCP tools have tier-conditional test evidence

The advertised-surface evidence matrix SHALL name a test or smoke for every graph-query MCP tool,
and that evidence SHALL cover both sides of each tool's own gate: for a capability-gated tool, an
honest refusal without the capability and a non-empty community-backed result with it; for an
ungated tool, a successful answer without the capability carrying the disclosure that the answer is
not community-backed. Appearing in `tools/list` SHALL NOT count as coverage.

#### Scenario: The refusal path is proven for a gated tool

- **GIVEN** a stack running without clustering enabled
- **WHEN** the acceptance check calls the capability-gated tool
- **THEN** the call returns an error naming the missing capability, and the check fails if it
  returns an empty success instead

#### Scenario: Ungated tools are proven to still answer

- **GIVEN** a stack running without clustering enabled
- **WHEN** the acceptance check calls each ungated graph-query tool
- **THEN** each answers successfully and its result discloses that the answer is not
  community-backed, and the check fails if an ungated tool refuses

#### Scenario: The enabled path is proven non-empty

- **GIVEN** a stack with clustering enabled whose community index is populated over a known corpus
- **WHEN** the acceptance check calls each graph-query tool
- **THEN** each returns a non-empty result, and community-backed results carry community attribution

#### Scenario: Roster listing does not count as coverage

- **WHEN** a graph-query tool stops returning results but still appears in `tools/list`
- **THEN** the acceptance check fails

### Requirement: Graph-query evidence stays deterministic

Graph-query acceptance evidence SHALL assert contract-level invariants only: capability gating,
readiness semantics, envelope shape, community attribution, and non-emptiness over a known corpus.
It SHALL NOT grade the prose quality of a synthesized answer, and SHALL NOT introduce an LLM judge
into the graded scorecard or into any gate that shares the scorecard's pass/fail signal — a judge
drifts between runs, and a drifting judge cannot support an A/B.

#### Scenario: A judge does not join the deterministic gates

- **WHEN** a proposed GraphRAG check grades the prose of a synthesized answer
- **THEN** it is kept out of the deterministic gate set, and if retained at all it runs as a
  separate instrument reporting its own pass/fail

#### Scenario: Documentation stops claiming MCP never reaches GraphRAG

- **GIVEN** `configs/tiers/README.md` and `scripts/scorecard/README.md` state that the MCP tools
  resolve through the fusion gateway and never reach GraphRAG
- **WHEN** the GraphRAG tools ship
- **THEN** both documents describe the new capability-gated tool family and state why the graded
  scorecard still does not measure it
