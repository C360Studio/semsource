# SemEngine port: import crosswalk ledger

This ledger maps each SemStreams import path SemSource uses to where it lives in SemEngine, so the qualification
branch can work from it. Source issue: [#226](https://github.com/C360Studio/semsource/issues/226). SemEngine is
read-only inventory: gaps go upstream as issues, never as code.

Measured 2026-11-10:

- SemSource `main` @ `a084ab6`, pinned to `semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.
- SemEngine `v0.1.0-alpha.3` @ `057bc8bf` (tag object `138ae558`).
- SemVocab `v0.2.2` @ `3faba58` (`main`).
- 200 Go files import SemStreams, 114 of them non-test, across 32 import paths (27 present in `v0.1.0-alpha.3`, 5 absent).

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

To check a destination, look for the path on SemEngine `v0.1.0-alpha.3` or `main` (a 404 means absent):
`gh api repos/C360Studio/semengine/contents/<path>?ref=v0.1.0-alpha.3`. To read the engine's decision for a package,
find its row in SemEngine's `docs/admission-ledger.yaml`.

## Crosswalk

Status values:

- **present:** the package exists in SemEngine `v0.1.0-alpha.3`.
- **change N:** the package lands in Slice 04A change N.
- **consumer-side:** SemEngine will not carry it; SemSource replaces it.
- **ask filed:** a SemEngine issue is open.

| Import | Files (non-test) | Status | Consumer-side change at the port |
| --- | --- | --- | --- |
| `component` | 70 (45) | present | Rewrite the import path. `Start(ctx)` must follow ADR-113 (see below) |
| `message` | 53 (29) | present | Rewrite the import path |
| `natsclient` | 37 (20) | present | Rewrite the import path. The contract is ADR-112. Test fixtures migrated to `internal/natstest` in [#228](https://github.com/C360Studio/semsource/issues/228) |
| `vocabulary` | 35 (20) | present | Rewrite the import path. `SemStreamsBase` is now `SemEngineBase` ([semengine#69](https://github.com/C360Studio/semengine/issues/69); [#256](https://github.com/C360Studio/semsource/issues/256)) |
| `pkg/fusion` | 26 (7) | present (`v0.1.0-alpha.3`, [semengine#565](https://github.com/C360Studio/semengine/pull/565)) | Tracked in [#253](https://github.com/C360Studio/semsource/issues/253). `NewEngine(graph, body, logger, opts...) (*Engine, error)` takes `WithSignals` and `WithMetrics` as options. `Resolve` returns `fusion.Resolution{Seeds, UnreadableRecords, FailedRecords}`; `exact_seed.go` must return the inner `Resolution` counts unchanged. Contract version is `"2"`, `index` and `view_revision` are removed, and `ui/src/lib/contracts/fusion.ts` must accept v2 |
| `graph` | 24 (6) | present | Rewrite the import path |
| `metric` | 24 (5) | present | Rewrite the import path |
| `payloadregistry` | 22 (3) | present | Rewrite the import path |
| `storage` | 20 (14) | present | Rewrite the import path |
| `types` | 20 (7) | present | Rewrite the import path |
| `config` | 14 (6) | present | Rewrite the import path |
| `pkg/types` | 13 (5) | present | Rewrite the import path |
| `pkg/fusion/fusionnats` | 12 (1) | present (`v0.1.0-alpha.3`, [semengine#577](https://github.com/C360Studio/semengine/pull/577)) | Tracked in [#253](https://github.com/C360Studio/semsource/issues/253). `fusionnats.New(requester, timeout) (*Client, error)` refuses nil requester / negative timeout; `Status` and `Close` are gone (remove `Status()` in `internal/governance/fusion_gateway_integration_test.go`). Poisoned seeds fall back to per-ID reads and list in `unhydrated` |
| `storage/objectstore` | 11 (5) | present | Rewrite the import path. Its `Register` and `RegisterPayloads` calls are in `run.go` |
| `pkg/errs` | 10 (3) | present | Rewrite the import path |
| `pkg/projection` | 8 (6) | present | Rewrite the import path |
| `vocabulary/cco` | 7 (1) | consumer-side | SemEngine ledger row `vocabulary/cco` is `defer-exclude`: CCO lives in SemVocab ([semengine#234](https://github.com/C360Studio/semengine/issues/234)). Add `github.com/c360studio/semvocab/catalog/cco` ([#257](https://github.com/C360Studio/semsource/issues/257)). Note: `semvocab` pins CCO v2.2 (`https://www.commoncoreontologies.org/ont0000xxxx`): 5 of SemSource's 10 constants keep their Go names (`Act`, `Person`, `DesignativeInformationContentEntity`, `InformationBearingArtifact`, `Algorithm`), 2 are renamed (`Artifact` → `MaterialArtifact`, `Document` → `MaterialCopyOfDocument`), and 3 are excluded as absent from CCO v2.2 (`SoftwareCode`, `Specification`, `Identifier`). Reconcile in [#257](https://github.com/C360Studio/semsource/issues/257) |
| `pkg/fusion/fusionvocab` | 4 (1) | present (`v0.1.0-alpha.3`, [semengine#565](https://github.com/C360Studio/semengine/pull/565)) | Tracked in [#253](https://github.com/C360Studio/semsource/issues/253) and [#257](https://github.com/C360Studio/semsource/issues/257). Call `fusionvocab.New(cco.Parents)` with `cco.Parents` from `github.com/c360studio/semvocab/catalog/cco` |
| `pkg/retry` | 4 (4) | present | Rewrite the import path |
| `storage/storeregistry` | 4 (2) | present | Rewrite the import path |
| `model` | 3 (1) | present; [semengine#155](https://github.com/C360Studio/semengine/issues/155) refactor open | Re-check the reader surface after #155 |
| `pkg/buffer` | 2 (1) | ask filed: [semengine#566](https://github.com/C360Studio/semengine/issues/566) | `internal/entitypub/publisher.go`: a bounded, blocking, never-dropping queue. If #566 rules `defer-exclude`, replace it with a local bounded channel |
| `processor/graph-index` | 2 (1) | present | Factory `Register` in `run.go` |
| `processor/graph-ingest` | 2 (1) | present | Factory `Register` in `run.go`. ADR-110: SemSource is a caller of the write seam (it publishes on `graph.ingest.*`) and never writes `ENTITY_STATES` itself |
| `processor/graph-query` | 2 (1) | present | Factory `Register` in `run.go`. Readiness comes from query replies (ADR-109; see #227 below) |
| `gateway/graph-gateway` | 1 (1) | `defer-exclude` upstream (03B D4/D5: GraphQL is a consumer-owned layer-4 adapter, not a tier-0 deferral); re-admission asked: [semengine#567](https://github.com/C360Studio/semengine/issues/567) | The 03B premise "SemSource does not exercise it" (A3.3) grepped only `test/setup03a`. SemSource's GraphQL route is a published consumer contract (`docs/integration/m5-consumer-integration.md:178-185`), SemDragon configures `/graph-gateway/graphql`, SemTeams needs the same GraphQL surface (`semteams#402`/`#406`), and the UI profile's Caddy exposes `/graphql`. (Note: `semspec` is retired and excluded from consumer requirements.) The consumer assertion for #567 (`test/ui/ui-profile.spec.cjs` and `scripts/core-profile-smoke.sh` asserting `entity`, `entitiesByPrefix`, `entitiesByPredicate`, and `pathSearch` against seeded IDs and triples) is in [#251](https://github.com/C360Studio/semsource/issues/251) |
| `graph/inference` | 1 (1) | present (the hierarchy part only, semengine#97) | `RegisterPayloads` for the hierarchy container; graph-ingest refuses to construct without it |
| `graph/readiness` | 1 (0) | present | Tests only (`Reading.Raw` removed in `v0.1.0-alpha.3` #577) |
| `output/websocket` | 1 (1) | change 5 per 04A D2; [semengine#34](https://github.com/C360Studio/semengine/issues/34) open; not in `v0.1.0-alpha.3` | Port-refactor: it reads applied state instead of a NATS subject. The raw `/ws` stream contract changes with it |
| `processor/graph-clustering` | 1 (1) | next after change 5 per 2026-11-08 ruling on [semengine#9](https://github.com/C360Studio/semengine/issues/9) (`graph/clustering` is present; the processor is not in `v0.1.0-alpha.3`) | Factory `Register` in `run.go`. Tier 0 groups by entity-ID prefix (ADR-099); LLM summaries are [semengine#485](https://github.com/C360Studio/semengine/issues/485). Until it ports, `localSearch` and `graph.query.searchGraph` text tier reply `index_not_ready` |
| `processor/graph-embedding` | 1 (1) | present (`v0.1.0-alpha.3`, [semengine#550](https://github.com/C360Studio/semengine/pull/550), [semengine#558](https://github.com/C360Studio/semengine/pull/558)) | Factory `Register` in `run.go`. Tier 0 BM25 follower + `search`/`similar` verbs are in `v0.1.0-alpha.3`. The HTTP embedder is [semengine#489](https://github.com/C360Studio/semengine/issues/489) (tier 1, slice 04C) |
| `service` | 1 (1) | present | Rewrite the import path. ADR-113 start context ([#231](https://github.com/C360Studio/semsource/issues/231)) |

Removed before the port ([#225](https://github.com/C360Studio/semsource/issues/225), PR #247):
`componentregistry` and `payloadbuiltins` (semengine#3, "never"). Registration is now explicit per package in
`cmd/semsource/run.go`. SemSource's `requireRegisteredFactories` fails boot on an unknown factory; drop it if
SemEngine's component manager already fails closed.

## SemEngine ADRs and Rulings SemSource Adopts at the Port

| ADR / Ruling | Decision | What SemSource changes |
| --- | --- | --- |
| 108, processor shell | A graph processor is the shell, plus a follower if it projects `ENTITY_STATES`, plus a plug-in | Nothing directly: no SemSource component follows `ENTITY_STATES`. It applies only if one is added |
| 109, ready after boot | Readiness rides the query reply (`index_not_ready` is transient; retry); the status bucket is information only | Stop gating on `GRAPH_STATUS` and read `indexed_revision` / `min_revision` from replies (semengine#103). [#227](https://github.com/C360Studio/semsource/issues/227) removed the decode of the legacy trio; [#253](https://github.com/C360Studio/semsource/issues/253) updates fusion/UI to contract v2 |
| 110, write seam | graph-ingest is the only writer of `ENTITY_STATES` values | Already conforms: SemSource publishes to `graph.ingest.*`. Keep it a caller |
| 112, natsclient contract | natsclient uses nats.go as nats.go provides it, plus a cuttable dialer, no locks across nats.go calls, and two settings options | Review the 20 non-test `natsclient` importers for assumptions outside the contract. The fixture coupling is [#228](https://github.com/C360Studio/semsource/issues/228) |
| 113, Start context bounds Start | `Start(ctx)` returns when ready and holds nothing; continuing work runs under its own run context; `Stop` is the only stop | Pass a cancellable context to `StartAll` ([#231](https://github.com/C360Studio/semsource/issues/231)) and audit the 12 non-test `Start(ctx)` implementations under `processor/`, `internal/`, and `cmd/` |
| [semengine#69](https://github.com/C360Studio/semengine/issues/69), outward names | Rename `semstreams` outward identifiers (`SemEngineBase`, `semengine_config`, `semengine.graph.mutation`, `semengine_*` metrics) | Tracked in [#256](https://github.com/C360Studio/semsource/issues/256) for Go code/tests (`run.go:818`, `internal/cutover/buckets.go`, `internal/governance/`, `test/e2e/`, `test/setup03a/`) and [#236](https://github.com/C360Studio/semsource/issues/236) for docs/dashboards |
| [semengine#17–#20](https://github.com/C360Studio/semengine/issues/8), 03B lifecycle rulings | #17 config KV replaces file at boot (PR #329); #18 applied-input barrier deferred; #19 `ExpectedRevision` on `ReconcileMutation` (PR #93); #20 commit ambiguity preserved (PR #93) | Keep `Enabled:false` until #17's replacement passes removal/restart/re-add proof on the qualification branch; keep #215 deferred per #18 |

## Test fixture and CI

`natsclient.NewTestClient` is not carried, and SemEngine ruled a consumer test fixture consumer-owned
([semengine#568](https://github.com/C360Studio/semengine/issues/568), 2026-11-09). SemSource's fixture is
`internal/natstest` ([#228](https://github.com/C360Studio/semsource/issues/228)): `natstest.New(t, opts...)` returns
a `*natsclient.Client` built with `NewClient`/`Connect`, the recipe SemEngine documents. At the port only the
`natsclient` import inside `internal/natstest` changes.

- **Sites migrated:** 48 calls in 38 `//go:build integration` files, plus 3 helper parameters (now `*natstest.Server`).
- **Options:** `WithJetStream` (it replaces `WithKV` and `WithJetStream`) and `WithStreams(natstest.Stream{...})`.
  Streams keep the same bounds (1 h, 64 MiB, discard old).
- **Lifetime:** one broker per test, as before. SemEngine's suggestion of one shared broker per package, emptied
  between tests, is the marked `// shortcut:` upgrade.
- **Pins at the port (`v0.1.0-alpha.3`):**
  - Go toolchain: SemSource `go 1.26.3`, SemEngine & SemVocab `go 1.26.9`.
  - NATS image: `natstest.Image` pins `nats:2.14.4-alpine` by digest (the SETUP 03A lane's image); SemEngine pins
    `nats:2.15.0-alpine`.
  - testcontainers-go: SemSource `v0.40.0`, SemEngine `v0.44.0`.
  - nats.go: SemSource `v1.52.0`, SemEngine `v1.55.0`.
  - semvocab: `v0.2.2` (currently requires `semengine v0.1.0-alpha.2`; MVS selects `v0.1.0-alpha.3` when SemSource
    pins `alpha.3`).

## Open items

- **semvocab & CCO v2.2 class reconciliation ([#257](https://github.com/C360Studio/semsource/issues/257)) — consumer-side now, optional shared pack later:**
  - **No `semvocab` issue is needed to unblock the SemSource port.** `semvocab`'s rule (`AGENTS.md` § 2) only admits
    catalog constants that exist verbatim in a pinned external standard release (`bfo-2020`, `cco-2.2`, W3C/OGC), and
    defers C360-local packs until consumers (`semsource`, `semteams`) finalize their shared entity/predicate schemas
    (see `semvocab#33` Blocker 1).
  - **What is 100% on SemSource's side right now (`source/ontology/ontology.go`, `source/ast/vocabulary.go`,
    `source/vocabulary/predicates.go`):**
    1. Replace the hardcoded BFO 2.0 `http://purl.obolibrary.org/obo/BFO_0000050` (`part of`, excluded in BFO 2020) in
       `CodeBelongsTo` and `CodeBelongs` with `bfo.Namespace + "BFO_0000176"` (`continuant part of`) from
       `github.com/c360studio/semvocab/catalog/bfo`.
    2. Map all 10 class stamps in `source/ontology/ontology.go` to existing `semvocab/catalog/cco` (CCO v2.2) constants
       so `cco.Parents(iri)` returns the full BFO 2020 + CCO v2.2 superclass chain for `fusionvocab.New(cco.Parents)`
       without requiring any `semvocab` changes:
       - `cco.Act`, `cco.Person`, `cco.DesignativeInformationContentEntity`, `cco.InformationBearingArtifact`,
         `cco.Algorithm`: **unchanged Go names** in `semvocab/catalog/cco`.
       - `cco.Artifact` (`ast.TypeRepo`) → **`cco.MaterialArtifact`** (`ont00000995`, renamed in CCO v2.2).
       - `cco.Document` (`web.page`, `web.doc`) → **`cco.MaterialCopyOfDocument`** (`ont00001298`, CCO v2.2 `altLabel`:
         `"Document"`) or `cco.DescriptiveInformationContentEntity` (`ont00000853`).
       - `cco.Identifier` (`config.image`) → **`cco.DesignativeInformationContentEntity`** (`ont00000686`; `Identifier`
         was an invented SemStreams constant absent from CCO 1.x/2.2, and `config.dependency` already uses
         `DesignativeInformationContentEntity`).
       - `cco.Specification` (`config` `gomod`/`project`/`package`) → **`cco.PrescriptiveInformationContentEntity`**
         (`ont00000965`; `Specification` was an invented SemStreams constant absent from CCO 1.x/2.2).
       - `cco.SoftwareCode` (non-function code symbols: struct, interface, class, type, var, const, package) →
         **`cco.PrescriptiveInformationContentEntity`** (`ont00000965`, direct parent of `cco.Algorithm` at depth 3 vs
         `Algorithm`'s depth 4, preserving function/method specificity over passive declarations) or
         `cco.InformationContentEntity` (`ont00000958`, depth 2).
  - **When to file with `semvocab` later:**
    1. If we want finer CCO v2.2 leaf classes that *do* exist in `semvocab`'s vendored `CommonCoreOntologiesMerged.ttl`
       (e.g. `Code Identifier` `ont00000062` or `Non-Name Identifier` `ont00000649`), that is a small `catalog/cco`
       admission ask with `source/ontology` as the named caller.
    2. Once SemSource and SemTeams agree on shared software/code & agentic vocabularies (`https://semspec.dev/ontology/code/`,
       `https://semspec.dev/ontology/agentic/`, #208), propose a C360-local pack in `semvocab` (alongside `semvocab#33`
       `packs/agnt`) anchored via `superclasses` to `bfo-2020` and `cco-2.2`.
- **graph-gateway:** the owner ruled (2026-11-09) to ask for re-admission:
  [semengine#567](https://github.com/C360Studio/semengine/issues/567), with SemSource's seeded GraphQL assertions (`test/ui/ui-profile.spec.cjs`, `scripts/core-profile-smoke.sh` covering `entity`, `entitiesByPrefix`, `entitiesByPredicate`, and `pathSearch`) in
  [#251](https://github.com/C360Studio/semsource/issues/251). Owner ruling (2026-11-10): `semspec` is retired and not considered an active consumer (`traverse` is retired `semspec`-only and out of scope).
- **pkg/buffer:** waiting on [semengine#566](https://github.com/C360Studio/semengine/issues/566).
- **output/websocket:** waiting on [semengine#34](https://github.com/C360Studio/semengine/issues/34) being scheduled.
- **processor/graph-clustering:** next port in SemEngine Slice 04A per
  [semengine#9](https://github.com/C360Studio/semengine/issues/9) (2026-11-08 ruling).
