# SemEngine Tier 0 consumer tasks

## 1. Planning claim and prerequisites

- [x] Obtain owner approval of this proposal and design. The merged reference #213 is recorded at
  `3604a9ce8d5aec717253d7a905389f7910efd25d`; consumer issue #221 is open.
- [x] Inspect live SemEngine #8/#9 and PR #21; record 03B contract work as a dependency, not an engine API.
- [x] Inventory concrete composition/provider and corpus dependencies; architect accepts the bounded
  consumer inventory for planning only. Record current seams, required changes and code references in
  `design.md`; no engine API, runtime change or available foundation config is implied.
- [x] Obtain independent review of the bounded consumer design and its prerequisite/acceptance mapping.
  Architect and independent reviewer approve planning only; strict OpenSpec and formatting checks pass.
  Implementation still requires the separate engine admission gate below.
- [x] Record approved 03B contracts, package scope, relevant #15–#20 repair/decision dispositions and
  the exact admitted usable SemEngine Tier 0 commit before implementation begins.
  03B contracts merged in SemEngine PR #21 at `9286055e2c9a18426a73da38a60766845ab9e48e`; #15–#20,
  #69, and #579 are closed with recorded rulings (`docs/upstream/semengine-port-ledger.md`, including
  the 2026-11-12 closures of `semengine#15` for stream/bucket generation tracking, `semengine#16` for
  boot-time refusal of stream filters overlapping reserved graph RPC subjects, and `semengine#579` in
  PR #603 at `7bf638643b98b4346101fabb39a1d016a7a15e7b` for `graph-query` `embedding_enabled: false`
  returning `not_enabled` class `invalid`).
  The [2026-11-08 pin ruling](https://github.com/C360Studio/semengine/issues/9#issuecomment-6724212898)
  (amended on #9 / #557), all 4 package admissions (`pkg/buffer` #566, `gateway/graph-gateway` #567,
  `output/websocket` #34, `processor/graph-clustering` #9), and #15/#16/#579 are met by
  `github.com/c360studio/semengine v0.1.0-alpha.4.0.20261112145006-7bf638643b98`
  (commit `7bf638643b98b4346101fabb39a1d016a7a15e7b`, 2026-11-12).

## 2. Future implementation after admission

- [x] Migrate the separate qualification branch to SemEngine exclusively; prove no SemStreams runtime
  or mixed-substrate binary, and measure production/test closure against the admitted Tier 0 boundary.
- [x] Implement a true no-embedder composition and capability-appropriate readiness/public interfaces
  using admitted contracts, with failing-first tests for changed consumer behavior (`configs/tiers/tier0-foundation.json`,
  `embedder_type: "none"` omitting `graph-embedding` and setting `embedding_enabled: false` on `graph-query`
  and `semantic_enabled: false` on `source-manifest` / `mcp-gateway`; HTTP 501 `not_enabled` on
  `/code-context/search` and `/doc-context/*` NL queries).
- [x] Add an explicit graph-foundation corpus selection without weakening existing known answers or
  incorrectly requiring lexical/neural APIs; retain the original BM25/neural reference workload
  (`SETUP03A_PROFILE=foundation`, `SemEngineSlices: [0]`).
- [x] Qualify an admitted public exact document/passage body path; the current corpus selects document
  bodies through NL retrieval. Preserve exact bytes and bodyless-parent semantics without a private-store shortcut
  (`docs.Lens.ResolveMode` routes dotted entity IDs and entity ID prefixes to `fusion.ResolveModePrefix` over
  `POST /doc-context/context`, preserving empty parent body + `passage` containment relations and byte-exact
  passage hydration from `CONTENT`).
- [x] Prove structural readiness and authoritative absence with no embedder present; expose unsupported
  retrieval capabilities honestly and test that no model service is required (`cold_semantic_disabled_contract`,
  `warm_semantic_disabled_contract`, `cold_exact_absence`, `warm_exact_absence`).
- [x] Preserve the Enabled:false envelope until the admitted SemEngine #17 contract passes its
  removal/restart/re-add proof; record #18–#20 decisions and their consumer consequences honestly
  (`TestDesiredSourceLifecycleWithoutRecovery` passes `original` 33/33 and `runtime_added` 37/37 on
  `SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed`).

## 3. Qualification and handoff

- [x] Pass graph ingestion, identity, metadata/provenance, exact queries and bodies, updates,
  deletion/recreation, application restart and broker re-ingestion through admitted public interfaces
  (`TestKnownAnswerCorpus` 64/64 observations pass on `SETUP03A_PROFILE=foundation SETUP03A_IDENTITY=governed`).
- [x] Require `broker_restart_reingested_exact_relationship` and
  `broker_restart_reingested_exact_content` to pass; no known-at-pin waiver applies to SemEngine
  (both pass in `TestKnownAnswerCorpus`; `restart-guard.json` records 16-byte `applied_generation` +
  `applied_sequence`).
- [x] Qualify #16's admitted reserved-RPC/startup-refusal contract using new engine assertions; preserve
  the historical PubAck-collision reproduction without counting hazard reproduction as an engine pass
  (`rpc_wildcard_collision_reproduced` + `rpc_wildcard_startup_refused` pass in `TestKnownAnswerCorpus`).
- [x] Record exact consumer/engine commits, binary/corpus/configuration hashes, commands, results and
  production/tagged-test closures. Reject unadmitted dependencies and distinguish captured build
  provenance from embedded binary revision metadata (`docs/upstream/semengine-port-ledger.md`).
- [x] Assert graceful application exit zero, intentional crash signals and exact owned broker cleanup;
  reuse the checked-stop behavior rather than inheriting the original probe's unasserted wait result
  (`application_restart_checked_exit`, `pending_broker_restart_forced_exit`, `rpc_collision_checked_exit`,
  `cleanup_checked_process_exit`, and zero leaked `semsource.qualification=setup03a` containers).
- [x] Record that SemEngine's separate SemConnect reference and engine dogfood workloads remain
  tracked on `C360Studio/semengine#9`; SemSource code/docs corpus results qualify the SemSource
  consumer gate without claiming those upstream obligations.
- [ ] Obtain independent Go/component and graph/event sign-off on implementation and exact
  qualification (including retired `graph.*` config rejection, ADR-0001 `ENTITY_STATES` KV-watch
  WebSocket contract update, >=4-part docs lens entity-ID prefix check, zero-`semstreams`-import
  guard test, and `StreamOverride.Subjects` validation).
- [x] Cut `main` over to SemEngine for Tier 0 (`none`) and statistical (`bm25`) compositions while
  retaining Tier 0 as a lower-profile regression gate for 04B/04C (`semengine#489`, `semengine#485`).

Current stop point: independent review sign-off on PR #222 at the updated HEAD and merge to `main`.
