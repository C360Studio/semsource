# SETUP 03A compatibility and evidence contract

This is the migration ledger for [SemSource #212](https://github.com/C360Studio/semsource/issues/212)
and [SemEngine #7](https://github.com/C360Studio/semengine/issues/7). The migration is delivered in
[draft PR #213](https://github.com/C360Studio/semsource/pull/213). Executed comparisons and code changes
do not constitute SETUP 03A admission or merge
approval. [Review and remaining blockers](review.md) keep those decisions explicit.

## Immutable baselines

[Exact pins](pins.json) were recorded before the first neural run. SemSource starts at
`34bda6406fb06fd723a040988647b204204a1583`, SemStreams beta.161 at
`9d0ff67f377ea3dd82dca2f3bf614871c0100766`. beta.163 was absent at start, so the selected shared
migration/extraction baseline is main `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, Go version
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.

The migration uses SemStreams exclusively. Later SemEngine qualification uses its own exclusive
integration branch; mainline cutover waits for the full semembed workload and retained lower profiles.
A later upstream tag does not change this pin without a new explicit baseline decision.

## Profile crosswalk

| SemEngine qualification slice | SemSource configuration | Provider obligation |
| --- | --- | --- |
| 0: foundation | `tier0-statistical.json`, structural operations | None; embedding is still composed |
| 1: lexical | `configs/tiers/tier0-statistical.json`, BM25 operations | None |
| 2: neural | `configs/tiers/tier1-semantic.json` and shipped semembed composition | Actual pinned semembed and model |
| Generation, separate admission | `tier2-semantic-instruct.json` or dev overlay | Separate qualification |

The numbered tier2-semantic-instruct JSON leaves clustering off; tier2-compose-dev and its opt-in
overlay enable clustering/LLM. The JSON and selected capability contract govern, not tier labels.
Structural coverage under BM25 does not establish an actual no-embedder binary. SemEngine owns that
later slice, while this baseline preserves the structural behavior to compare with it.

The provider pin includes the image digest, build SHA, executable hash, boot-selected model, actual
artifact revision and ONNX hash, vector dimension 384, query prefix, raw document preprocessing, and
resource limits. The same provider instance serves both comparison points. Any identity change
invalidates comparison and requires fresh indexes/caches and both reruns. A request model label or a
successful HTTP response alone does not establish served model identity.

## ADR inventory at the exact pin

All entries below were read from the downloaded module at the exact selected SHA. Where an ADR's
aspiration differs from landed behavior, the current implementation and migration guide limit claims.
ADR sources share this [pinned directory][adrs]. The full commit is recorded in `pins.json`.

[adrs]: https://github.com/C360Studio/semstreams/tree/8b99efe9c66a/docs/adr

| ADR | Retained obligation or disposition | Proving surface |
| --- | --- | --- |
| 094 | Sealed boot; desired writes require restart; drain live work | Source management and restart |
| 095 | Native consumer ownership; one-shot Stop; failed-Start cleanup | Callback/drain/join tests |
| 096 | Registry removes running handles; manager owns concrete boot set | Factory injection before Start |
| 097 | Lesson curator contract | No direct retained caller; transitive dependency only |
| 098 | Health outside graph facts; unknown freshness is not ready | Counts/status, metrics/logs |
| 099 | Community retirement accepted; pinned code still has LPA/edge controls | Preserve supported controls |
| 100 | Static ports, boot validation, read-only composition; flows retired | Ports and startup tests |
| 101 | Reserved coordinator replies; durable terminal routing | No retained caller; agentic not admitted |
| 102 | org.platform.system.domain.type.instance; local subject authority | Builders, scopes, fixture IDs |
| 103 | Registry owns types, floors, contracts; rejects unknown creates | Birth/contract tests |
| 104 | Mint once, retain effective authority; bucket by declared org/stem | First boot and retained restart |
| 105 | Framework UUID loop tokens are opaque | No consumer authoring in retained workload |
| 106 | Two-tier API freeze; sister compatibility gate | Exact pin/review; SETUP 03B ruling pending |
| 107 | Closed predicate types; web spelling at export edge | array→json, float64→float metadata |

Pinned `docs/operations/migration-restart-safe-nats-client.md` explicitly does **not** claim a new
Client Connect/Close protocol, exact native CLOSED observation, async-publish settlement during Close,
raw NATS-root retirement, or full controlled/dirty restart proof. Do not infer implementation from the
broader ADR-094 text. `ConsumeStreamWithConfig` returns the exact native `jetstream.ConsumeContext`;
the owner retains/drains/joins it. Client Close does not discover or stop its children.

## Concrete compatibility handoff

- Add `platform_id` as the declared stem (default `semsource`), validated at load; the effective
  value is read only after configuration Manager.Start. Distinct stems separate retained configuration
  and identity, not the whole service: graph subjects and storage remain shared. Independent deployments
  require isolated broker/account infrastructure; same-broker multi-deployment operation is not qualified.
- Thread immutable `entityid.Authority{Org, Platform}` from effective component dependencies through
  parsers and handlers. Canonical builder arguments follow source/system before taxonomy/domain.
  `semsource` remains a provenance string, not an authority. Public-org overrides cannot bypass local
  subject authority; reject mismatches before ingest unless a distinct import contract is admitted.
- Provision fresh storage for the one-time identity break. No aliases, old-ID rewrite, or dual reader.
  Compare semantic fixture keys between revisions; within a revision assert exact unchanged IDs across
  retained-store restart. Observe `platform_identity` via `config.BucketName(org, stem)`.
- Replace post-start `GetManagedComponents` mutation with a product-owned registration/factory closure
  injecting manifest ingest dependencies before Start. The component owns the subscriptions it starts.
- Remove retired `watch_config` and flow-builder composition. Preserve clustering controls still supported
  by the selected implementation; ADR-099 retirement has not fully landed at this pin.
  Source add/remove requests persist desired configuration and report restart required; the current
  boot's manifest/status and producers stay unchanged. Existing file watches remain live. Dynamic
  branch discovery and remote submodule discovery are blocked with warnings; their changes are not
  automatically persisted for the next boot. Operators must pre-expand supported sources before boot.
  Removal persists a disabled component envelope rather than deleting its retained key. The upstream
  manager merges retained keys over the original file, so deleting a key lets that file source return
  on restart ([SemStreams #1443](https://github.com/C360Studio/semstreams/issues/1443)). The disabled
  envelope preserves removal; re-add enables it. Real-manager restart/re-add and retry tests pass,
  and the workaround has independent Go approval. The final executable removal supplement passes
  restart retirement and same-handle re-add; final primary executable comparisons are recorded below.
  The deferred `source_removed` graph projection after a desired removal is missing and blocks merge.
- The target fusion NL scope is prefix-only. Taxonomy moved to segment four. Enumerating complete
  concrete source×taxonomy prefixes before limiting is acceptable; broad deployment search followed
  by post-limit filtering is not. `sourceScopeSystems` reads all enabled, admitted sources from the
  effective persisted boot component snapshot, including sources registered for the next boot.
  AST project/version, docs/config paths/project, URL hostnames, and objectstore project/bucket/version
  feed complete sorted system lists. Code/docs taxonomy sets are applied before limiting. Runtime
  desired edits require restart; they do not silently expand the current boot's admitted scope.
- `projection.Contract.MessageType` is structured. The explicit boot payload registry binds graph
  floors/contracts and is passed through dependencies before component startup. Source-manifest
  payload categories use the pinned single-segment grammar. No post-bootstrap global registration
  substitutes for that authority. The broad built-in registries remain imported; this is not yet
  SemEngine's selective extraction boundary.
- Declared datatype changes affect the published manifest even when Go signatures compile. Record
  framework metadata changes separately from SemSource's array→json declarations.

## Workload and durability decision

The [versioned fixture and runner](../../../test/setup03a/README.md) use public HTTP fusion and
`graph.query.*` assertions. The workload does not claim a separate MCP transport run. Checks
cover exact answers, duplicate-name anchors, scope before limits, provenance, bytes, update/remove
relationships, retained stale history, recreation, and application/broker restart. Raw KV supports
inspection, not substitute public assertions. Completion is bounded and semantic, never just startup.

The qualification harness's `stop(false)` does not assert the child `Wait` error. Its restart report
therefore proves the checked semantic state after restart, not graceful application exit. Separate
passing E2E runs and the canonical capacity probes' `application_exit=0` provide the recorded exit
evidence. Do not infer complete shutdown correctness from the semantic report alone.

For accepted but unfinished work, pause the application, publish an authentic fixture envelope as a
new stream message, prove acknowledgement with no authority revision advancement, kill the process, and
restart the broker with its retained file store. Inspect the transport before re-ingestion. Default
memory GRAPH may lose accepted transport messages; acceptance is not a durability claim. Restart and
verify recovery from the changed source. If target storage differs, record intended policy and prove
that outcome rather than normalizing the difference away.

## Evidence ledger and unresolved holds

| Evidence | beta.161 | Reviewed pin | Acceptance rule |
| --- | --- | --- | --- |
| Structural known answers | Pass | Pass | Exact entities, calls, provenance, bytes, absence |
| BM25/scoped retrieval | Pass, cold/warm | Pass, cold/warm | Scope before one-node cap |
| Real semembed | Pass; request counters advance | Pass; counters advance | Same served identity |
| Provider faults/recovery | 6/6 pass | 6/6 pass | Error/deadline is not absence; recovery exact |
| Update/delete/recreate | Pass | Pass | Retained stale history; same ID returns live |
| Application restart | Pass | Pass | Same-store answers, content, authority |
| Broker restart | KV/body survive; recovery fails | Same failure | Recover changed source |
| GRAPH #178 | Guarded transport completes | Guarded transport completes | Historical volume remains open |
| RPC #1143 | Collision reproduced | Collision reproduced | Explicit subjects work; wildcard unsafe |
| Consumer closure | 97 packages / 204,027 lines | 100 / 214,818 | Include upstream tests separately |
| Independent Go review | Evidence approved | Scoped draft/evidence approvals | Full merge/admission withheld |

Difference entries must state expected/observed behavior, exact case/configuration, pinned contract or
reproduced defect, and resolution/hold. Unavailable runs are blocked evidence, never empty successes.
Physical deletion (#210) is separate from retained stale history. Full OSH capacity and RPC collision
claims require their named probes. Optional generation and SemConnect qualification remain later
admission work; no blank column here implicitly accepts them.

## Dependency closure at both pins

The versioned `scripts/setup03a/dependency-closure.py` measures unique upstream source directories,
raw non-test Go lines, the direct-import/registration cut, and both root-only and all-retained-package
test closures. Consumer and retained tagged tests use `integration,e2e,qualification`. These counts
were measured on Darwin arm64, Go 1.26.4, CGO enabled. Test-only dependency resolution uses an
evidence-local modfile; SemSource's module files remain unchanged.

| Scope | beta.161 packages / lines | Pinned packages / lines |
| --- | --- | --- |
| Consumer production | 97 / 204,027 | 100 / 214,818 |
| Consumer tests, tagged | 97 / 204,027 | 100 / 214,818 |
| Direct imports without broad registries | 46 / 82,443 | 48 / 81,687 |
| Proposed production port set | 63 / 126,596 | 65 / 126,926 |
| Port roots plus their own tests | 108 / 218,089 | 112 / 229,805 |
| Port roots plus tagged tests | 110 / 218,808 | 112 / 229,805 |
| Every retained production package plus its tests | 109 / 218,176 | 114 / 232,126 |
| Every retained production package plus tagged tests | 110 / 218,808 | 114 / 232,126 |

[Package lists and exact commands](dependency-results.json) make the comparison reproducible.
The earlier test total of 115 / 250,136 double-counted seven synthetic external-test variants;
it is superseded by the unique-directory counts above. This correction does not remove their
imports from the closure. The pinned test graph adds `pgregory.net/rapid v1.3.0` as required by
upstream tests. No test package is approved for extraction merely by appearing in this report.
The 65-package production port set exceeds the plan's 63-package ceiling and needs SETUP 03B
architect review. Root-package tagged tests add another 47 directories, while testing every retained
production package expands the pinned total to 114 packages / 232,126 lines, 49 beyond production.
The wider scope additionally pulls in `frameworkadapters/otel` and `output/otel`. The earlier 112
count remains valid for root-package tests, but is not the complete retained-package test cost.
SemSource still imports the full registries; replacing them is a separately reviewed extraction
boundary decision.

## Recorded beta.161 results

[Machine-readable results](baseline-results.json) retain every assertion name, failed expectation,
exact binary/corpus hashes, provider counters, and restart evidence. The same original beta.161 binary
was used for the initial runs and final evidence refinements:
`16962d42348196f4e1260d62e45a816abba1508677eecc1a63a8a31874068b10`.
The version-1 corpus hash is
`9724050f1ebdc0623460557bd4bb5c73c9b2591b4032737b79f9e7db821b0258`.

The final BM25 workload passed 55 of 57 checks; neural passed 59 of 61. Both failures are the same
reproduced recovery defect. After the memory GRAPH stream disappears across a broker restart, durable
`GRAPH_INGEST_APPLIED_SEQ` remembers Run's prior sequence 33. Re-ingestion publishes changed Run at
new stream sequences below 33, so the guard treats the new generation as stale. The old Farewell call
and body remain current although the source now calls Greet. The entity KV revision and exact stored
body survived **before** the application restarted; persistent state loss does not explain this result.
Automated `restart-guard.json` evidence records each run's concrete sequences. This remains a defect,
not an accepted baseline behavior or a waived assertion.

The neural supplemental run passed all six checks: initial real-provider passage, injected HTTP 503
classified as `dependency_unavailable`, a five-second caller deadline with no absence result, exact
passage recovery, and before/after request metrics. The fault proxy used the same underlying semembed
artifact. It changed transport availability only. A caller deadline does not prove every upstream
cancellation or native client lifecycle invariant named aspirationally in ADR-094.

A one-result docs query can return a metadata parent with its passage relationship and no body. The
separate content check retrieves the passage with a wider cap and compares exact bytes, including the
terminal newline. This follows the documented parent/passage distinction; the early harness's demand
for a body on the metadata parent was corrected before retained comparison and is not a product defect.

## Final executable comparison

The [pinned result ledger](pinned-results.json) identifies the final binary and authoritative run
folders. The final implementation includes the reviewed disabled-envelope workaround for #1443;
earlier executable comparisons remain provenance only. Every beta.161 assertion remains. The target
adds six authority assertions: canonical ID mapping plus the retained platform record at initial
ingest, edit, recreation, application restart, and broker restart. These prove the intended ADR-102/104
identity change without relaxing any answer, provenance, content, or recovery expectation.

The final binary is
`7847df902bef611c777df4a19366dac896a9e86cc80cd475623d40d679c0a198`.
BM25 passes 61 of 63 checks; neural passes 65 of 67. Only the two already reproduced broker-recovery
expectations fail. Both retain applied sequence 33 versus a restarted stream ending at 20. Current
Run payloads appear at 9 and 12 for BM25, and 8 and 12 for neural. The Run→Greet relation and exact
body remain stale despite authority/content surviving before application restart. This is the same
generation defect as beta.161, not an intended canonical-identity difference.

All six neural fault-supplement checks pass. Request counters advance 216→230→233 during the primary
neural run and 204→216 during its supplement. Final provider verification confirms the same healthy
container, start time, zero restarts, image, executable, boot model, artifact revision, ONNX hash, and
resource limits. Dimensions and preprocessing remain unchanged. `pins.json` retains the verification;
no successful fallback substitutes for real semembed use.

The raw deadline response differs: beta.161 reaches its caller context deadline at about five seconds;
the final target returns HTTP 504 `upstream_timeout` at 5,001 ms. This is a timing-dependent error
surface at the shared deadline boundary. Both satisfy the same bounded, non-absence expectation;
the raw errors are not claimed identical.

The final removal/re-add supplement passes 9 of 10 checks. The original JSON source retires after
restart, and re-adding its same handle becomes live only after the next restart. This closes the
#1443 resurrection regression through the disabled-envelope workaround. The retained document parent
and passage still lack `source_removed`, and no automatic lifecycle RPC is captured. The beta.161
supplement also lacks those markers: this is a pre-existing removal-projection defect, and durable
replay after desired retirement remains absent. It is separate from watched-file stale history.

## Capacity evidence and limits

The [reproducible capacity probe](../../../scripts/setup03a/README.md) consumes OSH commit
`235c0eabf24b6d6137b499b4402943d2794b70e6` with the same six-language AST, docs, config, and BM25
composition on both revisions. beta.161 accounted for 32,720 offered/delivered source messages with
zero source loss/errors in 60.93 seconds. GRAPH held 32,726 messages and 239,366,950 bytes beneath its
268,435,456-byte ceiling. The application and owned-container cleanup both exited zero.

The canonical target delivered the same 32,720 offers with zero source loss/errors in 61.04 seconds.
GRAPH held 32,726 messages and 244,999,958 bytes; application and container cleanup both exited zero.
Both runs use the versioned probe, identical corpus/path/language inputs, BM25 settings, and declared
or default stem `semsource`. The shared corpus tree SHA-256 is
`eb10388f5b6d710c4dac7e156823cacca5370ba856bb97233803d6f61bdd62c9`.
The target's mandatory minted authority follows ADR-102/104. Its 5,633,008 additional GRAPH bytes are
observed alongside this wire-identity change; no exact per-field decomposition or performance
benchmark is claimed.

The preserved beta.161 binary was rerun for this canonical pair; its original pre-upgrade capacity
record is retained separately. An earlier target run used longer stem `setup03a-capacity`; it is
retained as diagnostic evidence and excluded from canonical paired-byte comparison.

These are transport results. At each terminal point, structural indexing was degraded/behind and
embedding was still building; this scale run does not qualify query or neural completion. The existing
#175 minified-asset/symbol guard reduced the historical #178 workload from 77,802 to 32,720 offers.
The source corpus was not manually trimmed. The guarded run does not establish that the historical
capacity regression is fixed. The earlier scratch attempt stopped at source phase `ready` before
all offers were delivered and is excluded. The canonical folders are `capacity-beta161-reviewed` and
`capacity-pinned-complete`; the earlier valid baseline remains under `capacity-beta161-complete`.

## Authority bounds and remaining admission

`entityid.Authority` validates the organization plus effective platform length against a 141-byte
combined budget. The consumer reserves 80 bytes for system, 10 for domain, 12 for type, five separators,
and eight hash bytes within the 256-byte ID ceiling. Declared organization plus platform stem must
fit 134 bytes, reserving the framework's seven-byte minted suffix. This is an explicit early rejection,
not truncation of an authority segment.

Final executable results and independent reviewer dispositions are recorded in separate ledgers.
Production, root-test, and all-retained-package test measurements are complete. The 65-package
production port set and 114-package complete test closure need SETUP 03B boundary review.
Physical purge (#210), generation/clustering qualification, SemConnect cases, and a SemEngine-only
binary remain outside this migration's admitted workload. SemSource stays on SemStreams;
the later SemEngine-only branch must pass the full semembed-backed workload before mainline cutover.

## Reproduced framework recovery defect

The final BM25 and real semembed runs reproduce beta.161's same two broker-recovery failures;
[SemStreams #1442](https://github.com/C360Studio/semstreams/issues/1442) records the exact guard
mechanism. Memory GRAPH disappearing at broker restart is expected. The defect is retained applied
sequence state suppressing valid re-ingestion from the current source. Neither the upstream pin nor
the broker storage policy was changed to hide this result. The failed assertions remain in both
result ledgers and block qualification.

## Difference attribution

| Change or observation | Attribution | Evidence or hold |
| --- | --- | --- |
| Taxonomy moves to segment four | Intended ADR-102 | Exact IDs and cross-domain scopes |
| Effective platform minted and retained | Intended ADR-104 | Config authority and restart |
| Dependencies injected before Start | Intended ADR-096/100 | Wiring tests and real startup |
| Source add/remove changes next boot only | Intended ADR-094/100 | Desired/live separation tests |
| Dynamic branch discovery blocked | Unsupported migration path | Warning; pre-expand boot sources |
| Remote submodule discovery blocked | Unsupported migration path | Warning; pre-expand boot sources |
| Explicit boot payload registry | Intended ADR-103 | Registry tests and entity birth |
| Datatypes/category grammar change | Intended ADR-103/107 | Schema/manifest tests and startup |
| Runtime context preserved until drain | Intended ADR-094/095 | Join and settlement tests |
| Desired removal lacks source_removed replay | Pre-existing marker failure; replay still absent | Merge held |
| Removed boot-file source reappears | Upstream boot merge gap #1443 | Disabled-entry workaround approved |
| Incomplete enumeration/mutation errors | SemSource replay defect | Must support trustworthy replay |
| Recreated GRAPH conflicts with retained guard | Upstream #1442, both pins | Two recovery failures |
| Wildcard GRAPH captures RPC requests | Upstream #1143, both pins | PubAck; explicit subjects work |
| Guarded OSH fits GRAPH ceiling | Limited workload evidence | Historical #178 unresolved |
| GRAPH bytes increase with governed IDs | Observed with mandated wire-ID change | No per-field/performance claim |
| Deadline returns HTTP 504 vs caller timeout | Shared deadline boundary timing | Same bounded non-absence oracle |

The `source_removed` hold is distinct from the corpus's watched-file deletion, which marks stale
history and successfully recreates the entity. Desired source deregistration must defer graph effects
until the old producer is retired, then durably replay the removal projection after restart. The
current implementation persists the desired component/manifest deletion but has no such replay.
Retained entities can therefore continue to look current. No independent review has approved that gap.
