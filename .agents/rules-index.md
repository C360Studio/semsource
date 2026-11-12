# Rules Enforcement Index

Mirror of `AGENTS.md` § Rules and what enforces them, section for section and row for row. Edit both together.

## 1. Entity identity and graph writes

| Gate | Rule | Where |
| --- | --- | --- |
| `verify+review` | Entity IDs are 6-part, purely intrinsic (no timestamps, instance IDs, or insertion order), valid NATS KV keys, built only via `entityid.*` with authority passed from component dependencies | `entityid` tests; `graph-event-reviewer` |
| `review` | Raw binary bytes never enter triples; store by reference | `graph-event-reviewer` |
| `review` | Every graph write carries a semantic envelope (ADR-055) | `graph-event-reviewer`, `go-component-reviewer` |
| `review` | The live graph is retention-first — never NATS TTL/MaxBytes for graph lifecycle | `graph-event-reviewer` |
| `review` | An identity-breaking change is a clean storage break plus reseed; existing IDs are never aliased or rewritten | `graph-event-reviewer`; SETUP 03A |

## 2. Components, payloads, and the substrate boundary

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | New components carry config.go, component.go, factory.go, and payload_registry.go per the component pattern above | `go-component-reviewer` |
| `review` | Payload registration is explicit at bootstrap via `buildPayloadRegistry()`; no `init()` side effects, no blank imports | `go-component-reviewer`; `/new-payload` |
| `review` | SemSource owns ingestion and projection; SemEngine owns governed graph state and mutation/query contracts | `openspec/config.yaml` |
| `review` | Framework gaps are filed as GitHub issues against `C360Studio/semengine` (or `semstreams`) with mechanism, file:line evidence, a reproduction, and our workaround; never a PR, commit, branch, or issue comment there | review |
| `verify` | Vendored decision skills match the pinned upstream byte for byte; forks record the upstream digest last reconciled | `task agents:check` |

## 3. Go quality and verification

| Gate | Rule | Where |
| --- | --- | --- |
| `verify` | `gofmt`, `go vet`, and revive v1.15.0 with zero warnings; `task lint` refuses a missing or drifted revive | `task lint`; CI `lint` |
| `verify` | Unit and integration tests pass before push | CI `test`, `test-integration` |
| `verify` | Workflow, Taskfile, and script changes pass the shell contract suites | `task ui:image:release:test`; CI |
| `verify` | `docs/QUICKSTART.md` marked command blocks run verbatim against public fixtures | `quickstart.yml` |
| `review` | Tests assert behavior and outcomes; concurrent tests synchronize explicitly, never sleep, and run with `-race` | review |
| `review` | Conclusions and handoffs end with what was skipped and residual risk; a deliberate limit is marked `// shortcut: <limit>, <when to upgrade>` | `.agents/protocol.md`; ponytail skill |

## 4. Workflow, review, and merge

| Gate | Rule | Where |
| --- | --- | --- |
| `review` | Non-trivial or cross-cutting work starts with an OpenSpec change before code; specs are seeded lazily and verified against code, never backfilled | "Spec-driven development" below |
| `review` | Before claiming or mutating state, Deliverable, Scope/non-goals, Definition of Done, and Stop boundary are clear | `.agents/protocol.md` |
| `review` | A claim is a draft PR with `Closes #n`, opened before the work, from its own worktree | `.agents/protocol.md` |
| `verify+review` | A PR merges only when CI is green on an up-to-date head and no `class:flake` issue is open; rerun-to-green is not a fix | `main` ruleset + `Merge check` (`task merge:check`) |
| `verify+review` | Code PR bodies record `implemented-by: <agent> (<model>, <effort>)` and a not-weaker `reviewed-by: <agent> (<model>, <effort>)` with a review record at the head; a completed OpenSpec change is archived before the PR is ready | `Merge check`; `ateam doctor` warns |
| `verify+review` | Spawning a role agent whose scope matches the diff is the default and needs no user permission; a reviewer is never weaker than the implementer | `task agents:check`; `.agents/README.md` |
| `verify` | `AGENTS.md` < 24,000 bytes, `CLAUDE.md` only imports it, `.agents/rules-index.md` mirrors these sections; contracts and skills have thin adapters, specialist contracts open with a pointer to their base | `task ateam:doctor`; `task agents:check` |
