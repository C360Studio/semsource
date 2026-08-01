## ADDED Requirements

### Requirement: Request-subject ownership has an automated guard

An automated gate SHALL assert that no subject a SemSource component subscribes to for
request/reply also appears in SemSource's declaration of the substrate query surface — the
graph-query input-port list it composes at startup. Both sides are maintained in-repo, so the gate
is exact for what SemSource declares.

The gate SHALL state its own limit: it compares against SemSource's declaration of the substrate's
surface, not against the substrate's authoritative handler registrations, which the pinned target
keeps unexported. A substrate release that adds a subject SemSource already claims is therefore
caught only once that declaration is updated. That residual gap SHALL be recorded as an upstream ask
rather than papered over.

This is the request/reply analogue of the existing GRAPH-stream subject-overlap pin, and it exists
because the collision it guards against was invisible to the whole test suite: the only test that
exercised the contested subject started the substrate component alone, so no handler ever competed.

#### Scenario: A newly contested subject fails CI

- **WHEN** a SemSource component subscribes to a request subject that also appears in the declared
  substrate query surface
- **THEN** the ownership gate fails, naming the subject and both claimants

#### Scenario: The declared substrate surface stays complete

- **WHEN** the declared graph-query input-port list omits a subject the pinned substrate serves
- **THEN** the omission is a known limit of the gate, recorded as an upstream ask, and the list is
  corrected when discovered

### Requirement: The restored graph-overview tool has coverage

The advertised-surface evidence matrix SHALL name a test for the `graph_summary` MCP tool, and that
evidence SHALL assert the substrate's payload shape — not merely a non-error result — so that a
regression which reintroduces a competing handler is caught by a wrong-shape failure rather than
passing silently.

#### Scenario: The wrong handler answering fails the test

- **WHEN** a handler other than the substrate's answers the tool's routed subject
- **THEN** the tool's coverage fails on the payload shape
