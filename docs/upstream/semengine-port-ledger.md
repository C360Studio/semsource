# SemEngine port: import crosswalk ledger

This ledger maps each SemStreams import path SemSource uses to where it lives in SemEngine, so the qualification
branch can work from it. Source issue: [#226](https://github.com/C360Studio/semsource/issues/226). SemEngine is
read-only inventory: gaps go upstream as issues, never as code.

Measured 2026-11-09:

- SemSource `main` @ `8737c7e`, pinned to `semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.
- SemEngine `main` @ `67a8650`. The latest tag is `v0.1.0-alpha.2`.
- 224 Go files import SemStreams, 113 of them non-test, across 32 import paths.

## Re-measure

Run this from the repository root. It prints one line per import path: the path, the number of files that import it,
and how many of those are non-test files. `git grep` reads only tracked files, so `.worktrees/` is excluded.

```sh
git grep -l 'github.com/c360studio/semstreams/' -- '*.go' \
  | xargs grep -oH '"github.com/c360studio/semstreams/[^"]*"' | sort -u \
  | awk -F: '{p=$2; gsub(/"|github.com\/c360studio\/semstreams\//,"",p); all[p]++;
      if ($1 !~ /_test\.go$/) prod[p]++}
      END {for (p in all) printf "%s\t%d\t%d\n", p, all[p], prod[p]+0}' \
  | sort -t "$(printf '\t')" -k2,2nr -k1,1
