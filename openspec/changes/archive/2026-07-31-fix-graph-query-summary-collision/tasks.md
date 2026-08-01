# Tasks

## 1. Vacate the contested subject

- [x] 1.1 Change `summaryQuerySubject` in `processor/source-manifest/component.go` from
      `graph.query.summary` to `graph.query.sourceSummary`. The handler body and `SummaryPayload` are
      unchanged; only the subject moves.
- [x] 1.2 Update the log line and the `payload_summary.go` doc comment, which both name the old
      subject.
- [x] 1.3 Confirm `GET /source-manifest/summary` is untouched and its existing tests still pass —
      that route is the primary consumer path and must not move.

## 2. Guard the boundary

- [x] 2.1 Add a test asserting that no subject any SemSource component subscribes to for
      request/reply appears in `graphQueryInputPorts()`. Name both claimants in the failure message.
- [x] 2.2 **Guard verified failing before the fix.** Written against the unfixed tree it reported:
      `contested request subject: graph.query.summary (claimed by semsource source-manifest and
      served by the substrate's graph-query)`. It passes only after the subject moved.
- [x] 2.3 Comment the guard with its limit: it compares against SemSource's *declaration* of the
      substrate surface, not the substrate's unexported handler registrations.
- [x] 2.4 Add `graph.query.byName` to `graphQueryInputPorts()` — the substrate serves it and the
      declaration omits it, which narrows the guard for no reason. Update the `run_test.go` pin.

## 3. Restore the graph_summary MCP tool — RESTORED, THEN CUT

- [x] 3.1 Re-add `GraphSummaryInput` and the `graphSummary` handler routing to `graph.query.summary`,
      returning the substrate payload verbatim with **no** disclosure block (design — D4).
- [x] 3.2 Register it in `buildServer()` and add it to `wantToolNames`, the compose smoke's
      `tools/list` loop, and `docs/integration/mcp-quickstart.md` — the quickstart table is pinned
      against the live roster, so it fails if missed.
- [x] 3.3 Integration test asserting the **substrate's shape** (`entity_types` / predicate summary),
      not merely a non-error result. This is what makes a reintroduced collision fail: the competing
      payload decodes to an empty `SummaryData`.
- [x] 3.4 Extend the tier-0 compose smoke with a real `tools/call` asserting that shape on a live
      stack — the collision was only ever visible with both components running.
- [x] 3.5 **Tool subsequently CUT before merge.** Restoring it was correct given the subject was
      uncontested, but a roster review found it would pollute agent context: `graph_summary` emits
      entity types and predicate names, and **no tool on SemSource's MCP roster accepts either**.
      Upstream's `summarize_graph` works because its roster has `query_by_type`; ours has no
      equivalent, so the overview would hand an agent a vocabulary it cannot spend and invite
      graph-shaped phrasing at tools that want natural language. The orientation need is already met
      by `source_status`. The collision fix stands on its own — it was a live defect, and
      `graph_summary` is what found it. `graph.query.summary` remains uncontested, pinned statically
      by the ownership guard and at runtime by the governance integration test.

## 4. Correct the record

- [x] 4.1 `docs/adr/0003-programmatic-source-add-api.md:44` — note that source-manifest does **not**
      own `graph.query.summary`; that claim was wrong and produced a subject collision. Correct in
      place with a dated note; do not rewrite the decision.
- [x] 4.2 `docs/integration/m5-consumer-integration.md` — add `graph.query.sourceSummary` to the
      source-manifest row, and add an explicit behavior-change notice: `graph.query.summary` now
      deterministically returns the substrate's `SummaryData`, and a consumer that had been receiving
      SemSource's `SummaryPayload` was receiving it by race, not by contract.
- [x] 4.3 Note in the same guide that `GET /source-manifest/summary` is the unchanged path for
      consumers who want SemSource's summary over HTTP.

## 5. Audit the remaining namespace claims

- [x] 5.1 Record, in the design or the consumer guide, that `graph.query.sources`,
      `graph.query.predicates`, and `graph.query.status` are SemSource concepts inside the
      substrate's namespace, that none is served by the substrate at the pinned target, and that the
      new guard watches them.
- [x] 5.2 Do **not** move them. They are working documented contracts; the audit records exposure,
      not a migration.

## 6. File the upstream ask

- [x] 6.1 Ask semstreams to export the graph-query request-subject list (or expose it via a
      component method), so the ownership guard can compare against the authority rather than a
      maintained in-repo copy. Record in `docs/upstream/semstreams-asks.md` and open an issue.
      Same shape as #819: the value exists inside the package and is never exposed.

## 7. Gates

- [x] 7.1 `gofmt`, `go vet`, `revive` (warnings fail, pinned v1.15.0), `go test ./...`, and
      `go test -tags=integration ./...` green. The governance integration test at
      `internal/governance/live_graph_integration_test.go:763` must still pass — it already expects
      the substrate shape, so it should be unaffected.
- [x] 7.2 `openspec validate fix-graph-query-summary-collision --strict` green.
- [x] 7.3 `./scripts/core-profile-smoke.sh` exits 0. `graph_summary` now returns the substrate shape
      (`entity_types` present, `entity_id_format` absent) where the same smoke previously returned
      source-manifest's payload — direct before/after evidence in one environment. The smoke also
      asserts `GET /source-manifest/summary` still serves the SemSource summary; the NATS subject
      `graph.query.sourceSummary` is pinned by a live round-trip in
      `TestIntegration_QuerySubjects`.
