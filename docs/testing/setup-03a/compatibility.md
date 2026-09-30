# SETUP 03A compatibility and evidence contract

This is the migration ledger for [SemSource #212](https://github.com/C360Studio/semsource/issues/212)
and [SemEngine #7](https://github.com/C360Studio/semengine/issues/7). It records reviewed contracts and
pending evidence, not a statement that migration or SETUP 03A qualification has passed.

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
| 0: graph foundation | `configs/tiers/tier0-statistical.json`, structural operations | None; this is not a no-embedder composition |
| 1: lexical | `configs/tiers/tier0-statistical.json`, BM25 operations | None |
| 2: neural | `configs/tiers/tier1-semantic.json` and shipped semembed composition | Actual pinned semembed and model |
| Separately admitted generation | `tier2-semantic-instruct.json` or dev overlay | Separate generation qualification; not admitted by this migration |

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
ADR source links share this [pinned directory](https://github.com/C360Studio/semstreams/tree/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr).

| ADR | Retained obligation or disposition | SemSource proving surface |
| --- | --- | --- |
| 094 | Boot seals composition; component writes are desired-only, restart required; keep work live while draining | Root shutdown, source management/branch configuration, app restart |
| 095 | Exact native consumer ownership; one-shot running Stop; separate failed-Start cleanup; no normal topology deletion | Source callback/drain/join tests and process restart |
| 096 | Registry no longer gives out running component handles; ComponentManager owns concrete boot set | Root replaces post-Start instance discovery with pre-Start factory injection |
| 097 | Lesson contract snapshot supports local curator composition | No direct lesson curator in retained SemSource workload; transitive registry dependency only |
| 098 | Substrate health stays out of graph facts; freshness unknown is not ready | Status/readiness plus authoritative counts; capacity diagnostics use metrics/logs |
| 099 | Accepted target derives communities; selected implementation still contains LPA and edge controls | Preserve supported controls; record decision/implementation gap; no generation qualification |
| 100 | Static factory ports, boot validation, read-only composition; flow-builder/engine/flowstore retired | Registration port parity, root service config, startup tests |
| 101 | Reserved coordinator replies and durable terminal routing | No retained source/tool caller authors coordinator replies; do not admit agentic behavior |
| 102 | ID order becomes org.platform.system.domain.type.instance; local subject authority enforced | All builders, parsers, handler paths, scopes, semantic envelope and known-answer IDs |
| 103 | Payload registry owns type, floor, and contracts; unknown creates rejected | Explicit bootstrap registration and projection contract tests |
| 104 | Effective authority is framework-minted once and retained; fixtures must observe it | First boot and same-store restart identities; derive config bucket by org/stem |
| 105 | Loop tokens are opaque framework UUIDs, never consumer-authored | No loop-token authoring in retained workload; no new migration behavior admitted |
| 106 | Two-tier API surface freeze; sister migration is a compatibility gate | Exact pin and independent review; SemEngine relationship ruling remains SETUP 03B |
| 107 | Pragmatic closed predicate types; semantic-web spelling only at export edge | Replace array→json and float64→float; test manifest wire metadata changes |

Pinned `docs/operations/migration-restart-safe-nats-client.md` explicitly does **not** claim a new
Client Connect/Close protocol, exact native CLOSED observation, async-publish settlement during Close,
raw NATS-root retirement, or full controlled/dirty restart proof. Do not infer implementation from the
broader ADR-094 text. `ConsumeStreamWithConfig` returns the exact native `jetstream.ConsumeContext`;
the owner retains/drains/joins it. Client Close does not discover or stop its children.

## Concrete compatibility handoff

- Add `platform_id` as the declared stem (default `semsource`), validated at load; the effective
  value is read only after configuration Manager.Start. Sharing an environment label does not isolate
  deployments; use distinct stems for independent environments on one broker.
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
  Source add/remove and branch changes persist desired state and report restart required. Existing
  file watches remain live. Desired status must never masquerade as effective running status.
- The target fusion NL scope is prefix-only. Taxonomy moved to segment four. Enumerating complete
  concrete source×taxonomy prefixes before limiting is acceptable; broad deployment search followed
  by post-limit filtering is not. Dynamic source systems or incomplete enumeration retain a blocker.
- `projection.Contract.MessageType` is structured; payload registrations must bind floors/contracts at
  the one registry authority. Update tests against actual birth behavior, not just compilation.
- Declared datatype changes affect the published manifest even when Go signatures compile. Record
  framework metadata changes separately from SemSource's array→json declarations.

## Workload and durability decision

The versioned fixture is `test/setup03a/fixtures/v1`. Public HTTP/MCP and `graph.query.*` assertions
cover exact answers, duplicate-name anchors, scope before limits, provenance, bytes, update/remove
relationships, retained stale history, recreation, and application/broker restart. Raw KV supports
inspection, not substitute public assertions. Completion is bounded and semantic, never just startup.

For accepted but unfinished work, pause the application, publish an authentic fixture envelope with a
fresh idempotency key, observe acknowledgement and an unfinished graph effect, kill the process, and
restart the broker with its retained file store. Inspect the transport before re-ingestion. Default
memory GRAPH may lose accepted transport messages; acceptance is not a durability claim. Restart and
verify recovery from the changed source. If target storage differs, record intended policy and prove
that outcome rather than normalizing the difference away.

## Evidence ledger and unresolved holds

| Evidence | Before | After | Acceptance rule |
| --- | --- | --- | --- |
| Structural known answers | Pending | Pending | Exact entities/relationships/provenance/bytes and absence |
| BM25 and scoped mixed retrieval | Pending | Pending | Expected answers, scope before limit, stable acceptance |
| Real semembed cold/warm/recovery | Pending | Pending | Same provider identity, real inference, explicit failure/recovery |
| Updates/deletion/recreation | Pending | Pending | Recorded stale retention and identity convergence |
| Application restart | Pending | Pending | Exact same-store authority/content retained or re-ingested as declared |
| Persistent broker restart | Pending | Pending | Inspect ack/effect/transport and prove declared recovery |
| GRAPH #178 | Pending | Pending | OSH corpus tail or measured equivalent; tiny success is insufficient |
| RPC #1143 | Pending | Pending | Reproduce wildcard stream overlap against retained request paths |
| Dependency closure | Compile and consumer tests: 97 packages / 204,027 lines | Pending | Production plus test dependencies at both pins, same tag policy |
| Independent Go review | Pending | Pending | Separate code approval from complete baseline acceptance |

Difference entries must state expected/observed behavior, exact case/configuration, pinned contract or
reproduced defect, and resolution/hold. Unavailable runs are blocked evidence, never empty successes.
Physical deletion (#210) is separate from retained stale history. Full OSH capacity and RPC collision
claims require their named probes. Optional generation and SemConnect qualification remain later
admission work; no blank column here implicitly accepts them.

## Baseline dependency finding

At beta.161, the direct-import closure after excluding broad registries is 46 SemStreams packages /
82,443 raw non-test Go lines; adding composed graph processors and gateway gives 63 / 126,596.
The full consumer compile and consumer test closures both contain 97 / 204,027. Retaining the selected
upstream packages’ own tests is larger: `go list -deps -test` on the 28 port roots reaches 115 packages
/ 250,136 lines, 52 packages beyond the production port set. This is dependency evidence for SETUP 03B,
not approval to extract those additional packages. Record exclusions and their proving tests before
using the production closure ceiling to claim a smaller complete port.