```

To check a destination, look for the path on SemEngine `main` (a 404 means absent):
`gh api repos/C360Studio/semengine/contents/<path>?ref=main`. To read the engine's decision for a package, find its
row in SemEngine's `docs/admission-ledger.yaml`.

## Crosswalk

Status values:

- **present:** the package exists on SemEngine `main`.
- **change N:** the package lands in Slice 04A change N.
- **consumer-side:** SemEngine will not carry it; SemSource replaces it.
- **ask filed:** a SemEngine issue is open.

| Import | Files (non-test) | Status | Consumer-side change at the port |
| --- | --- | --- | --- |
| `component` | 70 (45) | present | Rewrite the import path. `Start(ctx)` must follow ADR-113 (see below) |
| `natsclient` | 61 (19) | present | Rewrite the import path. The contract is ADR-112. Test fixtures are [#228](https://github.com/C360Studio/semsource/issues/228) |
| `message` | 53 (29) | present | Rewrite the import path |
| `vocabulary` | 35 (20) | present | Rewrite the import path |
| `pkg/fusion` | 26 (7) | change 5, PR C ([semengine#565](https://github.com/C360Studio/semengine/pull/565), open) | Only the lens path is carried. `Fuse`, `SubQuery`, `GraphQueryClient`, and `Evidence` are not carried (03B D5). The envelope copy is removed ([semengine#110](https://github.com/C360Studio/semengine/issues/110) item 2). The UI `index` shape in `ui/src/lib/contracts/fusion.ts` follows it |
| `graph` | 24 (6) | present | Rewrite the import path |
| `metric` | 24 (5) | present | Rewrite the import path |
| `payloadregistry` | 22 (3) | present | Rewrite the import path |
| `storage` | 20 (14) | present | Rewrite the import path |
| `types` | 20 (7) | present | Rewrite the import path |
| `config` | 14 (6) | present | Rewrite the import path |
| `pkg/types` | 13 (5) | present | Rewrite the import path |
| `pkg/fusion/fusionnats` | 12 (1) | change 5, PR D (after B and C) | Readiness comes from the reply (ADR-109). This is where the no-embedder composition proof lands |
| `storage/objectstore` | 11 (5) | present | Rewrite the import path. Its `Register` and `RegisterPayloads` calls are in `run.go` |
| `pkg/errs` | 10 (3) | present | Rewrite the import path |
| `pkg/projection` | 8 (6) | present | Rewrite the import path |
| `vocabulary/cco` | 7 (1) | consumer-side | SemEngine ledger row `vocabulary/cco` is `defer-exclude`: CCO lives in SemVocab (semengine#234). Import `github.com/c360studio/semvocab/catalog/cco`; all 10 constants SemSource uses are defined there (semvocab `main` @ `3faba58`). The one non-test importer is `source/ontology/ontology.go` |
| `pkg/fusion/fusionvocab` | 4 (1) | change 5, PR C | It rides with `pkg/fusion`. The `cco.Parents` question is open upstream (PR C carries no `vocabulary/cco`); check rank signals at the port |
| `pkg/retry` | 4 (4) | present | Rewrite the import path |
| `storage/storeregistry` | 4 (2) | present | Rewrite the import path |
| `model` | 3 (1) | present; [semengine#155](https://github.com/C360Studio/semengine/issues/155) refactor open | Re-check the reader surface after #155 |
| `pkg/buffer` | 2 (1) | ask filed: [semengine#566](https://github.com/C360Studio/semengine/issues/566) | `internal/entitypub/publisher.go`: a bounded, blocking, never-dropping queue. If #566 rules `defer-exclude`, replace it with a local bounded channel |
| `processor/graph-index` | 2 (1) | present | Factory `Register` in `run.go` |
| `processor/graph-ingest` | 2 (1) | present | Factory `Register` in `run.go`. ADR-110: SemSource is a caller of the write seam (it publishes on `graph.ingest.*`) and never writes `ENTITY_STATES` itself |
| `processor/graph-query` | 2 (1) | present | Factory `Register` in `run.go`. Readiness comes from query replies (ADR-109; see #227 below) |
| `gateway/graph-gateway` | 1 (1) | consumer-side; **owner decision needed** | SemEngine ledger row is `defer-exclude`: layer 4 of 03B D5 is consumer-owned (semengine#8 comment 5930898291). SemSource composes it for GraphQL (`/graph-gateway/graphql`, exposed at `/graphql` by the UI profile). At the port SemSource either drops GraphQL or owns its own adapter |
| `graph/inference` | 1 (1) | present (the hierarchy part only, semengine#97) | `RegisterPayloads` for the hierarchy container; graph-ingest refuses to construct without it |
| `graph/readiness` | 1 (0) | present | Tests only |
| `output/websocket` | 1 (1) | change 5 per 04A D2; [semengine#34](https://github.com/C360Studio/semengine/issues/34) open; not in the 2026-11-08 change-5 direction | Port-refactor: it reads applied state instead of a NATS subject. The raw `/ws` stream contract changes with it |
| `processor/graph-clustering` | 1 (1) | change 7 (`graph/clustering` is present; the processor is not) | Factory `Register` in `run.go`. Tier 0 groups by entity-ID prefix (ADR-099); the LLM summaries are [semengine#485](https://github.com/C360Studio/semengine/issues/485) |
| `processor/graph-embedding` | 1 (1) | present (change 5, PRs A and B) | Factory `Register` in `run.go`. The HTTP embedder is [semengine#489](https://github.com/C360Studio/semengine/issues/489) (tier 1, slice 04C) |
| `service` | 1 (1) | present | Rewrite the import path. ADR-113 start context |

Removed before the port ([#225](https://github.com/C360Studio/semsource/issues/225), PR #247):
`componentregistry` and `payloadbuiltins` (semengine#3, "never"). Registration is now explicit per package in
`cmd/semsource/run.go`. SemSource's `requireRegisteredFactories` fails boot on an unknown factory; drop it if
SemEngine's component manager already fails closed.

## SemEngine ADRs SemSource adopts at the port

| ADR | Decision | What SemSource changes |
| --- | --- | --- |
| 108, processor shell | A graph processor is the shell, plus a follower if it projects `ENTITY_STATES`, plus a plug-in | Nothing directly: no SemSource component follows `ENTITY_STATES`. It applies only if one is added |
| 109, ready after boot | Readiness rides the query reply (`index_not_ready` is transient; retry); the status bucket is information only | Stop gating on `GRAPH_STATUS` and read `indexed_revision` / `min_revision` from replies (semengine#103). [#227](https://github.com/C360Studio/semsource/issues/227) removed the decode of the legacy trio |
| 110, write seam | graph-ingest is the only writer of `ENTITY_STATES` values | Already conforms: SemSource publishes to `graph.ingest.*`. Keep it a caller |
| 112, natsclient contract | natsclient uses nats.go as nats.go provides it, plus a cuttable dialer, no locks across nats.go calls, and two settings options | Review the 19 non-test `natsclient` importers for assumptions outside the contract. The fixture coupling is #228 |
| 113, Start context bounds Start | `Start(ctx)` returns when ready and holds nothing; continuing work runs under its own run context; `Stop` is the only stop | Audit the 12 non-test `Start(ctx)` implementations under `processor/`, `internal/`, and `cmd/` for run goroutines derived from the `Start` context |

## Open items

- **graph-gateway:** an owner decision is needed: drop GraphQL at the port, or own a consumer adapter.
- **pkg/buffer:** waiting on [semengine#566](https://github.com/C360Studio/semengine/issues/566).
- **output/websocket:** waiting on [semengine#34](https://github.com/C360Studio/semengine/issues/34) being scheduled.
