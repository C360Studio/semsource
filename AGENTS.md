# SemSource

The single entry point for AI coding agents (Claude, Codex, Gemini). `CLAUDE.md` only imports this file.

## Project

SemSource is the source-knowledge ingestion service for the C360 sem* family. It ingests repos, docs, URLs, config
files, and media into governed SemStreams graph facts, then exposes source/query/status surfaces for agents and
operator UIs (SemSpec, SemDragon, SemOps, SemTeams). The raw WebSocket export still exists for stream-oriented consumers, but the primary read contracts are
graph query, MCP, HTTP status, and GraphQL through the UI profile. Part of Complete 360 Studio; MIT licensed.

**Product boundary:** SemSource owns source ingestion and projection. SemStreams owns the governed graph substrate and
its mutation/query contracts. Work that belongs upstream goes upstream (rule 2 below).

- **Language:** Go. **Platform:** `github.com/c360studio/semstreams` (governed entity state, component/service APIs,
  NATS JetStream). **Config:** JSON (`semsource.json`). **Lint:** revive, not golangci-lint.
- **Transport:** SemStreams ServiceManager over NATS JetStream/KV, HTTP/MCP status and tool routes, GraphQL via the UI
  profile, plus the raw WebSocket graph stream (`output/websocket`) on port 7890 from the external service.

## Commands

```bash
semsource init              # Interactive setup wizard → writes semsource.json
semsource run               # Start the ingestion engine
semsource add [type]        # Add a source (interactive or with flags)
semsource remove            # Remove a source (interactive or --index N)
semsource sources           # List configured sources
semsource validate          # Check config without starting
semsource version           # Print version
```

Non-interactive: `semsource add ast --path ./src --language go --watch`, `semsource remove --index 2`. Bare
`semsource` with no args auto-runs if `semsource.json` exists.

```bash
task tools:install                         # once: revive at the CI pin; ateam pin into the main checkout's .tools/bin
task lint                                  # vet + gofmt + revive (warnings fail)
go build ./...
go test ./...                              # unit tests only
go test -tags=integration ./...            # include integration tests (testcontainers)
go test -tags=e2e ./test/e2e/              # black-box binary tests
go test -tags=garage ./storage/s3store/     # S3 compatibility against real Garage
go test -run TestName ./path/to/package    # single test
go test -race -tags=integration ./...      # race detection
task agents:check                          # agent contracts/adapters/skills vs the semstreams pin
task ateam:doctor                          # harness (pinned ateam): AGENTS.md size, rules-index parity, hooks
task spec:check                            # openspec validate --strict + Hold: placement (in task check and CI)
task merge:check -- <pr>                   # the Merge check locally: ruleset, attestation lines, review record, flakes
```

Before pushing, also run `task ui:image:release:test` if you touched `.github/workflows/`, `Taskfile.yml`, or
`scripts/` — those shell contract suites assert the *shape* of the workflows and are not covered by the Go gate.

Hooks and MCP registrations call `scripts/ateam`, which runs the main checkout's pinned `.tools/bin/ateam`
(shared by every worktree) and falls back to the `ateam` on PATH.

## Architecture

```text
[semsource.json] -> [Source Intake] -> [Handler Dispatch] -> [Entity Publisher]
    -> [SemStreams graph-ingest / ENTITY_STATES] -> [graph query / MCP / GraphQL]
```

Follows the semstreams pattern **Listen → Process → Persist → Publish**. All handlers implement `SourceHandler`
(`Ingest`, `Watch`, `Supports`, `SourceType`):

| Handler | Sources | Watch mechanism |
| --- | --- | --- |
| `GitHandler` | Local/remote repos | git hook / polling |
| `ASTHandler` | Go, TS/JS, Java, Python, Svelte | fsnotify |
| `DocHandler` | Markdown, plain text | fsnotify |
| `ConfigHandler` | go.mod, package.json, Dockerfile | fsnotify |
| `URLHandler` | HTTP/S URLs | configurable polling |

**Entity identity (6-part ID):** `{org}.{platform}.{system}.{domain}.{type}.{instance}`, e.g.
`acme.catalog-a1b2c3.github-com-acme-gcs.golang.function.NewController`. `org.platform` is the deployment authority
supplied by the composition root; `system` identifies the source; `domain.type` its taxonomy. `platform_id` is a
configured stem (default `semsource`), not the effective authority — SemStreams mints and retains the effective
identity in the namespaced config bucket. Foreign subjects require a separately admitted import lane and are
read-only mirrors; an open-source repository does not authorize a handler to replace local authority.

