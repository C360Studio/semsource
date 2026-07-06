## Why

The fusion gateway (`code-context` / `doc-context`, ADR-0004) runs two lens
instances — `code` and `docs` — over **one shared embedding index**. NL seed
resolution has no per-lens domain filter, so in a code-heavy corpus a small doc
set is **diluted**: an NL `doc_context` query ranks code entities above the
relevant document. Measured live (httpx dogfood): 1304 Python code entities vs
30 docs → `doc_context "what exceptions can be raised"` returned Python test
functions above `docs/exceptions.md`; the same query with docs not drowned
returns 100% documents, top-1 correct.

SemStreams **beta.141** (ADR-071, gh#463 — our filed ask #16) added the missing
hook: `fusion.Request.Scope []string`, a list of OR-matched dot-delimited
entity-ID **prefixes** that constrain NL seed resolution to a lens's own domain.
It is NL-only (ignored by symbol/prefix resolve modes) and empty = today's
behavior. SemSource is on beta.141 but does not set it. The **product half is
ours**: choose and wire the scope per lens.

## What Changes

- The fusion gateway sets a **default `Scope` per lens** before fusing, when the
  request does not already carry one:
  - `docs` lens → the doc domain prefix (`{org}.semsource.web`).
  - `code` lens → the code-language domain prefixes (`{org}.semsource.golang`,
    `.python`, `.typescript`, `.javascript`, `.java`, `.svelte`).
- The `{org}` segment is the deployment's single global org, already available to
  the component as `deps.Platform.Org` (= the required top-level `namespace`,
  forced onto every source). No new config field.
- A caller-provided `Scope` is respected verbatim; the default applies only when
  `Scope` is empty. When the org is unknown (standalone/tests with no platform
  identity), no default is applied — exactly today's behavior.

Strictly additive: unchanged default request bodies now scope to their lens's
domain; the only observable change is that a small domain stops being diluted by
a larger co-resident one.

## Non-goals

- No change to the SemStreams `pkg/fusion` engine or the `fusion.Lens` interface
  (the scope hook already landed upstream in beta.141; this is pure adoption).
- No new config surface, per-request toggle, or "unscoped NL" escape hatch beyond
  passing an explicit `Scope` (YAGNI for MVP; add later if a cross-domain NL use
  case appears).
- No change to symbol/prefix resolve modes — scope is NL-only by contract.
- No multi-org support — SemSource enforces a single global org per deployment
  (`namespace`); revisit only if that invariant changes.

## Capabilities

### New Capabilities

- `fusion-gateway`: the deterministic code_context/doc_context fusion gateway
  gains **domain-scoped NL retrieval** — each lens instance retrieves seeds only
  from its own domain so a smaller domain is not diluted by a larger co-resident
  one. (Lazy-seeded here: only the requirement this change touches.)

## Impact

- `processor/code-context/component.go` — default `req.Scope` per lens in `serve`.
- `processor/code-context/` — a new unit test for the per-lens prefix derivation.
- `internal/governance/` — an integration test: ingest code + doc entities, fuse
  a doc NL query with the docs lens, assert a doc seeds/ranks top (not drowned).
- `docs/upstream/semstreams-asks.md` — mark ask #16 RESOLVED + ADOPTED.
- Consumers (SemSpec, SemDragon via MCP/HTTP `doc_context`) see accurate doc
  retrieval in mixed code+doc corpora with no client change.
