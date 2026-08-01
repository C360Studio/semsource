## ADDED Requirements

### Requirement: Graph-query MCP tools have tier-conditional test evidence

The advertised-surface evidence matrix SHALL name a test or smoke for every graph-query MCP tool,
and that evidence SHALL prove the tool answers on a stack without clustering and discloses that its
answer is not community-backed. Evidence for the community-backed and LLM-synthesized rungs of the
capability ladder MAY be hermetic, driven by recorded substrate responses, rather than requiring a
live clustered stack. Appearing in `tools/list` SHALL NOT count as coverage.

#### Scenario: The default-tier path is proven

- **GIVEN** a stack running without clustering enabled
- **WHEN** the acceptance check calls each graph-query tool
- **THEN** each answers successfully and its result discloses that the answer is not
  community-backed, and the check fails if a tool refuses or omits the disclosure

#### Scenario: The escalated rungs are proven

- **GIVEN** substrate responses representing a community-backed answer and an LLM-synthesized answer
- **WHEN** the disclosure is derived from each
- **THEN** the community-backed rung and the model attribution are reported, and a template answer
  is never reported as LLM-synthesized

#### Scenario: Roster listing does not count as coverage

- **WHEN** a graph-query tool stops returning results but still appears in `tools/list`
- **THEN** the acceptance check fails

### Requirement: Graph-query evidence stays deterministic

Graph-query acceptance evidence SHALL assert contract-level invariants only: disclosure derivation,
envelope shape, community attribution, and non-emptiness over a known corpus. It SHALL NOT grade the
prose quality of a synthesized answer, and SHALL NOT introduce an LLM judge into the graded
scorecard or into any gate that shares the scorecard's pass/fail signal — a judge drifts between
runs, and a drifting judge cannot support an A/B.

#### Scenario: A judge does not join the deterministic gates

- **WHEN** a proposed check grades the prose of a synthesized answer
- **THEN** it is kept out of the deterministic gate set, and if retained at all it runs as a
  separate instrument reporting its own pass/fail

#### Scenario: Documentation stops claiming MCP never reaches GraphRAG

- **GIVEN** `configs/tiers/README.md` and `scripts/scorecard/README.md` state that the MCP tools
  resolve through the fusion gateway and never reach GraphRAG
- **WHEN** the graph-query tools ship
- **THEN** both documents describe the new tool family and the capability ladder, and state why the
  graded scorecard still does not measure it
