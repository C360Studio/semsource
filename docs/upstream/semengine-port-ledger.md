# SemEngine port: import crosswalk ledger

This ledger maps each SemStreams import path SemSource uses to where it lives in SemEngine, so the qualification
branch can work from it. Source issue: [#226](https://github.com/C360Studio/semsource/issues/226). SemEngine is
read-only inventory: gaps go upstream as issues, never as code.

Measured 2026-11-12:

- SemSource qualification branch `codex/semengine-tier0-qualification` (Draft PR #222), pinned to `semengine v0.1.0-alpha.4.0.20261112145006-7bf638643b98` (`7bf638643b98b4346101fabb39a1d016a7a15e7b`, including `semengine#579` PR #603 on top of `v0.1.0-alpha.4` `9f7f5adc86368a0a251e3c12960a9e59274de666`) and `semvocab v0.2.2` (`3faba58`).
- SemSource `main` @ `a084ab6` remains pinned to `semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` until Tier 0/1/2 qualification and cutover gates complete.
- All 31 engine import paths (`vocabulary/cco` moved to `semvocab/catalog/cco`) are present in `semengine v0.1.0-alpha.4.0.20261112145006-7bf638643b98`; zero `github.com/c360studio/semstreams` imports remain on the qualification branch.

## Re-measure

Run this from the repository root. It prints one line per import path: the path, the number of files that import it,
and how many of those are non-test files. `git grep` reads only tracked files, so `.worktrees/` is excluded.

```sh
git grep -l 'github.com/c360studio/semengine/' -- '*.go' \
  | xargs grep -oH '"github.com/c360studio/semengine/[^"]*"' | sort -u \
  | awk -F: '{p=$2; gsub(/"|github.com\/c360studio\/semengine\//,"",p); all[p]++;
      if ($1 !~ /_test\.go$/) prod[p]++}
      END {for (p in all) printf "%s\t%d\t%d\n", p, all[p], prod[p]+0}' \
  | sort -t "$(printf '\t')" -k2,2nr -k1,1
```

To check a destination, look for the path on SemEngine `v0.1.0-alpha.4` or `main` (a 404 means absent):
`gh api repos/C360Studio/semengine/contents/<path>?ref=v0.1.0-alpha.4`. To read the engine's decision for a package,
find its row in SemEngine's `docs/admission-ledger.yaml`.

## Crosswalk

Status values:

- **present:** the package exists in SemEngine `v0.1.0-alpha.4` and is adopted on `codex/semengine-tier0-qualification`.
- **consumer-side:** SemEngine does not carry it; SemSource replaces it.

| Import | Files (non-test) | Status | Consumer-side change at the port |
| --- | --- | --- | --- |
| `component` | 70 (45) | present | Rewritten to `semengine/component`. `Start(ctx)` follows ADR-113 ([#231](https://github.com/C360Studio/semsource/issues/231)) |
| `message` | 53 (29) | present | Rewritten to `semengine/message` |
| `natsclient` | 37 (20) | present | Rewritten to `semengine/natsclient`. The contract is ADR-112. Test fixtures migrated to `internal/natstest` in [#228](https://github.com/C360Studio/semsource/issues/228) |
| `vocabulary` | 35 (20) | present | Rewritten to `semengine/vocabulary`. `SemStreamsBase` renamed to `SemEngineBase` ([semengine#69](https://github.com/C360Studio/semengine/issues/69); [#256](https://github.com/C360Studio/semsource/issues/256)) |
| `pkg/fusion` | 26 (7) | present (`v0.1.0-alpha.3`+, [semengine#565](https://github.com/C360Studio/semengine/pull/565)) | Completed in [#253](https://github.com/C360Studio/semsource/issues/253). `NewEngine(graph, body, logger, opts...) (*Engine, error)` takes `WithSignals` and `WithMetrics` as options. `Resolve` returns `fusion.Resolution{Seeds, UnreadableRecords, FailedRecords}`; `exact_seed.go` returns the inner `Resolution` counts unchanged. Contract version is `"2"`, `index` and `view_revision` are removed from the engine envelope (`deferred` and `defer_reason` replace `index`), and `ui/` accepts v2 |
| `graph` | 26 (8) | present | Rewritten to `semengine/graph` (includes `ErrorCodeNotEnabled` from [semengine#579](https://github.com/C360Studio/semengine/issues/579)) |
| `metric` | 24 (5) | present | Rewritten to `semengine/metric` |
| `payloadregistry` | 22 (3) | present | Rewritten to `semengine/payloadregistry` |
| `storage` | 20 (14) | present | Rewritten to `semengine/storage` |
| `types` | 20 (7) | present | Rewritten to `semengine/types` |
| `config` | 14 (6) | present | Rewritten to `semengine/config` |
| `pkg/types` | 13 (5) | present | Rewritten to `semengine/pkg/types` |
| `pkg/fusion/fusionnats` | 12 (1) | present (`v0.1.0-alpha.3`+, [semengine#577](https://github.com/C360Studio/semengine/pull/577)) | Completed in [#253](https://github.com/C360Studio/semsource/issues/253). `fusionnats.New(requester, timeout) (*Client, error)` refuses nil requester / negative timeout; `Status` and `Close` are gone. Poisoned seeds fall back to per-ID reads and list in `unhydrated` |
| `storage/objectstore` | 11 (5) | present | Rewritten to `semengine/storage/objectstore`. Its `Register` and `RegisterPayloads` calls are in `run.go` |
| `pkg/errs` | 10 (3) | present | Rewritten to `semengine/pkg/errs` |
| `pkg/projection` | 8 (6) | present | Rewritten to `semengine/pkg/projection` |
| `vocabulary/cco` | 7 (1) | consumer-side | SemEngine ledger row `vocabulary/cco` is `defer-exclude`: CCO lives in SemVocab ([semengine#234](https://github.com/C360Studio/semengine/issues/234)). Completed in [#257](https://github.com/C360Studio/semsource/issues/257) via `github.com/c360studio/semvocab/catalog/cco` (CCO v2.2) and `catalog/bfo` (BFO 2020) |
| `pkg/fusion/fusionvocab` | 4 (1) | present (`v0.1.0-alpha.3`+, [semengine#565](https://github.com/C360Studio/semengine/pull/565)) | Completed in [#253](https://github.com/C360Studio/semsource/issues/253) and [#257](https://github.com/C360Studio/semsource/issues/257). Calls `fusionvocab.New(cco.Parents)` with `cco.Parents` from `github.com/c360studio/semvocab/catalog/cco` |
| `pkg/retry` | 4 (4) | present | Rewritten to `semengine/pkg/retry` |
| `storage/storeregistry` | 4 (2) | present | Rewritten to `semengine/storage/storeregistry` |
| `model` | 3 (1) | present | Rewritten to `semengine/model` |
| `pkg/buffer` | 2 (1) | present (`v0.1.0-alpha.4`, [semengine#566](https://github.com/C360Studio/semengine/issues/566)) | Rewritten to `semengine/pkg/buffer` |
| `processor/graph-index` | 2 (1) | present | Factory `Register` in `run.go` |
| `processor/graph-ingest` | 2 (1) | present | Factory `Register` in `run.go`. ADR-110: SemSource is a caller of the write seam (it publishes on `graph.ingest.*`) and never writes `ENTITY_STATES` itself |
| `processor/graph-query` | 2 (1) | present | Factory `Register` in `run.go`. Readiness comes from query replies (ADR-109; see #227 below); `embedding_enabled: false` disables semantic/similar queries in `embedder_type: "none"` ([semengine#579](https://github.com/C360Studio/semengine/issues/579)) |
| `gateway/graph-gateway` | 1 (1) | present (`v0.1.0-alpha.4`, [semengine#567](https://github.com/C360Studio/semengine/issues/567)) | Rewritten to `semengine/gateway/graph-gateway`. Seeded GraphQL assertions (`test/ui/ui-profile.spec.cjs`, `scripts/core-profile-smoke.sh` covering `entity`, `entitiesByPrefix`, `entitiesByPredicate`, and `pathSearch`) landed in [#251](https://github.com/C360Studio/semsource/issues/251) |
| `graph/inference` | 1 (1) | present (the hierarchy part only, semengine#97) | `RegisterPayloads` for the hierarchy container; graph-ingest refuses to construct without it |
| `graph/readiness` | 1 (0) | present | Tests only (`Reading.Raw` removed in `v0.1.0-alpha.3` #577) |
| `output/websocket` | 1 (1) | present (`v0.1.0-alpha.4`, [semengine#34](https://github.com/C360Studio/semengine/issues/34)) | Rewritten to `semengine/output/websocket`. Forced wire-format change (ADR-0001 amended): SemEngine's `output/websocket` refuses JetStream inputs (`output/websocket/config.go:161`) and drops `delivery_mode` (`ack`/`nack`). `cmd/semsource/run.go` wires `KVWatchPort{Bucket: "ENTITY_STATES"}`, so `/ws` clients receive applied-state `put`/`synced` frames (at-most-once, coalesced per key) instead of raw `graph.ingest.*` (`SEED`/`DELTA`/`RETRACT`/`HEARTBEAT`) envelopes |
| `processor/graph-clustering` | 1 (1) | present (`v0.1.0-alpha.4`, [semengine#9](https://github.com/C360Studio/semengine/issues/9)) | Factory `Register` in `run.go`. Tier 0 groups by entity-ID prefix (ADR-099); LLM summaries are [semengine#485](https://github.com/C360Studio/semengine/issues/485) |
| `processor/graph-embedding` | 1 (1) | present (`v0.1.0-alpha.3`+, [semengine#550](https://github.com/C360Studio/semengine/pull/550), [semengine#558](https://github.com/C360Studio/semengine/pull/558)) | Factory `Register` in `run.go` (omitted when `embedder_type: "none"`). Tier 0 BM25 follower + `search`/`similar` verbs are present. The HTTP embedder is [semengine#489](https://github.com/C360Studio/semengine/issues/489) (tier 1, slice 04C) |
| `service` | 1 (1) | present | Rewritten to `semengine/service`. ADR-113 start context ([#231](https://github.com/C360Studio/semsource/issues/231)) |

Removed before the port ([#225](https://github.com/C360Studio/semsource/issues/225), PR #247):
`componentregistry` and `payloadbuiltins` (semengine#3, "never"). Registration is now explicit per package in
`cmd/semsource/run.go`.

## SemEngine ADRs and Rulings SemSource Adopts at the Port

| ADR / Ruling | Decision | What SemSource changes |
| --- | --- | --- |
| 108, processor shell | A graph processor is the shell, plus a follower if it projects `ENTITY_STATES`, plus a plug-in | Nothing directly: no SemSource component follows `ENTITY_STATES`. It applies only if one is added |
| 109, ready after boot | Readiness rides the query reply (`index_not_ready` is transient; retry); the status bucket is information only | Stop gating on `GRAPH_STATUS` and read `indexed_revision` / `min_revision` from replies (semengine#103). [#227](https://github.com/C360Studio/semsource/issues/227) removed the decode of the legacy trio; [#253](https://github.com/C360Studio/semsource/issues/253) updates fusion/UI to contract v2 (`deferred` / `defer_reason`) |
| 110, write seam | graph-ingest is the only writer of `ENTITY_STATES` values | Already conforms: SemSource publishes to `graph.ingest.*`. Keep it a caller |
| 112, natsclient contract | natsclient uses nats.go as nats.go provides it, plus a cuttable dialer, no locks across nats.go calls, and two settings options | Review the 20 non-test `natsclient` importers for assumptions outside the contract. The fixture coupling is [#228](https://github.com/C360Studio/semsource/issues/228) |
| 113, Start context bounds Start | `Start(ctx)` returns when ready and holds nothing; continuing work runs under its own run context; `Stop` is the only stop | Pass a cancellable context to `StartAll` ([#231](https://github.com/C360Studio/semsource/issues/231), completed) and audit the 12 non-test `Start(ctx)` implementations under `processor/`, `internal/`, and `cmd/` |
| [semengine#69](https://github.com/C360Studio/semengine/issues/69), outward names | Rename `semstreams` outward identifiers (`SemEngineBase`, `semengine_config`, `semengine.graph.mutation`, `semengine_*` metrics) | Completed in [#256](https://github.com/C360Studio/semsource/issues/256) for Go code/tests (`run.go`, `internal/cutover/buckets.go`, `internal/governance/`, `test/e2e/`, `test/setup03a/`) and [#236](https://github.com/C360Studio/semsource/issues/236) for docs/dashboards |
| [semengine#15–#20](https://github.com/C360Studio/semengine/issues/8), 03B/04A lifecycle & recovery rulings | #15 closed (2026-11-12): `GRAPH_INGEST_APPLIED_SEQ` records stream creation time (generation) alongside applied sequence so recreated lower-sequence streams are accepted, and `graph/follower` records `ENTITY_STATES` generation; #16 closed (2026-11-12): `composition/analyze.go` refuses at boot any stream filter that captures a reserved graph request subject (`graph.QueryVerbs()` + `graphmutation.SubjectFamily`); #17 config KV replaces file at boot (PR #329); #18 applied-input barrier deferred; #19 `ExpectedRevision` on `ReconcileMutation` (PR #93); #20 commit ambiguity preserved (PR #93); #579 closed (2026-11-12, PR #603): `graph-query` `embedding_enabled: false` refuses `semantic`/`similar` with `not_enabled` (class `invalid`) | Qualified in [#221](https://github.com/C360Studio/semsource/issues/221): `configs/tiers/tier0-foundation.json` (`embedder_type: "none"`) omits `graph-embedding` and sets `embedding_enabled: false`; `TestKnownAnswerCorpus` (64/64) and `TestDesiredSourceLifecycleWithoutRecovery` (33/33 + 37/37) pass on `SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed` |

## Test fixture and CI

`natsclient.NewTestClient` is not carried, and SemEngine ruled a consumer test fixture consumer-owned
([semengine#568](https://github.com/C360Studio/semengine/issues/568), 2026-11-09). SemSource's fixture is
`internal/natstest` ([#228](https://github.com/C360Studio/semsource/issues/228)): `natstest.New(t, opts...)` returns
a `*natsclient.Client` built with `NewClient`/`Connect`, the recipe SemEngine documents.

- **Sites migrated:** 48 calls in 38 `//go:build integration` files, plus 3 helper parameters (now `*natstest.Server`).
- **Options:** `WithJetStream` (it replaces `WithKV` and `WithJetStream`) and `WithStreams(natstest.Stream{...})`.
  Streams keep the same bounds (1 h, 64 MiB, discard old).
- **Lifetime:** one broker per test, as before. SemEngine's suggestion of one shared broker per package, emptied
  between tests, is the marked `// shortcut:` upgrade.
- **Pins at the port (`v0.1.0-alpha.4.0.20261112145006-7bf638643b98`):**
  - Go toolchain: `go 1.26.11` (matched in `Dockerfile` and `go.mod`).
  - NATS image: `natstest.Image` pins `nats:2.14.4-alpine` by digest (the SETUP 03A lane's image); SemEngine pins
    `nats:2.15.0-alpine`.
  - testcontainers-go: `v0.42.0`.
  - nats.go: `v1.51.0`.
  - semvocab: `v0.2.2`.

## Tier 0 Qualification Evidence ([#221](https://github.com/C360Studio/semsource/issues/221))

- **Upstream Slice 04A blockers — all closed and verified (2026-11-12):**
  - **[semengine#579](https://github.com/C360Studio/semengine/issues/579) (closed 2026-11-12, PR #603, `7bf638643b98b4346101fabb39a1d016a7a15e7b`):** `graph-query` adds `embedding_enabled` (defaults to `true`; `embedding_enabled: false` refuses `semantic`/`similar` outright with `not_enabled` class `invalid`). SemSource `configs/tiers/tier0-foundation.json` (`embedder_type: "none"`) omits `graph-embedding`, sets `embedding_enabled: false` on `graph-query`, sets `semantic_enabled: false` on `source-manifest` and `mcp-gateway`, reports `semantic_index` as `disabled` (`not_enabled`) and `code_search` as `unsupported` (`not_enabled`), and maps `not_enabled` to HTTP 501 `not_enabled` (`class: "invalid"`, `retryable: false`).
  - **[semengine#15](https://github.com/C360Studio/semengine/issues/15) (closed 2026-11-12):** `GRAPH_INGEST_APPLIED_SEQ` tracks stream generation (`applied_generation` + `applied_sequence`, 16 bytes) and `graph/follower` tracks `ENTITY_STATES` generation. Both `broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content` pass in `TestKnownAnswerCorpus`.
  - **[semengine#16](https://github.com/C360Studio/semengine/issues/16) (closed 2026-11-12):** `composition/analyze.go` refuses at boot any stream filter overlapping reserved graph request subjects (`graph.QueryVerbs()` and `graphmutation.SubjectFamily`). Verified in `TestKnownAnswerCorpus` (`rpc_wildcard_collision_reproduced` + `rpc_wildcard_startup_refused`).
- **Live Docker NATS Qualification Runs (`2026-11-12`):**
  - **Engine pin:** `github.com/c360studio/semengine v0.1.0-alpha.4.0.20261112145006-7bf638643b98` (`7bf638643b98b4346101fabb39a1d016a7a15e7b`)
  - **Binary SHA-256:** `17c9a86af5d178cab237d7be52771900ce34e7b24e9575fe3bb945ffe7e6d259`
  - **NATS image:** `nats:2.14.4-alpine`
  - **`TestKnownAnswerCorpus` (`SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed`):**
    - Command: `SETUP03A_BINARY=/tmp/semsource-tier0 SETUP03A_OUT=/tmp/setup03a-tier0-foundation SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed go test -tags=qualification -count=1 -v -run TestKnownAnswerCorpus ./test/setup03a`
    - Corpus SHA-256: `9724050f1ebdc0623460557bd4bb5c73c9b2591b4032737b79f9e7db821b0258`
    - Result: **PASS (64/64 observations, `SemEngineSlices: [0]`, 32.5s)** — includes `cold_semantic_disabled_contract`, `warm_semantic_disabled_contract`, `cold_doc_exact_passage_content`, `warm_doc_exact_passage_content`, `broker_restart_reingested_exact_relationship`, `broker_restart_reingested_exact_content`, `rpc_wildcard_collision_reproduced`, `rpc_wildcard_startup_refused`, and checked process exits (`application_restart_checked_exit`, `pending_broker_restart_forced_exit`, `rpc_collision_checked_exit`).
  - **`TestDesiredSourceLifecycleWithoutRecovery` (`SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed`):**
    - Command: `SETUP03A_BINARY=/tmp/semsource-tier0 SETUP03A_OUT=/tmp/setup03a-tier0-desired-lifecycle SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed go test -tags="qualification,removal" -count=1 -v -run TestDesiredSourceLifecycleWithoutRecovery ./test/setup03a`
    - Corpus SHA-256: `c972f68cc82863046b147a5a68fc1e48a75fa016768afda9113da264e5f416bb`
    - Result: **PASS (`original`: 33/33 observations; `runtime_added`: 37/37 observations; `SemEngineSlices: [0]`, 213.2s)**.
- **semvocab & CCO v2.2 class reconciliation ([#257](https://github.com/C360Studio/semsource/issues/257)) — completed consumer-side (`source/ontology/ontology.go`, `source/ast/vocabulary.go`, `source/vocabulary/predicates.go`):**
  - Uses `bfo.Namespace + "BFO_0000176"` (`continuant part of`) from `github.com/c360studio/semvocab/catalog/bfo` and CCO v2.2 constants from `github.com/c360studio/semvocab/catalog/cco` so `cco.Parents(iri)` returns the full BFO 2020 + CCO v2.2 superclass chain for `fusionvocab.New(cco.Parents)`.
  - Optional future shared pack: once SemSource and SemTeams agree on shared software/code & agentic vocabularies (#208), propose a C360-local pack in `semvocab` anchored via `superclasses` to `bfo-2020` and `cco-2.2`.
