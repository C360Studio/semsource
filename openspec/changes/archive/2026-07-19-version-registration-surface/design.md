# Design: Version Registration Surface

## Context

The version-intelligence chain (version triples → supersession pass → `code_changes` /
`graph.query.versionDiff`) is fully built and fully unreachable:

- `processor/ast-source` already accepts `watch_paths[].version` and emits
  `code.artifact.project`/`code.artifact.version` triples when it is non-empty
  (versioned-source-supersession spec, gated on `version != ""`). But `config.SourceEntry` has
  no version field, `astComponentConfig` never sets one, `add_source` has no argument, and repo
  expansion drops the concept — no deployment can produce a non-empty version.
- Non-semver supersession ordering (`processor/supersession/ordering.go`) keys on
  `indexedAt` = `dc.terms.created`, which graph-ingest rewrites on restart/re-ingest — lineage
  direction can invert and go cyclic (both versions demoted).
- `versionDiff`'s `hydrateOne` returns `("", false)` for storage errors and for genuinely
  absent bodies alike — an incomplete answer is indistinguishable from a complete one.
- The existing `TestIntegration_VersionDiff` publishes entities directly; nothing proves the
  chain from a real registration surface, which is exactly how it broke unnoticed.

## Goals / Non-Goals

**Goals:**

- A version is declarable on every registration surface (config entries, `add_source`,
  repo-expanded sources) and flows to the ast component's existing `version` field.
- Non-semver ordering is a pure function of the version strings — restart-stable by
  construction.
- Storage failures in diff hydration are visible per symbol, distinct from absence.
- An automated proof ingests two fixture versions through the real registration path and
  asserts a correct diff.

**Non-Goals:**

- Implicit ref-derived version defaults for branch-expanded sources. Branch identity already
  scopes entity IDs (`BranchScopedSlug`); silently stamping `version=branch` would change every
  multi-branch deployment's IDs (a breaking re-key) to encode information the IDs already
  carry. The delta's "where design so decides" is decided: explicit-only, propagated through
  expansion like `Language` — a declared version on a repo entry reaches its ast leaf.
- Wizard prompts (the wizard writes `semsource.json`; the field is documented and hand-editable;
  interactive prompting can follow demand).
- Changing semver ordering, correspondence keys, or edge predicates (all shipped and correct).

## Decisions

### D1: `Version` — and `Project` — ride the same path `Language` does

`config.SourceEntry` gains `Version string` (json `version`). `astComponentConfig` passes it
into the component's `watch_paths[].version` verbatim. `AddSourceInput` gains `version`,
mapped in `sourceEntryFromAddInput`. Repo expansion (`expandSingleBranch`,
`expandBranchSources`, `BranchWatcherRef`) propagates it to the ast leaf exactly as `Language`/
`Languages` propagate. No surface invents a version; every surface transports one.

*Implementation finding (design amended):* version alone cannot make the chain reachable —
supersession corresponds entities by **project**, and `astComponentConfig` derives project from
the path slug, so two versions of one dependency registered at two paths
(`…/depA@v1.9.0`, `…/depA@v1.10.0`) get different projects and never relate. `SourceEntry`
therefore also gains `Project string` (json `project`, optional): when set it overrides the
path-derived slug (slugified for ID-safety, still branch-scoped), and the ast instance name
gains a version suffix when a version is set so two versioned registrations of one project get
distinct component instances. `add_source` gains the matching `project` argument; expansion
propagates it. Absent both fields, every ID and instance name is byte-identical to today.

### D2: Non-semver ordering = natural version-string comparison

Replace the `indexedAt` key with a numeric-aware (natural) comparison of the version strings
themselves: split into digit and non-digit runs, compare runs pairwise (numeric runs by value,
text runs lexically). `v9 < v10`, `r2023b < r2024a`, and every comparison is a pure function of
the two strings — total, transitive (derived from a canonical key), and identical on every
restart by construction. `versionComparable` for non-semver pairs becomes "different version
strings" (was "different timestamps"). `indexedAt` leaves the ordering path entirely; a
restart-rewritten `dc.terms.created` can no longer touch lineage direction.

*Alternative considered:* persisting a first-seen timestamp. Rejected — write-once semantics
don't exist in the substrate's per-predicate-replace merge, and inventing a side-store for
ordering state adds a failure mode where a pure function suffices. Natural comparison can still
mis-order exotic schemes (e.g. date-less codenames); it is then *stably, documentedly* ordered —
the failure the audit found was instability, not scheme coverage.

### D3: Hydration errors are a third state

`hydrateOne` distinguishes `(body, skipped, failed)`: a resolver error marks the symbol's body
as failed. The wire response gains an additive per-symbol boolean `body_error` (absent when
false) plus a response-level note listing how many bodies failed, so a consumer can tell
"changed, body unavailable due to storage error" from "changed, no body was ever offloaded".
Budget-skips keep their existing truncation accounting.

### D4: Reachability proof drives the real registration path

New integration test in `internal/governance/`: build two fixture source trees (v1/v2 of one
tiny Go project), register them through `sourcespawn.Build` with explicit versions (the same
builder every surface funnels into), instantiate the resulting ast-source component configs as
real components over the live graph stack, trigger the supersession pass, and assert
`graph.query.versionDiff` returns the fixture's known added/removed/changed/unchanged counts
with correct before/after bodies. The MCP `code_changes` → `graph.query.versionDiff` mapping is
already pinned by mcp-gateway's own tests; this proof owns the half that was never covered —
registration to answer.

## Risks / Trade-offs

- [Natural ordering changes existing non-semver lineage direction where it disagreed with
  ingest timing] → intended: timing-derived direction was the unstable defect; edges are
  re-emitted idempotently by the next supersession run (diffNew), and the audit found the
  feature unreachable in practice — there are no real deployments to re-order.
- [`body_error` widens the wire response] → additive, omitted-when-false; consumers that ignore
  it see today's shape.
- [Explicit-only versions mean unversioned deployments stay unversioned] → correct by design —
  version-less stays byte-identical (the supersession spec's D1 win); the surfaces now exist
  and are documented.

## Migration Plan

Additive fields on config/tool surfaces; ordering change affects only non-semver lineage (see
risk above); no ID changes for any existing entity. Rollback = revert.

## Open Questions

- None blocking.
