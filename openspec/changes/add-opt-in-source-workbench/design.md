## Context

SemSource currently has an optional `ui` Compose profile that targets a sibling SemTeams UI checkout.
The archived `add-ui-profile` change intentionally treated that profile as integration plumbing and
rejected a SemSource-focused frontend. That decision was appropriate for proving SemTeams as a
consumer, but it leaves standalone SemSource users without a product surface for source readiness,
search, evidence, project views, and interoperability actions.

The C360 repositories also contain several copies of the Svelte Sigma/Graphology graph explorer. The
shared work is substantial, but `semstreams-ui` is currently a private application rather than a
published component library, and downstream copies contain improvements that are not present in its
version. The workbench must consolidate that work rather than create another copy.

This change records a product-boundary pivot: SemSource owns an optional standalone workbench
experience, while remaining headless by default. It supersedes both archived `add-ui-profile`
decisions without rewriting that history: SemTeams no longer owns the app launched by SemSource, and
`../semteams/ui` is no longer the default UI checkout.

## Goals / Non-Goals

**Goals:**

- Keep headless and embedded SemSource complete and UI-free by default.
- Repurpose the explicit standalone `ui` profile to use a pinned released SemSource UI artifact.
- Reuse `semstreams-ui` as the canonical shared shell and graph-visualization owner.
- Keep source, evidence, materialized-view, and OKF semantics in SemSource backend contracts.
- Make the graph a drill-down from useful project views rather than the whole product experience.
- Establish correctness and accessibility gates for the canonical graph surface.

**Non-Goals:**

- Changing SemTeams code or deciding how SemTeams packages its own UI.
- Implementing materialized views, OKF interoperability, or one-action installation here.
- Requiring an npm component package before the first released workbench profile.
- Moving source interpretation or graph authority into Svelte.
- Eliminating every downstream UI copy as part of the SemSource implementation.
- Adding production authentication, TLS, or arbitrary repository writes.

## Decisions

### D1 - Headless remains the default and complete product contract

`docker compose up`, direct binary execution, MCP clients, and embedded sem* consumers SHALL NOT
resolve, pull, build, or start a UI artifact. Workbench activation is explicit through the existing
optional `ui` profile. All meaningful workbench actions remain backed by HTTP, MCP, GraphQL, or CLI
contracts.

This preserves SemSource's role as an optional service inside SemTeams, SemSpec, SemDragon, SemOps,
and other products. A browser improves standalone usability; it is not required for correctness.

### D2 - SemSource takes ownership of the existing `ui` profile

The deployment shapes are:

```text
docker compose up                  headless SemSource
docker compose --profile ui up     optional SemSource workbench
```

The existing `ui` profile becomes the SemSource workbench; a second `workbench` profile is not
introduced. This is an intentional breaking change for users who used SemSource's profile to launch
SemTeams. SemTeams now owns that application packaging and can connect to SemSource through the
headless contracts. Embedded consumers that never selected the profile are unaffected.

### D3 - SemSource owns semantics; `semstreams-ui` owns reusable presentation

SemSource owns project/source identity, readiness meaning, source inventory, provenance, authority,
freshness, materialized-view definitions, OKF behavior, backend APIs, and workbench acceptance tests.

`semstreams-ui` owns the reusable Svelte shell, graph renderer, layouts, generic search/filter/detail
components, accessible graph navigation, product-profile seam, and released UI artifact. A
SemSource-specific product profile can live in that UI repository while its product contract remains
defined and tested from SemSource.

SemSource SHALL NOT copy the renderer into this repository. The first delivery may consume a pinned
application image; publishing a standalone component package is optional follow-up work.

### D4 - Canonicalize the best graph implementation before workbench dependency

The shared UI team must compare the live graph implementations across `semstreams-ui`, SemTeams,
SemSpec, SemDragon, and SemConnect. The inventory records the strongest implementation and tests for:

- directed and multi-edge graph modeling;
- renderer lifecycle and attribute-aware updates;
- layout start, stop, refresh, and failure behavior;
- filters, selection, detail, search, and community navigation;
- truthful confidence, provenance, authority, and timestamp presentation;
- loading, empty, disconnected, and query-not-ready states;
- keyboard and screen-reader alternatives to WebGL interaction;
- responsive layout, test seams, performance, and large-project evidence; and
- packaging, tokens, and product-profile boundaries.

The initial live-repo audit selects this composite rather than treating any current copy as complete:

