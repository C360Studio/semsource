# AGENTS.md

Tool-neutral entry point for AI coding agents. This file stands alone for the
essentials — an agent that reads only this can work safely — and points to
`CLAUDE.md` for depth rather than restating it, so the two cannot disagree.

## Project

SemSource is the source-knowledge ingestion service for the C360 sem* family. It
turns repos, docs, URLs, config files, and media into typed SemStreams graph
facts and publishes governed entity-state payloads for downstream consumers
(SemSpec, SemDragon, SemOps). Go; NATS JetStream via
`github.com/c360studio/semstreams`; Task runner; revive.

**Product boundary:** SemSource owns source ingestion and projection. SemStreams
owns the governed graph substrate and its mutation/query contracts. Work that
belongs upstream goes upstream — see the last non-negotiable below.

## Agent roles

Canonical, platform-neutral role contracts live in `.agents/contracts/`.
`.claude/agents/` and `.codex/agents/` are thin adapters that name exactly one
contract and carry none of its content.

| Role | Use it for | Contract |
| --- | --- | --- |
| `go-component-reviewer` | Reviewing a new or changed semstreams component — config tags, Discoverable, factory registration, payload registry, NATS usage | `.agents/contracts/go-component-reviewer.md` |
| `graph-event-reviewer` | Reviewing entity identity, event semantics (SEED/DELTA/RETRACT/HEARTBEAT), federation merge, watch correctness | `.agents/contracts/graph-event-reviewer.md` |

Both are read-only: they report findings with file:line references and a
severity, and do not implement fixes unless separately authorized.

**Spawning a role agent whose scope matches the diff is the default execution
path for nontrivial work — no user permission needed.** There is no "don't spawn
agents unless asked" rule in this repository. Spawn both concurrently when a
change touches both failure classes; spawn neither when it touches neither, since
a reviewer outside its scope returns noise. The owner session keeps the decision —
these roles return findings, never commits. Massively-parallel `Workflow`
orchestration is a separate question and remains opt-in.

Four shared decision skills live in `.agents/skills/` — `kv-or-stream` and
`orchestration-check` are **vendored** from semstreams (framework truth we do not
own), `new-payload` and `query-pattern` are **deliberate forks** where our
conventions genuinely differ. Read the canonical `.agents/skills/<name>/SKILL.md`
directly; the `.claude/skills/` entries of the same names are thin adapters.
`.agents/README.md` explains both modes and why each skill is in the one it is in.

Run `task agents:check` after touching any contract, adapter, skill, or the
semstreams pin.

## Where the depth lives

- **`CLAUDE.md`** — architecture, component patterns, entity-identity and
  predicate conventions, the roadmap. Read it before implementing.
- **`openspec/config.yaml`** (`context:`) — the canonical planning context:
  Purpose, Product Boundary, per-artifact rules, non-negotiables. Read it before
  scoping.
- **`docs/adr/`** — genuine decision records (irreversible / cross-repo). The
  mechanics an ADR implies live in the capability spec, not the ADR.
- **`docs/upstream/semstreams-asks.md`** — framework gaps, with triage status.

## Spec-driven development (OpenSpec)

Non-trivial or cross-cutting work starts with an OpenSpec **change** (proposal +
tasks + spec deltas) *before* code — `openspec new` / the `/opsx:*` commands
(`new`, `continue`, `apply`, `verify`, `archive`). Small mechanical fixes don't
need one. `openspec/specs/<capability>/spec.md` is current truth (seeded lazily,
verified against code — never backfilled); `openspec/changes/<id>/` holds
proposals until archived. Run `openspec validate` before finalizing.

## Build, test, gate

```bash
task tools:install                          # once: revive at the CI-pinned version
task lint                                   # vet + gofmt + revive (warnings fail)
go test ./...                               # unit
go test -tags=integration ./...             # integration (testcontainers)
go test -tags=e2e ./test/e2e/               # black-box binary
go test -tags=garage ./storage/s3store/     # S3 compatibility against real Garage
task agents:check                           # agent config vs the semstreams pin
```

Before pushing, also run `task ui:image:release:test` if you touched
`.github/workflows/`, `Taskfile.yml`, or `scripts/` — those shell contract suites
assert the *shape* of the workflows and are not covered by the Go gate.

## Non-negotiables (also in openspec/config.yaml)

- Entity IDs are deterministic 6-part IDs, valid NATS KV keys; construct via
  `entityid.*` only. Raw binary bytes never enter triples (store by reference).
- Every graph write carries a semantic envelope (semstreams ADR-055).
- The live graph is retention-first — never NATS TTL/MaxBytes for graph lifecycle.
- CI green before push: revive **warnings fail** (v1.15.0), gofmt, go vet, go test.
  `task lint` refuses to run against a missing or drifted revive rather than
  reporting a clean local lint that CI then contradicts.
- SemStreams has its own team — file framework gaps as GitHub issues and record
  them in `docs/upstream/semstreams-asks.md`. **Never open a PR, commit, branch,
  or issue comment against that repository from here.**
