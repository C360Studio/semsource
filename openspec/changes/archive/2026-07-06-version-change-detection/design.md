# Design: version-change-detection

## Context

The supersession component (`processor/supersession/`) already enumerates versioned
code entities (`graphquery.Client.QueryPrefixAll`), projects each into a
version-independent `candidate` (`correspondence.go`), groups them by `corrKey`
(`org, project, path, name, ctype, pkg`), and classifies a body-hash change between
a pair (`classifyChange` in `edges.go`). It emits adjacent `code.lineage.*` edges as
a background pass triggered on `graph.supersession.run`.

The version diff needs exactly those primitives, applied **between two named
versions** instead of adjacent pairs, plus verbatim body hydration and a query
surface. So it lives in the same component and reuses `correspondence.go` directly.

## Decisions

### D1 — MVP is the body-hash set-diff (ADR-0008 row 69), nothing richer

Scope is the ADR's tier-0 row: correspond symbols across the two version scopes and
classify by body hash. Explicitly out: git-diff-hunk changesets (row 70 / #4),
rename tracking (row 71), semantic summaries (row 72). A rename shows as
`removed` + `added` — documented, not silently mis-corresponded.

### D2 — Direct two-version correspondence, not adjacency chain-walking

The supersession *edges* are adjacency-based (for lineage/ranking). The diff instead
corresponds the `from` and `to` candidate sets **directly**: group both by `corrKey`,
then per key look at which side(s) are present. This makes any `(from, to)` pair work
— adjacent or not (v1.9 → v1.12) — with no chain traversal, and is independent of
whether a supersession pass has run.

### D3 — Classification (honest about "can't tell")

Per `corrKey` bucket, using only the `from`/`to` candidates:

| Present | Body hashes | Status |
|---|---|---|
| `to` only | — | `added` |
| `from` only | — | `removed` |
| both | equal, same kind | `unchanged` |
| both | differ, same kind | `changed` |
| both | either hash absent, or kinds differ | `indeterminate` |

`indeterminate` reuses `classifyChange`'s `ok=false` path (it refuses to compare
hashes from different predicates, whose encodings differ — an equal-looking compare
would fabricate a `changed`). It is surfaced as its own status, never folded into
`changed`. If a `corrKey` has multiple candidates on one side at the same version
(shouldn't happen — same version + same identity — but possible with imperfect data),
the newest by `indexedAt` wins and the collision is counted in the response.

### D4 — Surface: `graph.query.versionDiff` + HTTP + `code_changes` MCP tool

- **NATS** request/reply on `graph.query.versionDiff`, request
  `{project, from, to, want_bodies?, budget?}`, subscribed by the supersession
  component (which already owns the query client). Follows the graph-query subject
  namespace (`graph.query.*`) so it sits with the other read APIs.
- **HTTP** `POST /supersession/versionDiff` via `RegisterHTTPHandlers` (bounded body,
  same shape) so a non-NATS consumer can call it.
- **MCP** `code_changes(project, from, to)` on mcp-gateway, forwarding to the NATS
  subject and returning the JSON verbatim — modeled on `codeContext`/`fusionQuery`.

Home = the supersession component (least wiring; it already has the query client,
correspondence, and versioned-entity enumeration). Alternative considered: factor
`correspondence.go` into a shared package and add a standalone `versiondiff`
component — rejected for MVP (a second consumer would justify it; one does not).

### D5 — Verbatim before/after bodies, budget-capped

Changed symbols carry `from_body` and `to_body` (verbatim), added carry `to_body`,
removed carry `from_body` — hydrated from the `code.body.store` + `code.body.key`
ObjectStore handle via a `fusion.BodyResolver` (the same resolver the code-context
gateway uses; prefer the shared `StoreRegistry`, fall back to attaching the CONTENT
bucket). Hydration is bounded by a budget (`max_symbols`, `max_body_bytes`); when the
cap is hit, the response is truncated and `truncated: true` + the dropped count are
surfaced (never silently). `want_bodies=false` skips hydration entirely (status-only,
cheap).

### D6 — Version resolution and scope

`from`/`to` are matched against `code.artifact.version`; `project` against
`code.artifact.project`. `project` is **required** — `corrKey` is source-scoped by
`(org, project)`, so without it identical path/name across sources would collide.
`org` is the intrinsic leading ID segment (`entityid.OrgFromID`), not user input.
Only **versioned code entities** (carrying `code.artifact.version`) participate;
non-versioned or non-code entities are skipped. If either version resolves to zero
entities, the response is empty with a note naming the missing version — not a
false-empty diff.

### D7 — Honest readiness envelope

The response carries a readiness/absence envelope (like the fusion gateway): if the
structural index is not ready, return the envelope with `ready:false` rather than a
diff that looks complete but isn't. Enumeration truncation (`QueryPrefixAll` hitting
`max_entities`) is surfaced too — a split correspondence group would otherwise
under-report added/removed.

## Response shape (sketch)

```json
{
  "project": "semstreams", "from": "1.9.0", "to": "1.10.0",
  "ready": true,
  "counts": { "added": 3, "removed": 1, "changed": 7, "unchanged": 120, "indeterminate": 0 },
  "changes": [
    { "name": "Fuse", "path": "pkg/fusion/engine.go", "type": "method", "package": "fusion",
      "status": "changed", "from_id": "...", "to_id": "...",
      "from_body": "func (e *Engine) Fuse(...) {...}", "to_body": "func (e *Engine) Fuse(...) {...}" }
  ],
  "truncated": false
}
```

Unchanged symbols are counted but omitted from `changes` by default (they are the
bulk); a future `include_unchanged` flag can list them if a consumer needs it.

## Risks & mitigations

- **Rename/move shows as remove+add (D1).** Documented limitation; tier-1
  embedding correspondence (ADR row 71) is the future fix. Not misleading as long as
  the response never *claims* rename detection.
- **Missing body hashes → `indeterminate` (D3).** Surfaced honestly; a consumer sees
  which symbols couldn't be classified rather than a false `changed`/`unchanged`.
- **Large diffs.** Budget cap + surfaced truncation (D5); `want_bodies=false` for a
  cheap status-only pass.
- **Enumeration cost.** Each call enumerates the project's versioned entities via
  `QueryPrefixAll` (bounded by `max_entities`). Acceptable for an on-demand query;
  if it becomes hot, a prefix scoped tighter than the whole graph is a follow-up.

## Alternatives considered

- **Walk supersession edges and aggregate** — rejected: only works for adjacent
  pairs without multi-hop traversal, and depends on a supersession pass having run.
  Direct correspondence (D2) is simpler and pass-independent.
- **New standalone component** — rejected for MVP (D4): duplicates query-client and
  body-resolver wiring for one consumer.
- **Emit diff edges at ingest** — rejected: O(versions²) pairwise, and the ADR's
  model is retain-and-compute-on-read, not precompute-all-pairs.