| Concern | Canonical source behavior |
| --- | --- |
| Renderer structure | SemSpec: lazy initialization, `MultiDirectedGraph`, failure state, layout refresh |
| Selection | SemDragon: explicit selected-node emphasis and selected/neighbor z-ordering |
| Search focus | SemConnect: focused result sets rather than only selected-node adjacency |
| Initial layout | SemConnect: deterministic ID-seeded positions, without its quadratic degree calculation |
| Force layout | `semstreams-ui`: worker-based bounded ForceAtlas2 controller |
| Panel shell | SemSpec: persisted panels, keyboard resize, safe shortcuts, and reduced-motion handling |
| Responsive access | SemConnect: stack/drawer behavior that keeps filters and evidence reachable |
| Filters | Composite: SemStreams dimensions/previews, SemDragon counts, SemConnect state, SemSpec totals |
| Entity/evidence detail | Composite: SemSpec identity/copy, SemConnect source-per-fact, shared context action |
| Source overview | SemDragon source/readiness cards plus the compact `semstreams-ui` graph overview |
| Search lifecycle | `semstreams-ui`: browse/replace/merge, cancellation, duration, errors, and result list |
| Partial failure | SemConnect: settled loading, cancellation, partial status, live/hybrid/error states |
| Test foundation | `semstreams-ui`: unit/attack coverage and the live SemSource Playwright stack |

SemTeams contributes the integration consumer and compatibility surface but no distinct newer graph
behavior in the audited copy. `semstreams-ui` remains the canonical destination and test harness;
SemSpec supplies the renderer/layout foundation, with selected interaction and resilience patterns
ported from SemDragon and SemConnect.

The canonical search behavior explicitly does not adopt SemSpec's heuristic that treats multiword text
as natural-language intent. Search classification remains a backend contract with visible evidence.

Two gates are non-negotiable. Missing evidence values remain unknown rather than being manufactured,
and stable entity/relationship IDs must not suppress attribute-only updates. The current shared code
does not yet meet either gate consistently. The canonical graph model must also preserve direction and
parallel predicates explicitly; it must not infer edges by testing whether a triple object merely looks
like a six-part entity ID.

### D5 - Released UI artifacts replace sibling-checkout requirements

The `ui` profile pulls a pinned `semstreams-ui` release image or immutable artifact.
End users do not need a sibling repository checkout, Node installation, or local UI build. A
SemSource-specific source override such as `SEMSOURCE_UI_CONTEXT` may remain available only for
development and compatibility tests. The former `UI_CONTEXT` meaning is not a stable production
contract.

The SemSource release pins the compatible artifact version or digest and runs a real SemSource
Playwright smoke against that pin.

### D6 - Workbench composition is capability-driven

A SemSource-owned browser capability contract tells the shared UI which product identity, readiness,
query surfaces, materialized views, and actions are available. Unsupported capabilities are absent or
explicitly unavailable; the UI does not probe SemTeams-only or other product routes and interpret
expected failures as degraded SemSource.

This change may start with existing status, source-manifest, search, and graph contracts. Later
materialized-view and OKF changes extend the capability contract without moving their semantics into
the UI.

### D7 - Project knowledge leads; the graph is drill-down

The workbench landing experience prioritizes project/source status, readiness and freshness, search,
source inventory, and opinionated materialized views when available. The graph explorer supports
investigation and evidence traversal, but a whole-graph canvas is not the default explanation of a
project.

The WebGL graph must have a synchronized, accessible entity/result navigator. Automated acceptance
tests must exercise user-visible list/detail behavior rather than depending only on browser test hooks
that bypass the canvas.

### D8 - Product actions execute through one backend contract

Future OKF preview/export, import validation, materialized-view refresh, and source actions call the
same SemSource backend contracts used by CLI and MCP automation. The browser does not implement a
second codec, planner, evidence model, or authority policy.

For the initial OKF slice, a browser download is preferred over granting server-side write authority.
Managed repository publication remains an explicit later mode.

### D9 - Explicit graph projection stays in the governed query boundary

The workbench needs explicit nodes, directed relationships, property facts, evidence metadata, and a
query/view revision. Before UI implementation, the architect must audit the live SemStreams graph
query/gateway contract to determine whether that payload already exists. If it does not, SemSource
records the framework gap in `docs/upstream/semstreams-asks.md` and raises it with the SemStreams team.