| Entity type | Identity inputs after retained org/platform |
| --- | --- |
| Code symbol | Source/project/version slug, language, symbol type, scoped symbol instance |
| Git commit | Repository slug, git taxonomy, commit type, shortened commit SHA |
| URL / document | Source slug, web taxonomy, page/doc/chunk type, canonical URL or logical-path instance |
| Config entity | Source slug, config taxonomy, artifact/dependency type, type-specific stable key |

ADRs 102/104 require a one-time clean storage break from beta.161: provision fresh owned graph storage and re-ingest;
do not alias or rewrite existing IDs. Restarts within one supported deployment retain authority. The migration pin,
profile crosswalk, recorded failures, and acceptance limits are in
[SETUP 03A](docs/testing/setup-03a/compatibility.md).

**Event types:** SEED (full graph on start and consumer reconnect), DELTA (incremental upserts from watch events),
RETRACT (explicit removal), HEARTBEAT (liveness during quiet periods). SemSource publishes governed entity state
through the substrate; consumers query it through `graph.query.*`, HTTP/MCP, or GraphQL and never register a
federation processor or bridge the raw event stream into consumer-owned storage. Consumers wait for
`graph.query.status` → `phase: "ready"` before querying.

**Key design decisions:** no separate batch mode (initial seeding is the first pass of the continuous loop);
`at-least-once` WebSocket delivery with ack/nack; MVP targets same-LAN deployment (no TLS/reverse proxy); AST parsers,
doc parsers, weburl, and vocabulary packages are self-contained in `source/` (copied from semspec, no cross-repo
dependency); `graph/` holds SemSource's domain payload and graph integration helpers.

### Component pattern

New components follow the semstreams structure; `semstreams/processor/ast-indexer/` is the canonical reference.

- **config.go** — Config struct with `json` + `schema` tags, `Validate()`, `DefaultConfig()`
- **component.go** — implements `component.Discoverable` (Meta, InputPorts, OutputPorts, ConfigSchema, Health,
  DataFlow)
- **factory.go** — `Register()` with full registration config (Name, Factory, Schema, Type, Protocol, Domain,
  Description, Version)
- **payload_registry.go** — package-level `RegisterPayloads(reg *payloadregistry.Registry)` listing a
  `payloadregistry.Registration` per payload type; each payload's `Schema() message.Type` matches its registration's
  Domain/Category/Version

New payloads: implement `message.Payload` (`Schema()`, `Validate()`, alias-based `MarshalJSON`/`UnmarshalJSON`) → add
to the package's `RegisterPayloads(reg)` → wire that call into `buildPayloadRegistry()` in `cmd/semsource/run.go`.
References: `graph/event_payload.go`, `processor/source-manifest/payload_registry.go`; checklist in `/new-payload`.

The AST source accepts only complete `watch_paths` entries; component JSON is decoded strictly:

```json
{
  "watch_paths": [
    {"path": "./service", "org": "acme", "project": "service", "languages": ["go"], "excludes": ["vendor"]}
  ],
  "watch_enabled": true
}
```

The former top-level `repo_path`, `org`, `project`, `version`, `languages`, and `exclude_patterns` values move into
each `watch_paths` entry (`exclude_patterns` becomes `excludes`).

## Rules and what enforces them

`.agents/rules-index.md` mirrors these sections row for row. "Gate": `review` means review only (no command fails when
broken); `verify` means a command or CI job fails; `verify+review` means a structural guard covers the core rule and
edge cases stay review-only. A new rule names what enforces it and adds its row here and in the index.

### 1. Entity identity and graph writes

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Entity IDs are 6-part, purely intrinsic (no timestamps, instance IDs, or insertion order), valid NATS KV keys, built only via `entityid.*` with authority passed from component dependencies | `entityid` tests; `graph-event-reviewer` |
| `review` | Raw binary bytes never enter triples; store by reference | `graph-event-reviewer` |
| `review` | Every graph write carries a semantic envelope (semstreams ADR-055) | `graph-event-reviewer`, `go-component-reviewer` |
| `review` | The live graph is retention-first — never NATS TTL/MaxBytes for graph lifecycle | `graph-event-reviewer` |
| `review` | An identity-breaking change is a clean storage break plus reseed; existing IDs are never aliased or rewritten | `graph-event-reviewer`; SETUP 03A |

