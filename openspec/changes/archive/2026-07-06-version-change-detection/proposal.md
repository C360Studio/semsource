## Why

SemSource already **retains** every version of a symbol and links adjacent versions
with supersession + a `code.lineage.change` (`changed`/`unchanged`) marker
(ADR-0008 #2/#3, live). The data to answer *"what changed between version X and
version Y?"* is therefore in the graph — but there is **no query that answers it**.
A consumer today must enumerate both versions, walk supersession chains, and
aggregate by hand; there is no `diff`/`changeset` verb (verified: none in the
gateway or MCP tools).

ADR-0008 already scoped this exact capability as **tier-0, no model**:

> "What changed v1.9 → v1.10" — **tier-0 code** — *body-hash set-diff over the two
> scopes* — no model.

This change ships that query: given a project and two versions, return the
symbol-level changeset — **added / removed / changed / unchanged** — computed by
corresponding symbols across the two retained subgraphs and comparing body hashes,
with verbatim before/after bodies for changed symbols. It is purely additive: a
read over already-retained data, no deletion, no `semstreams#433`, no model.

## What Changes

- A new query, **`graph.query.versionDiff`** (NATS request/reply + HTTP), that takes
  `{project, from, to}` and returns the changeset over the two version scopes.
- An MCP tool **`code_changes(project, from, to)`** on the mcp-gateway that forwards
  to it — the agent-facing surface (alongside `code_context` / `code_search`).
- The diff reuses the supersession component's correspondence projection
  (`candidate` / `corrKey` / `groupByCorrespondence` / `bodyHashOf`) and change
  classification, applied **directly between the two named versions** (not adjacency
  chain-walking), so any pair — including non-adjacent (v1.9 → v1.12) — works.
- Changed symbols carry verbatim **before/after bodies** (hydrated from the
  ObjectStore handle, budget-capped); a `want_bodies=false` returns status-only.
- An honest readiness/absence envelope: a not-ready graph or a missing version
  returns an envelope with a note, never a false-empty "nothing changed".

## Non-goals

- **No git-diff-hunk changeset** (commit/PR → touched-symbol edges — ADR-0008 #4,
  ADR row 70). This ships the version-set diff (row 69); the commit-level changeset
  stays a later change.
- **No rename/move correspondence** (ADR row 71, tier-1 embeddings). A renamed or
  moved symbol appears as `removed` + `added`; documented as a known limitation.
- **No semantic/LLM change summary** (ADR row 72, tier-2). Status + verbatim bodies
  only; an agent can summarize from those.
- **No new predicates or ingest changes.** The diff reads existing versioned-code
  triples (`code.artifact.version`/`project`/`path`/`type`/`package`, `dc.terms.title`,
  `code.body.key`/`code.artifact.hash`, `code.body.store`).
- **No deletion** and no dependence on `semstreams#433` / cascade-delete.

## Capabilities

### New Capabilities

- `version-change-detection`: a deterministic query answering "what changed between
  two versions of a source" as a symbol-level changeset (added/removed/changed/
  unchanged) with verbatim before/after bodies, over the retained versioned-source
  subgraphs.

## Impact

- `processor/supersession/` — a new `graph.query.versionDiff` request handler + HTTP
  route + diff computation (reusing `correspondence.go`), and a body resolver for
  verbatim before/after hydration.
- `processor/mcp-gateway/` — a new `code_changes` MCP tool forwarding to the subject.
- `internal/governance/` — an integration test over the real graph stack.
- `docs/adr/0008-...md` — mark row 69 ("what changed X→Y") as realized by this change.
- Consumers (SemSpec, SemDragon, Claude Code via MCP) gain "what changed between
  these two versions" without walking the graph themselves.
