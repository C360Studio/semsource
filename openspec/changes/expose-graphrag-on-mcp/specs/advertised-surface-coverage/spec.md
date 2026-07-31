## ADDED Requirements

### Requirement: GraphRAG MCP tools have tier-conditional test evidence

The advertised-surface evidence matrix SHALL name a test or smoke for every GraphRAG MCP tool, and
that evidence SHALL cover both sides of the capability gate: an honest refusal on a stack without
clustering, and a non-empty community-backed result on a stack whose community index is populated
over a known corpus. Appearing in `tools/list` SHALL NOT count as coverage.

#### Scenario: The refusal path is proven

- **GIVEN** a stack running without clustering enabled
- **WHEN** the acceptance check calls each GraphRAG tool
- **THEN** every call returns an error naming the missing capability, and the check fails if any
  call returns an empty success instead

#### Scenario: The enabled path is proven non-empty

- **GIVEN** a stack with clustering enabled whose community index is populated over a known corpus
- **WHEN** the acceptance check calls each GraphRAG tool
- **THEN** each returns a non-empty, community-backed result carrying community attribution

#### Scenario: Roster listing does not count as coverage

- **WHEN** a GraphRAG tool stops returning results but still appears in `tools/list`
- **THEN** the acceptance check fails

### Requirement: GraphRAG evidence stays deterministic

GraphRAG acceptance evidence SHALL assert contract-level invariants only: capability gating,
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