### 2. Components, payloads, and the substrate boundary

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | New components carry config.go, component.go, factory.go, and payload_registry.go per the component pattern above | `go-component-reviewer` |
| `review` | Payload registration is explicit at bootstrap via `buildPayloadRegistry()`; no `init()` side effects, no blank imports | `go-component-reviewer`; `/new-payload` |
| `review` | SemSource owns ingestion and projection; SemStreams owns governed graph state and mutation/query contracts | `openspec/config.yaml` |
| `review` | Framework gaps are filed as GitHub issues against `C360Studio/semstreams` (or `semengine`) with mechanism, file:line evidence, a reproduction, and our workaround; never a PR, commit, branch, or issue comment there | review |
| `verify` | Vendored decision skills match the pinned upstream byte for byte; forks record the upstream digest last reconciled | `task agents:check` |

### 3. Go quality and verification

| Gate | Rule | Where |
| --- | --- | --- |
| `verify` | `gofmt`, `go vet`, and revive v1.15.0 with zero warnings; `task lint` refuses a missing or drifted revive | `task lint`; CI `lint` |
| `verify` | Unit and integration tests pass before push | CI `test`, `test-integration` |
| `verify` | Workflow, Taskfile, and script changes pass the shell contract suites | `task ui:image:release:test`; CI |
| `verify` | `docs/QUICKSTART.md` marked command blocks run verbatim against public fixtures | `quickstart.yml` |
| `review` | Tests assert behavior and outcomes; concurrent tests synchronize explicitly, never sleep, and run with `-race` | review |
| `review` | Conclusions and handoffs end with what was skipped and residual risk; a deliberate limit is marked `// shortcut: <limit>, <when to upgrade>` | `.agents/protocol.md`; ponytail skill |

### 4. Workflow, review, and merge

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | Non-trivial or cross-cutting work starts with an OpenSpec change before code; specs are seeded lazily and verified against code, never backfilled | "Spec-driven development" below |
| `review` | Before claiming or mutating state, Deliverable, Scope/non-goals, Definition of Done, and Stop boundary are clear | `.agents/protocol.md` |
| `review` | A claim is a draft PR with `Closes #n`, opened before the work, from its own worktree | `.agents/protocol.md` |
| `verify+review` | A PR merges only when CI is green on an up-to-date head and no `class:flake` issue is open; rerun-to-green is not a fix | `main` ruleset + `Merge check` (`task merge:check`) |
| `verify+review` | Code PR bodies record `implemented-by: <agent> (<model>)` and a not-weaker `reviewed-by: <agent> (<model>, <effort>)` with a review record at the head; a completed OpenSpec change is archived before the PR is ready | `Merge check`; `ateam doctor` warns |
| `verify+review` | Spawning a role agent whose scope matches the diff is the default and needs no user permission; a reviewer is never weaker than the implementer | `task agents:check`; `.agents/README.md` |
| `verify` | `AGENTS.md` < 24,000 bytes, `CLAUDE.md` only imports it, `.agents/rules-index.md` mirrors these sections; contracts and skills have thin adapters, specialist contracts open with a pointer to their base | `task ateam:doctor`; `task agents:check` |

## Spec-driven development (OpenSpec)

Three homes, three jobs:

| Home | Holds | Drifts? |
| --- | --- | --- |
| `openspec/specs/<capability>/spec.md` | **Current truth** — what a capability does today (`Requirement` + `GIVEN/WHEN/THEN`) | No — every change edits it via a delta |
| `openspec/changes/<id>/` | **Proposed target state** — `proposal.md` + `tasks.md` + spec deltas; archived on completion | Resolves on archive |
| `docs/adr/` | **Genuine decisions only** — irreversible choices and cross-repo contracts (the why) | No — history |

Drive a change with `/opsx:new`, `/opsx:continue`, `/opsx:apply`, `/opsx:verify`, `/opsx:archive`; run
`openspec validate --all` before finalizing. Small mechanical fixes don't need a change. ADRs record the decision; the
mechanics it implies live in the capability's spec. Read `openspec/config.yaml`'s `context:` first when scoping — it
carries the Purpose, the Product Boundary, per-artifact rules, and the non-negotiables.

