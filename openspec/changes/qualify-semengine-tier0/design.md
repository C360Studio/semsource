# SemEngine Tier 0 consumer qualification design

## Status and ownership

Planning only for [SemSource #221][source221] and [SemEngine #9][engine9]. The consumer reference is
merged #213, `3604a9ce8d5aec717253d7a905389f7910efd25d`. This branch still uses the reference SemStreams
module; it is not an engine build or a qualified Tier 0 composition.

The architect's consumer inventory and independent review approve this bounded plan for planning only.
The reviewer found no scope or qualification-mapping blocker. Runtime migration requires approved [03B contracts and port scope][engine8], currently
claimed by Claude in [PR #21][engine21], plus an exact usable, admitted SemEngine Tier 0 SHA. At planning
start, engine main `34c9dc64fd4467001c33686a776cbe6b7c6878f9` provides foundation/harness packages,
not the required production engine.
A planning PR, compiling reference binary or elapsed time cannot satisfy this gate.

SemSource owns composition, source adapters, consumer query policy and the acceptance workload.
SemEngine owns substrate APIs, extraction and package admission. This document supplies consumer
requirements to 03B; it does not choose missing engine APIs or duplicate that work.

## Current seams and required changes after admission

Code references identify the merged consumer layout. Their names describe current code, not promised
SemEngine types, flags, subjects or package paths.

| Seam | Current behavior | Required Tier 0 outcome |
| --- | --- | --- |
| [Composition][graph-compose] | BM25 and graph-embedding always enabled | No embedding component/provider |
| [Graph config][graph-config] | bm25/http; no disable selector | Explicit foundation composition |
| [Validation][provider-validation] | HTTP requires embedding capability | No foundation model prerequisite |
| [Registries][registries] | Full framework registrations | Only admitted engine dependencies |
| [Gateway][gateway] | Graph/index and agentic requester declarations | Only admitted foundation interfaces |
| [Fusion][fusion] | Requires NATS, body resolver and readiness | Exact lookup/body contracts without embedding |
| [Query verbs][verbs] | All code/doc verbs exposed in every profile | Explicitly unavailable unsupported retrieval |
| [Removal][remove] | Disabled envelope defeats file overlay | Preserve until #17 replacement is proven |
| [Corpus selector][selector] | Accepts only bm25/neural | Add an explicit foundation slice; preserve old profiles |
| [Corpus Stop][stop] | Joins child without asserting wait error | Checked exits and intentional crashes |

No current `embedder_type: none` or foundation-profile flag is claimed. Omitting a provider leaves the
BM25 embedding component composed. Turning off an optional component also cannot remove imports pulled
in by registries. [Graph transport][transport] currently uses explicit ingestion subjects, memory
storage, 256 MiB, one-hour retention and DiscardNew; preserve and record that comparison policy unless
03B explicitly admits a different contract. Persistent entity/content storage is a separate obligation.

## Contract inputs required from 03B

Before implementation, record approved decisions and their exact references:

- Composition/registration, component dependency injection, owned Start/Stop/drain semantics and the
  admitted production/test package boundary, including any reviewed bridge and its exit condition.
- Indexed exact entity/name/query operations, minimal deterministic fusion, body/reference retrieval,
  scope enforcement, and a public readiness contract that works without an embedding component.
- Effective authority and identity persistence, desired-versus-running config semantics, storage
  naming/access contracts, graph mutation authorization, retained-history behavior, and the distinction
  between accepted writes and applied/durable effects.
- #15 broker-generation repair, #16 reserved RPC/startup-refusal behavior, and #17 config overlay repair.
- #18 applied-input, #19 conditional-reconcile and #20 unknown-outcome decisions, including explicit
  defer consequences for #215. A new API alone does not qualify publication/withdrawal eligibility.
- Placement of the required SemConnect reference cases and engine dogfood workload. The current
  code/docs corpus does not supply those separate evidence obligations.

#18 is a contract request; #19 is a verified API/TOCTOU gap; #20 is a verified code-contract ambiguity.
They are not all reproduced live defects or commitments to implement a particular primitive. The
consumer retains safe refusal until the relevant contract and qualification are admitted.

## Foundation corpus partition

Keep the frozen fixtures and result ledgers unchanged. Add an explicit foundation selection and record
which assertions belong to each profile; do not make a failed assertion pass by skipping its work.
The selection mechanism is future consumer implementation, not an available command-line option.

| Evidence | Current location | Foundation treatment |
| --- | --- | --- |
| Delivery settled with zero loss | [ingestion oracle][ingestion] | Required; not sufficient alone for readiness |
| Exact IDs, relationships, provenance | [structural checks][structural] | Preserve expected answers |
| Exact code body | [body check][code-body] | Preserve exact-symbol lookup and verbatim bytes |
| Duplicate names / absence | [query checks][exact-queries] | Preserve deterministic public answers |
| Document parent/passage and exact bytes | [document check][doc-body] | Add admitted exact public retrieval path |
| Updates, delete/recreate, restart | [lifecycle][lifecycle] | Retain identity/history contract |
| Broker restart / re-ingestion | [broker sequence][broker] | Both relationship/content assertions must pass |
| NL scope/ranking / searchGraph | [NL][nl-checks], [searchGraph][search] | 04B lexical / 04C neural, per 03B |
| Paraphrase / provider faults | [neural][neural], [faults][provider-faults] | 04C; not claimed by 04A |

Concrete assertion mapping, with final engine operations supplied by 03B:

- `*_entity_*`, `*_persisted_authority`, `*_exact_relationship` and `*_provenance`: 04A exact public graph
  reads plus the admitted authority observation. Preserve IDs, references and source metadata.
- `*_exact_content`, `*_duplicate_name_anchors` and `*_exact_absence`: 04A deterministic lookup and
  public hydration. Preserve exact code bytes, both Greet anchors and a ready authoritative miss.
- `*_doc_exact_passage_content`: 04A exact document/passage retrieval through the newly admitted public
  path; its current NL selection stays in the 04B/04C profiles. Direct KV/object-store inspection cannot
  substitute for public body retrieval qualification.
- `deleted_retains_stale_history`, `deleted_query_visibility` and `recreated_same_identity_not_stale`:
  04A exact retained-state and admitted deterministic visibility checks after lifecycle changes.
- `*_scope_before_limit` and `graph_search_public_query`: retain in admitted 04B/04C retrieval lanes;
  04A must separately prove scope on its admitted exact operations, not advertise unsupported NL.
- `neural_paraphrase`, `provider_metrics_*` and provider fault/recovery assertions: 04C only.
- `broker_restart_reingested_exact_relationship` and `broker_restart_reingested_exact_content`:
  mandatory 04A public relationship/body answers after broker generation repair; never skipped.

The exact document body expectation already exists, but its current public selection path is NL
`doc-context/context` with a phrase query. There is no separately qualified public exact-document/body
path for Tier 0 today. Obtain the admitted operation from 03B, then test deterministic parent/passage
selection and exact bytes through it. Do not invent an engine endpoint, silently read a private store
instead, or mistake a bodyless metadata parent for a missing passage.

The code exact-body and absence checks also pass through fusion. [Exact symbol filtering][exact-seed]
provides consumer policy, but the [response oracle][response-oracle] currently requires an aggregate
bootstrap flag. The future foundation readiness oracle must use the admitted structural readiness
contract, distinguish unavailable/deferred from authoritative absence, and verify actual known answers.
It must neither wait for an absent embedder nor waive structural indexing completion.

The existing profile reports slices 0/1 for BM25 and 0/2 for neural. A future foundation report must
identify only its admitted capabilities. Preserve the old cold/warm lexical and neural cases for 04B
and 04C; do not relabel BM25 as a no-embedder proof.

## Repair and lifecycle acceptance boundaries

Tier 0 must pass `broker_restart_reingested_exact_relationship` and
`broker_restart_reingested_exact_content`. The reference known-at-pin classification does not carry
into engine qualification. Preserve the authentic acknowledged-but-unindexed setup, persistent
entity/content checks and changed-source re-ingestion oracle. The current [private guard capture][guard]
is diagnostic reference evidence, not an engine API or an applied-input completeness proof.

The [old collision probe][collision] deliberately succeeds when wildcard GRAPH steals an RPC and
returns PubAck. Preserve that historical reproduction, but qualify #16's admitted safe composition and
startup-refusal behavior with new engine assertions. Reproducing the hazard cannot count as engine repair.

Keep the disabled component envelope from [sourcespawn.Remove][remove] until #17's approved replacement
passes original-file and runtime-added remove/restart/re-add cases. Preserve truthful restart-required
receipts and unknown-handle/error semantics. [The existing real-manager regression][config-test] is a
starting behavior oracle, not permission to retain SemStreams test dependencies in the engine branch.

Reuse or adapt [checked process exit evidence][checked-stop] for the new lane: graceful shutdown must
exit zero, forced-crash cases must observe their expected signal, and every owned broker must be removed
by exact ID after logs are captured. Preserve deadlines, cancellation, drain/join and failed-start cleanup.
#215's stronger completion/reactivation claims remain bounded by the recorded 03B decisions.

## Implementation sequence after the gate

1. Freeze the approved contract matrix, package ledger and usable engine SHA. Create/update the separate
   qualification branch without changing mainline's SemStreams dependency.
2. Port consumer bindings and tests exclusively to admitted SemEngine contracts. The qualification
   binary must import no SemStreams packages, not merely avoid starting them. Prove production and
   tagged-test closures contain no SemStreams dependency or unadmitted higher-profile package.
3. Write failing composition, capability and readiness tests; implement the actual no-embedder profile
   with no embedding/model-service requirement. Retain approved storage, source and query responsibilities.
4. Write failing foundation corpus selection and exact-document retrieval tests, then implement the
   public adapters and explicit assertion partition while keeping historical profiles intact.
5. Qualify retained source configuration, lifecycle, both broker-reingestion assertions and #16's safe
   RPC boundary. Run admitted SemConnect/dogfood evidence as separately identified obligations.
6. Record exact source/binary/config/corpus identities, results, dependency costs and resource cleanup;
   obtain independent component and graph/event review before declaring Tier 0 qualified.

No implementation step above is admitted by this design alone. Missing or contradictory engine
contracts stop the dependent step and return to the owning 03B decision; the consumer must not supply
a private compatibility shim that invents the missing substrate behavior.

## Planned validation, not executed

These commands are future implementation checks. The engine module must already be admitted and all
referenced test fixtures adapted before running the complete tagged lane. They are not qualification
results, and no foundation-profile invocation exists yet.

```bash
go list -deps -json ./... > production-dependencies.json
go list -deps -test -tags=integration,e2e,qualification,removal,garage -json ./... > test-dependencies.json
go test -race -count=1 ./...
go test -race -tags=integration -count=1 ./...
task lint
task agents:check
openspec validate qualify-semengine-tier0 --strict
```

Dependency records must be checked for forbidden SemStreams packages and against the approved port
ledger; generating files alone is not a gate. Once implemented, record the exact foundation selector
and process command beside its result instead of publishing an invented invocation here. Required
behavior includes absent embedding composition/provider, truthful unavailable interfaces, structural
readiness, exact document bytes, restart recovery, config removal persistence and checked cleanup.

Evidence must distinguish the build's captured source revision from embedded binary metadata, record
the executable SHA, and name every failed or unavailable case. Passing Tier 0 unlocks only the next
approved slice. Mainline cutover still requires full semembed-backed 04C and retained lower-profile
regressions; no lexical, neural or generation claim is made by this plan.

## Planning review

Architect inventory and an independent Go/component reviewer approve this consumer plan. The reviewer
checked the public proof paths, exclusive dependency boundary, no-embedder readiness, restart/config
obligations, resource ownership and 03B stop point. Strict OpenSpec, diff and line-length checks pass.
The reviewer ran documentation checks only; passing CI on the unchanged SemStreams reference does not
qualify an engine workload. This review admits no implementation, engine API, Tier 0 result or
mainline substrate change.

[source221]: https://github.com/C360Studio/semsource/issues/221
[engine8]: https://github.com/C360Studio/semengine/issues/8
[engine9]: https://github.com/C360Studio/semengine/issues/9
[engine21]: https://github.com/C360Studio/semengine/pull/21
[graph-compose]: ../../../cmd/semsource/run.go#L732
[graph-config]: ../../../config/config.go#L32
[provider-validation]: ../../../config/config.go#L385
[registries]: ../../../cmd/semsource/run.go#L294
[gateway]: ../../../cmd/semsource/run.go#L879
[fusion]: ../../../processor/code-context/component.go#L193
[verbs]: ../../../processor/code-context/component.go#L64
[remove]: ../../../internal/sourcespawn/sourcespawn.go#L304
[selector]: ../../../test/setup03a/qualification_test.go#L92
[stop]: ../../../test/setup03a/qualification_test.go#L428
[transport]: ../../../cmd/semsource/run.go#L987
[ingestion]: ../../../test/setup03a/qualification_test.go#L131
[structural]: ../../../test/setup03a/qualification_test.go#L667
[code-body]: ../../../test/setup03a/qualification_test.go#L711
[exact-queries]: ../../../test/setup03a/qualification_test.go#L723
[doc-body]: ../../../test/setup03a/qualification_test.go#L774
[lifecycle]: ../../../test/setup03a/qualification_test.go#L197
[broker]: ../../../test/setup03a/qualification_test.go#L800
[nl-checks]: ../../../test/setup03a/qualification_test.go#L750
[search]: ../../../test/setup03a/qualification_test.go#L183
[neural]: ../../../test/setup03a/qualification_test.go#L164
[provider-faults]: ../../../test/setup03a/provider_test.go#L27
[exact-seed]: ../../../processor/code-context/exact_seed.go#L34
[response-oracle]: ../../../test/setup03a/qualification_test.go#L589
[guard]: ../../../test/setup03a/qualification_test.go#L1009
[collision]: ../../../test/setup03a/qualification_test.go#L929
[config-test]: ../../../internal/sourcespawn/removal_tombstone_integration_test.go#L18
[checked-stop]: ../../../test/setup03a/removal_replay_helpers_test.go#L77