The beta.145 audit found that lens-driven fusion v1 already supports the primary workbench
search/list/detail/body/relations/impact views through SemSource HTTP, but it is not a lossless graph
projection: relation refs omit target handles, predicates, direction, evidence, and edge identity;
property facts are absent; relationship truncation is silent; and index readiness is not a coherent
view revision. The framework gap is recorded as upstream ask 18 and
[semstreams#533](https://github.com/C360Studio/semstreams/issues/533). Fusion-backed non-graph views
may proceed; the canonical graph drill-down remains gated on adoption and live validation of that
additive governed projection contract.

SemSource SHALL NOT infer relationships from literal string shape, manufacture evidence, or implement
a parallel graph substrate to satisfy the UI. The pinned workbench release is blocked until a real
backend payload and live compatibility test prove the required semantics.

### D10 - Bootstrap the browser with one product-owned HTTP capability document

The workbench bootstraps from `GET /source-manifest/capabilities`. The source-manifest component owns
this document because it is always present in the headless product, already owns authoritative source
status and inventory, has access to the graph readiness responders, and is mounted through the
ServiceManager shared HTTP mux. The endpoint is available without the optional UI profile.

This is a discovery/control document, not a graph-data query. GraphQL remains the browser's
schema-driven data-exploration surface, MCP remains the agent tool surface, and NATS remains the
internal readiness/query transport. Using GraphQL for the bootstrap document would make discovery
depend on a graph gateway whose own availability is being advertised; using MCP would couple a
browser to agent session semantics; exposing NATS would cross the external-client boundary.

The version-1 response has this stable shape:

```json
{
  "contract_version": 1,
  "product": {"key": "semsource", "name": "SemSource"},
  "project": {"key": "acme", "identity_kind": "deployment_namespace"},
  "readiness": {
    "overall": "ready",
    "source": {
      "available": true,
      "ready": true,
      "state": "ready",
      "source_count": 2,
      "total_entities": 418,
      "timestamp": "2026-07-15T15:00:00Z"
    },
    "structural_index": {
      "available": true,
      "ready": true,
      "state": "ready",
      "indexed_revision": 42,
      "target_revision": 42,
      "lag": 0
    },
    "semantic_index": {
      "available": true,
      "ready": true,
      "state": "ready",
      "indexed_revision": 42,
      "target_revision": 42,
      "lag": 0
    }
  },
  "queries": {
    "source_inventory": {
      "availability": "ready",
      "method": "GET",
      "href": "/source-manifest/sources",
      "readiness": ["source"]
    },
    "source_status": {
      "availability": "ready",
      "method": "GET",
      "href": "/source-manifest/status",
      "readiness": ["source"]
    },
    "project_summary": {
      "availability": "ready",
      "method": "GET",
      "href": "/source-manifest/summary",
      "readiness": ["source"]
    },
    "predicate_schema": {
      "availability": "ready",
      "method": "GET",
      "href": "/source-manifest/predicates",
      "readiness": ["source"]
    },
    "code_context": {
      "availability": "ready",
      "method": "POST",
      "href": "/code-context/context",
      "readiness": ["structural_index"]
    },
    "code_impact": {
      "availability": "ready",
      "method": "POST",
      "href": "/code-context/impact",
      "readiness": ["structural_index"]
    },
    "code_search": {
      "availability": "ready",
      "method": "POST",
      "href": "/code-context/search",
      "readiness": ["semantic_index"]
    },
    "doc_context": {
      "availability": "ready",
      "method": "POST",
      "href": "/doc-context/context",
      "readiness": ["structural_index"]
    },
    "graph_projection": {
      "availability": "unsupported",
      "reason": {
        "code": "upstream_contract_pending",
        "message": "The governed fusion graph projection is not available",
        "retryable": false
      }
    }
  },
  "actions": {
    "source_add": {
      "availability": "ready",
      "method": "POST",
      "href": "/source-manifest/sources"
    },
    "source_remove": {
      "availability": "ready",
      "method": "DELETE",
      "href": "/source-manifest/sources/{id}"
    },
    "okf_import": {
      "availability": "unsupported",
      "reason": {
        "code": "not_implemented",
        "message": "OKF import is not available",
        "retryable": false
      }
    },
    "okf_export": {
      "availability": "unsupported",
      "reason": {
        "code": "not_implemented",
        "message": "OKF export is not available",
        "retryable": false
      }
    }
  },
  "project_views": {
    "availability": "unsupported",
    "reason": {
      "code": "not_implemented",
      "message": "Project views are not available",
      "retryable": false
    }
  },
  "contracts": {"fusion_http_error": "1"}
}
```

`project.key` is the configured SemSource deployment namespace. Its
`identity_kind: deployment_namespace` label is load-bearing: it is a workbench scope key, not a
canonical project entity and never a six-part entity ID. A later project-view change may add a
separate canonical project identity without reinterpreting this field.

Capability support and readiness are distinct. `ready` means a verified route is implemented and its
named readiness dependencies are ready; `not_ready` means the verified route exists but a dependency
is building, degraded, or unavailable; and `unsupported` means a known contract is not implemented.
The top-level `readiness.overall` is `ready` when source ingestion and the structural index are ready;
semantic readiness gates `code_search` without downgrading unrelated structural/source surfaces. A
missing, timed-out, or undecodable optional readiness responder
produces an unavailable signal with `state: unknown` and a sanitized `status_unavailable` reason; it
does not fail the whole document or expose raw NATS detail. Index status requests share one bounded
500-millisecond context and execute concurrently through SemStreams `RequestReady`, whose expected
no-responder/timeout path does not count against the shared NATS circuit breaker. Source readiness
combines the aggregate phase with per-source phase, error count, and last-error presence; an
all-reported aggregate with a source error is degraded, never ready.

A partially ready response therefore keeps supported routes visible while describing why a result is
not yet authoritative:

```json
{
  "contract_version": 1,
  "readiness": {
    "overall": "partial",
    "source": {"available": true, "ready": true, "state": "ready"},
    "structural_index": {"available": true, "ready": false, "state": "building", "lag": 8},
    "semantic_index": {
      "available": false,
      "ready": false,
      "state": "unknown",
      "reason": {
        "code": "status_unavailable",
        "message": "Semantic index readiness is unavailable",
        "retryable": true
      }
    }
  }
}
```

Maps make query/action additions backward compatible. Within contract version 1, servers may add
fields and map entries; clients must ignore unknown fields and capability keys. Removing or
reinterpreting an existing field, enum value, query key, action key, or readiness signal requires a
new contract version served through negotiation or a distinct route while version 1 remains stable.

## Follow-on change sequence

These IDs are proposed planning handles, not created or approved changes:

- `canonicalize-shared-graph-workbench` (`semstreams-ui`): after the cross-repo contract, deliver the
  canonical renderer and SemSource profile.
- `materialize-project-views` (SemSource): after the query/readiness contract, deliver bounded
  project-scoped materialized context views.
- `add-okf-interop-mvp` (SemSource): after project views, deliver consume-only import and bounded OKF
  export.
- `add-one-action-local-start` (SemSource): after the released workbench artifact, deliver headless
  start plus an explicit UI option.

A materialized project view is the project-scoped profile of the design note's generic materialized
context view. The profile narrows subjects, evidence, revision, and budgets; it is not a second
abstraction or an OKF page.

## Risks / Trade-offs

- **Cross-repo delivery can drift.** → Pin the UI artifact and gate SemSource releases with a real
  compatibility smoke.
- **A shared UI can absorb product semantics.** → Keep capability definitions, actions, and acceptance
  scenarios in SemSource; pass typed data to generic presentation components.
- **The `ui` flag changes meaning.** → Mark the takeover as breaking, publish the old-to-new mapping,
  and hand SemTeams packaging to the SemTeams team before release.
- **Canonicalization can become a large rewrite.** → Inventory and select behavior first; require only
  the renderer, evidence, accessibility, and product-profile gates needed by the initial workbench.
- **A graph canvas can dominate the UX because it already exists.** → Make project views and source
  status the primary navigation and treat visualization as drill-down.
- **Optional can become untested.** → Run both headless and pinned-workbench smokes in the relevant
  release gate.

## Migration Plan

1. Completed: archive `add-ui-profile` to preserve its SemTeams integration decision as historical
   truth and seed the current `ui-profile` specification.
2. Accept the cross-repo contract, then record the breaking ownership pivot and both superseded
   decisions in ADR-0009.
3. Complete the cross-product graph UI inventory in a coordinated `semstreams-ui` change.
4. Prove or obtain the explicit governed graph/evidence/revision query contract.
5. Fix evidence fidelity, attribute refresh, accessible navigation, and selected canonical behaviors.
6. Add the SemSource product profile, then publish the final pinned `semstreams-ui` artifact.
7. Replace the existing Compose `ui` service target with the pinned artifact and publish the SemTeams
   handoff and release note.
8. Prove headless and workbench behavior against the same SemSource revision.
9. Propose the follow-on changes in the dependency order recorded above.

Operational rollback pins the previous SemSource release or restores the former `ui` service
definition. The additive capability endpoint may remain inert, and no graph-state migration is
involved. The independently versioned shared UI artifact is rolled back in `semstreams-ui`, never by
changing SemTeams code from SemSource. The headless core remains unchanged throughout.

## Open Questions

1. Should the pinned workbench be a separate container image or static assets embedded into a small
   SemSource web service?
