# Proposal: Versioned-Source Supersession

## Why

Open item #1 (PR #39) made each indexed dependency version a distinct subgraph, but the versions
sit in the graph **unrelated** — there is no way to ask "what changed in `RankSignals` between
`beta.129` and `beta.130`?", which is the payoff ADR-0008 retains versions for. This change
**relates** corresponding entities across versions so the dependency-upgrade query (SemSpec's driving
use case) and version diffing become answerable. It is the tier-0, deterministic, no-model core of
ADR-0008 §2 (`docs/adr/0008-versioned-source-retention-supersession.md`).

## What Changes

- **Emit a version triple** on each code entity, carrying the raw registered `version` (the config
  field added in open item #1). This is the version signal correspondence keys on — the version is
  folded into the `system` ID slug and is **not** recoverable by parsing the ID.
- **Deterministic correspondence pass** — identify "the same symbol across versions" by grouping
  entities on their **version-independent** facts (`code.artifact.path`, name, type, package) plus the
  version triple. No ID string-parsing.
- **Changed vs unchanged** — a corresponding pair is "changed" when its `code.artifact.body`
  (`code:<sha>`) differs across the two version scopes, "unchanged" when identical.
- **Emit supersession edges** — a relationship triple linking corresponding entities across versions
  (newer supersedes older), so the graph carries the version lineage.
- Additive only — **no deletion, no retraction** (retention-first, ADR-0008); no embeddings/LLM.

## Capabilities

### New Capabilities
- **`versioned-source-supersession`** — the version triple, the deterministic cross-version
  correspondence rule, and the supersession edges (the tier-0 version lineage over code entities).

### Modified Capabilities
- None. `openspec/specs/` is empty (specs are lazy-seeded); no existing capability's requirements
  change. The version-triple emission is part of this new capability, not a modification to an
  as-yet-unspecified code-ingestion capability.

## Impact

- **New correspondence/supersession pass.** Open design question (→ `design.md`): *where it runs*. It
  must see **both** version scopes at once, so it is a **graph-level pass** (enumerate each version
  scope via `graph.query.prefix`, compare, emit edges), not a per-file parse step. Candidate homes: a
  new `processor/` component vs. extending an existing ingestion/clustering pass.
- **Vocabulary.** A version predicate (e.g. `code.artifact.version`, matching the existing
  `code.artifact.*` family) and a supersession predicate (e.g. `code.lineage.supersedes`) in
  `source/ast/predicates.go` / the vocabulary registry. Exact names settled in `design.md`.
- **Graph writes** all go through the semantic-envelope path (semstreams ADR-055); supersession edges
  are relationship triples (object = entity ID). Retention-first — nothing is removed.
- **Depends on** open item #1 (version scoping, PR #39): the `version` config field and
  distinct-per-version IDs are prerequisites.
- **Consumers:** SemSpec (dependency-upgrade "what changed between versions"), SemDragon.

## Non-goals

- **Rename/move correspondence** (a symbol whose name or path changed across versions) — needs
  body/signature similarity (tier-1 embeddings). Deferred.
- **Semantic change summaries** ("this bump changed retry semantics") — tier-2 LLM. Deferred.
- **The "current" ranking marker / demotion of historical versions** — ADR-0008 open item #3, a
  separate change.
- **Deletion / retirement / GC of superseded versions** — retention-first; deletion is a separate,
  framework-gated exception (ADR-0008 §5).
- **Changeset → symbol (commit/PR) modeling** — ADR-0008 open item #4, a separate change.