## Where state lives

- **GitHub issues:** what is wanted and decided; rulings as comments. **Milestones:** what gates a release.
- **Draft PRs:** claims (`Closes #n`). **`openspec/changes/`:** target state and holds. **ADRs:** why.
- **`.ateam/state.json`** (gitignored): the local cache `ateam state pickup` / `ateam state handoff` maintain.

There is no handoff document; a prose file claiming to be one is history.

## Working here (Claude, Codex, and Gemini)

The shared protocol is `.agents/protocol.md`; read it before taking, landing, or closing work. Start a session with
`/pickup`, end it with `/handoff`. Four gates never become a pointer:

- **Clarify:** read named issues, PRs, and files first; confirm Deliverable, Scope/non-goals, Definition of Done, and
  Stop boundary. If one is ambiguous, propose a 3–4 bullet Definition of Done and ask at most 1–3 questions.
- **Claim:** a draft PR opened before the work, in its own worktree on an agent-prefixed branch (`claude/`, `codex/`,
  `gemini/`; Enjoy desktop sessions use `move_to_worktree` under `.worktrees/`). No draft PR, no claim.
- **Merge:** the `main` ruleset requires `lint`, `test`, `test-integration`, `ui-quality`, `e2e-local`, and
  `Merge check` on an up-to-date head; `Merge check` wants `implemented-by:`, `reviewed-by:`, and an approving review
  record on a code PR, and no open `class:flake` issue; squash merge.
- **Close:** the squash merge of a PR that declared `Closes #n` is the authorization; otherwise the owner's word on
  the issue.

Agents mutate only this repository. SemStreams, SemEngine, and other sister repositories are read-only inventory.

## Roles and skills

Canonical, platform-neutral definitions live in `.agents/`; `.claude/` and `.codex/` hold thin adapters that name
exactly one canonical file and carry none of its body. `.agents/README.md` is the map. Run `task agents:check`
after touching any of it, or after moving the semstreams pin.

| Contract | Use it for |
| --- | --- |
| `.agents/contracts/developer.md` | Base rules for any implementing run (ateam); read by every developer |
| `.agents/contracts/reviewer.md` | Base rules and the `reviewed-by:` record grammar for any independent review (ateam; `/review-pr`) |
| `.agents/contracts/architect.md` | Base rules for a design-track `design.md` draft (ateam; `/design`) |
| `go-component-reviewer` | Reviewing a new or changed semstreams component — config tags, Discoverable, factory registration, payload registry, NATS usage |
| `graph-event-reviewer` | Reviewing entity identity, event semantics (SEED/DELTA/RETRACT/HEARTBEAT), federation merge, watch correctness |

The two specialists are read-only: findings with file:line references and a severity, no fixes unless separately
authorized. Each opens with a pointer to the base reviewer contract.

**Spawning a role agent whose scope matches the diff is the default execution path for nontrivial work —
no user permission needed.** There is no "don't spawn agents unless asked" rule here. Spawn both specialists
concurrently when a change touches both failure classes; spawn neither when it touches neither, since a reviewer
outside its scope returns noise. The owner session keeps the decision — these roles return findings, never commits. Massively-parallel
`Workflow` orchestration is a separate question and remains opt-in.

Decision skills in `.agents/skills/`: `kv-or-stream`, `entity-or-bucket`, and `orchestration-check` are **vendored**
from semstreams (must match the pinned upstream byte for byte); `new-payload` and `query-pattern` are **deliberate
forks** whose manifest records the upstream digest last reconciled. ateam ships `pickup`, `handoff`, `preflight`,
`review-pr`, `design`, and `ponytail`. The `openspec-*` entries under `.claude/skills/` are Claude-workflow tooling.

## Where the depth lives

- **`ROADMAP.md`** — the public roadmap and the per-tag release detail (current tag beta.9); not the historical
  milestone list in `docs/spec/semsource-spec-v3.md`.
- **`docs/spec/semsource-spec-v3.md`** — the full specification. **`docs/adr/`** — decision records.
- **`openspec/config.yaml`** (`context:`) — the canonical planning context.
