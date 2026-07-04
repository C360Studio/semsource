# AGENTS.md

Tool-neutral entry point for AI coding agents working in this repository. The
detailed, canonical guidance lives in the files below — this file just points
there so it cannot drift.

## Project

SemSource is the source-knowledge ingestion service for the C360 sem* family. It
turns repos, docs, URLs, config files, and media into typed SemStreams graph
facts and publishes governed entity-state payloads for downstream consumers
(SemSpec, SemDragon, SemOps). Go; NATS JetStream via
`github.com/c360studio/semstreams`; Task runner; revive.

## Where the guidance lives

- **`CLAUDE.md`** — working guidance: architecture, build/test commands,
  component patterns, entity-identity and predicate conventions, milestones.
  Read this first for how to work in the repo.
- **`openspec/project.md`** — Purpose and the **Product Boundary** (SemSource owns
  source ingestion; SemStreams owns the graph substrate). Read before scoping any
  change.
- **`openspec/config.yaml`** — OpenSpec context + per-artifact rules + the
  non-negotiables (deterministic 6-part IDs, semantic envelope, retention-first
  graph, revive-warnings-fail CI).
- **`docs/adr/`** — genuine decision records (irreversible / cross-repo). Mechanics
  live in the capability spec, not the ADR.

## Spec-driven development (OpenSpec)

Non-trivial or cross-cutting work starts with an OpenSpec **change** (proposal +
tasks + spec deltas) *before* code — `openspec new` / the `/opsx:*` slash commands
(`new`, `continue`, `apply`, `verify`, `archive`). Small mechanical fixes don't
need one. `openspec/specs/<capability>/spec.md` is current truth (seeded lazily,
verified against code — never backfilled); `openspec/changes/<id>/` holds proposals
until archived. Run `openspec validate` before finalizing.

## Non-negotiables (also in openspec/config.yaml)

- Entity IDs are deterministic 6-part IDs, valid NATS KV keys; construct via
  `entityid.*` only. Raw binary bytes never enter triples (store by reference).
- Every graph write carries a semantic envelope (semstreams ADR-055).
- The live graph is retention-first — never NATS TTL/MaxBytes for graph lifecycle.
- CI green before push: revive **warnings fail** (v1.15.0), gofmt, go vet, go test.
- SemStreams has its own team — file framework gaps as GitHub issues
  (`docs/upstream/semstreams-asks.md`), never a PR/commit to that repo.
