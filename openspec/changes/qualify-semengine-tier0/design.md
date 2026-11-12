# SemEngine Tier 0 consumer qualification design

## Status and ownership

Consumer qualification and substrate cutover for [SemSource #221][source221] and [SemEngine #9][engine9].
The SemStreams reference baseline is merged #213 (`3604a9ce8d5aec717253d7a905389f7910efd25d`).
[03B contracts and port scope][engine8] landed through [PR #21][engine21] at
`9286055e2c9a18426a73da38a60766845ab9e48e`, and all Slice 04A prerequisites (`pkg/buffer` #566,
`gateway/graph-gateway` #567, `output/websocket` #34, `processor/graph-clustering` #9, #15 broker
generation marker, #16 reserved-RPC startup refusal, and #579 no-embedder verb refusal) are closed
on SemEngine main at `7bf638643b98b4346101fabb39a1d016a7a15e7b`
(`v0.1.0-alpha.4.0.20261112145006-7bf638643b98`).

SemSource owns composition, source adapters, consumer query policy and the acceptance workload.
SemEngine owns substrate APIs, extraction and package admission. Per the 2026-11-12 owner ruling,
PR #222 cuts SemSource `main` over to SemEngine with `semsource.json` (`embedder_type: "bm25"`) and
`configs/tiers/tier0-foundation.json` (`embedder_type: "none"`), while HTTP neural embeddings
(`semengine#489`, 04C), LLM community summaries (`semengine#485`), and SemEngine's separate
SemConnect/dogfood obligations remain tracked in their respective slices.

## Seams and Tier 0 outcomes

| Seam | Previous behavior | Tier 0 SemEngine outcome |
| --- | --- | --- |
| [Composition][graph-compose] | BM25 and `graph-embedding` always enabled | `embedder_type: "none"` omits `graph-embedding` and sets `graph-query.embedding_enabled: false` |
| [Graph config][graph-config] | Accepted retired SemStreams keys | Rejects retired `gateway_bind`, `enable_playground`, `embedding_batch_size`, `clustering_llm`, `entity_id_edges` |
| [Validation][provider-validation] | HTTP requires `embedding` capability | Foundation (`none`) and BM25 require no model registry |
| [Registries][registries] | SemStreams imports | Exclusive SemEngine + `semvocab` registrations; zero `semstreams` imports |
| [Gateway][gateway] | Shared ServiceManager HTTP mux | `graph-gateway` on `:8080/graph-gateway/graphql`; `output/websocket` watches `ENTITY_STATES` KV |
| [Fusion][fusion] | Contract v1 (`index` block) | Contract v2 (`deferred` / `defer_reason`); `not_enabled` maps to HTTP 501 `not_enabled` |
| [Docs lens][verbs] | NL-only doc retrieval | `ResolveModePrefix` for 4..6-part valid entity IDs/prefixes; NL for ordinary prose and `<4`-part tokens |
| [Removal][remove] | Disabled envelope in `semstreams_config` | Preserved in `semengine_config` KV; passes `original` (33/33) and `runtime_added` (37/37) |
| [Corpus selector][selector] | `bm25` / `neural` only | Adds `foundation` (`SemEngineSlices: [0]`) while preserving `bm25` and `neural` |
| [Corpus Stop][stop] | Unchecked wait | `checkedStop` asserts exit 0 on graceful stop, signal exit on forced crash, and zero container leaks |

## Foundation corpus partition

- **Structural & provenance (`*_entity_*`, `*_persisted_authority`, `*_exact_relationship`, `*_provenance`):**
  exact public graph reads and authority retention across restarts.
- **Exact code & absence (`*_exact_content`, `*_duplicate_name_anchors`, `*_exact_absence`):**
  deterministic symbol lookup, verbatim `CONTENT` hydration, and ready authoritative miss.
- **Exact document & passage body (`*_doc_exact_passage_content`):**
  `POST /doc-context/context` with a 4..6-part entity ID/prefix returns empty parent body, `passage`
  containment relations, and verbatim passage bytes from `CONTENT`.
- **Disabled semantic contract (`cold_semantic_disabled_contract`, `warm_semantic_disabled_contract`):**
  `/code-context/search` and NL `/doc-context/*` return HTTP 501 with `code: "not_enabled"`,
  `class: "invalid"`, `retryable: false`.
- **Lifecycle & broker recovery (`deleted_*`, `recreated_*`, `broker_restart_reingested_*`):**
  both `broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content`
  pass via `GRAPH_INGEST_APPLIED_SEQ` 16-byte `applied_generation` + `applied_sequence` (#15).
- **Reserved-RPC boundary (`rpc_wildcard_collision_reproduced`, `rpc_wildcard_startup_refused`):**
  preserves the raw NATS PubAck hazard probe and verifies SemEngine's boot-time refusal (#16).

## Verification gates

```bash
go test -race -count=1 ./...
go test -race -tags=integration -count=1 ./...
task check && task agents:check && task ui:image:release:test
SETUP03A_BINARY=/tmp/semsource-tier0 SETUP03A_OUT=/tmp/setup03a-tier0-foundation \
  SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed \
  go test -tags=qualification -count=1 -v -run TestKnownAnswerCorpus ./test/setup03a
SETUP03A_BINARY=/tmp/semsource-tier0 SETUP03A_OUT=/tmp/setup03a-tier0-desired-lifecycle \
  SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed \
  go test -tags="qualification,removal" -count=1 -v -run TestDesiredSourceLifecycleWithoutRecovery ./test/setup03a
```

[source221]: https://github.com/C360Studio/semsource/issues/221
[engine8]: https://github.com/C360Studio/semengine/issues/8
[engine9]: https://github.com/C360Studio/semengine/issues/9
[engine21]: https://github.com/C360Studio/semengine/pull/21
[graph-compose]: ../../../cmd/semsource/run.go#L760
[graph-config]: ../../../config/config.go#L25
[provider-validation]: ../../../config/config.go#L389
[registries]: ../../../cmd/semsource/run.go#L294
[gateway]: ../../../cmd/semsource/run.go#L835
[fusion]: ../../../processor/code-context/component.go#L193
[verbs]: ../../../source/fusion/lens/docs/docs.go#L54
[remove]: ../../../internal/sourcespawn/sourcespawn.go#L304
[selector]: ../../../test/setup03a/qualification_test.go#L92
[stop]: ../../../test/setup03a/qualification_test.go#L428
