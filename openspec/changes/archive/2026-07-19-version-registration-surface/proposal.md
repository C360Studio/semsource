# Proposal: Version Registration Surface

**Priority: P2** — revives a shipped-but-unreachable flagship feature

## Why

The audit confirmed that **the entire version-intelligence chain — supersession edges,
`code_changes`, `graph.query.versionDiff` — is unreachable end-to-end**: no registration surface
(config file, wizard, `add_source`, expanded repo sources) can set a source version, and without
`code.artifact.version` triples the correspondence/supersession pass has nothing to relate
(`internal/sourcespawn/components.go:96`; live-verified: mvp stack emits no version triples and
`code_changes` honestly reports no indexed entities for any version pair). The feature the
roadmap advertises for "what changed between X and Y" cannot be exercised by any consumer today.

Secondary defect: non-semver supersession ordering keys on `dc.terms.created`, which is rewritten
on every restart/edit (`processor/supersession/ordering.go:55`) — lineage can invert and go
cyclic, demoting BOTH versions.

## What Changes

- Version becomes settable on every registration surface: `semsource.json` source entries,
  `add_source` (new optional `version` argument), and branch/tag-expanded repo sources (version
  derived from the ref when explicit version is absent — decided in design).
- Sources with a version emit the `code.artifact.project`/`code.artifact.version` triples exactly
  as the versioned-source-supersession spec already requires (that spec's gating on
  `version != ""` is unchanged — this change makes non-empty versions reachable).
- Non-semver ordering uses a stable, source-anchored timestamp (e.g. registration-time version
  sequence or ref commit time), never a restart-rewritten predicate; ordering is pinned by a
  restart-survival test.
- `versionDiff` body hydration surfaces storage errors distinctly from "no body exists"
  (`processor/supersession/versiondiff_serve.go:166`).
- Acceptance: a two-version ingest of a real repo produces supersession edges and a correct
  `code_changes` answer through MCP (the audit's Q-set gains a live version-diff question).

## Capabilities

### Modified Capabilities
- `versioned-source-supersession`: "Version and source identity on code entities" gains the
  registration-surface requirement (a version CAN be declared everywhere sources are declared);
  "Deterministic cross-version correspondence" gains the stable non-semver ordering requirement.
- `version-change-detection`: "Version changeset query" gains an error-honesty scenario (storage
  failure ≠ empty body) and an end-to-end reachability scenario.

### New Capabilities
<!-- none — this completes existing capabilities -->

## Impact

- `internal/sourcespawn/`, `config/` (source entry schema), `processor/mcp-gateway/` (add_source
  argument), `processor/supersession/` (ordering, versionDiff serve), wizard optionally.
- Consumers: semdev A/B harness (version diffs are a demo-able differentiator vs Graphify).
- Boundary check: product-side; supersession pass and diff already live in SemSource.
