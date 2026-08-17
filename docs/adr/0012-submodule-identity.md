# ADR-0012: Submodule identity is canonical (URL-derived project, gitlink-SHA version); instances are parent-scoped

- **Status:** Accepted
- **Date:** 2026-08-17

## Context

A repo that links git submodules previously ingested them (when their trees
happened to be materialized) under the PARENT watch path's org/project/version.
Every consumer of a shared submodule therefore forked its identity, the pinned
commit appeared nowhere in provenance, and two pins of one submodule collided
on the same entity IDs with different content. Entity IDs are permanent: the
scoping rule chosen here cannot be revised without re-keying every graph that
contains submodule code (semsource#185).

## Decision

Entities produced from a declared git submodule carry identity derived from
the submodule itself, never from the parent that links it:

1. **project** = `URLToSlug` of the submodule's **resolved** URL (relative
   `.gitmodules` URLs resolve against the declaring repo's remote first),
   passed through `entityid.SystemSlug`.
2. **version** = the first **12 hex characters** of the gitlink SHA, as a
   fixed truncation — not `git rev-parse --short`, whose width varies by
   repo. It rides the existing version-scoping surface
   (`entityid.ScopedSystemSlug`), so code from two pins of one submodule
   occupies two disjoint, deterministic system scopes. Doc and config
   entities carry the same pin via their project slug (`<project>-<sha12>`).
3. **org** = the parent source's org. Namespace policy is unchanged; no
   automatic `public.*` adoption.

Consequence: identical (resolved URL, gitlink SHA) linked from ANY parent, at
any checkout location, yields byte-identical entity IDs — the governed graph
merges them instead of forking per consumer.

Component **instance** names are deliberately NOT canonical: they carry a
parent+link-scoped suffix (`-via-<parent>-<path>`), because
`sourcespawn`/ConfigManager registration is idempotent by instance name — a
shared name would let one parent's registration overwrite another's (or one
link's removal tear down its twin's instance). Entity identity canonical,
instance identity parent-scoped.

## Consequences

- **BREAKING** for graphs that accidentally ingested submodule code under
  parent identity: fresh-reseed posture applies (`down -v` + reseed).
- A submodule's provenance question — "which pinned commit produced this
  symbol" — is answerable from the ID's version scope alone.
- Identity expansion requires the resolved URL and the gitlink SHA, which
  exist only in a materialized checkout; discovery is therefore a runtime
  concern (see the git-submodule-ingestion spec and the change's design.md
  for mechanics — they are not part of this decision).
