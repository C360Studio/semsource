# SemStreams beta.148 clean cutover

## Supported migration

This is a one-way clean beta cutover. It does not support aliases, translation, dual reads, dual writes,
mixed-version writers, in-place rewrites, or preservation of incompatible graph state. Authoritative source
inputs are the recovery authority.

Do not begin until the command sheet names the exact NATS context and account, the beta.148 release artifact,
the rendered `semsource.json`, every writer, and the maintenance-window reviewers. Rollback to beta.145 is valid
only before the first deletion. After any deletion, recover by forward reseed or by restoring separately validated
backup state; never reconnect beta.145 to partial or deleted state. Beta.148 does not read the old graph state.

## Prepare the reviewed command sheet

Render the deployment's real configuration, review it, and record its checksum. Replace the example paths and
context below with literal values before the sheet is approved; an approved sheet contains no placeholders or
unset variables.

Before deleting `semstreams_config`, compare its current desired-source inventory with the rendered
`semsource.json`. The reviewed file must contain every desired source currently represented in
`semstreams_config`; a missing source blocks the cutover.

```bash
nats context select <exact-cutover-context>
nats context info
nats account info
jq -S . /absolute/path/semsource.json
shasum -a 256 /absolute/path/semsource.json
semsource validate --config /absolute/path/semsource.json
```

Record the service-manager, container, or process commands that stop every SemSource and SemStreams graph writer
using that account. Execute those literal stop commands, then attach status and process evidence showing that no
writer remains. Do not infer writer shutdown from quiet logs.

Capture the account inventory after writers stop and before deletion:

```bash
nats context info
nats account info
nats kv ls
nats stream ls
nats object ls
```

Save the complete outputs with the command sheet. Resolve configured bucket or stream overrides from the rendered
`semsource.json`; an override replaces the default candidate and must be reviewed under its literal resolved name.

## Review the minimum deletion set

Create one row per candidate with literal name, resource kind, observed status, enabled status, delete or preserve
decision, reason, reviewer, and command output. Copy a command below into the executable sheet only when that exact
resource was observed and the corresponding component is enabled. Never paste the whole block into a shell, use a
wildcard, derive names at execution time, or delete an unobserved resource.

The default candidates are `semstreams_config`, the `GRAPH` stream, every beta.148
`graph.FrameworkOwnedBuckets()` value, the two graph-ingest guard buckets, and an observed legacy catalog:

```bash
# Candidates only: copy individually into the reviewed sheet after inventory resolution.
nats kv rm semstreams_config
nats stream rm GRAPH
nats kv rm ENTITY_STATES
nats kv rm PREDICATE_INDEX
nats kv rm INCOMING_INDEX
nats kv rm OUTGOING_INDEX
nats kv rm ALIAS_INDEX
nats kv rm NAME_INDEX
nats kv rm SPATIAL_INDEX
nats kv rm TEMPORAL_INDEX
nats kv rm TEMPORAL_INDEX_REVERSE
nats kv rm CONTEXT_INDEX
nats kv rm EMBEDDINGS_CACHE
nats kv rm EMBEDDING_INDEX
nats kv rm EMBEDDING_DEDUP
nats kv rm COMMUNITY_INDEX
nats kv rm ANOMALY_INDEX
nats kv rm STRUCTURAL_INDEX
nats kv rm ENTITY_SUFFIX_INDEX
nats kv rm GRAPH_INGEST_APPLIED_SEQ
# Legacy only: include only when this retired bucket was observed before cutover.
nats kv rm PREDICATE_CATALOG
```

`PREDICATE_CATALOG` is not a beta.148 framework bucket and a fresh deployment must not recreate it. Delete an old
default or override only when the pre-cutover inventory proves it exists.

Explicitly preserve:

- the authoritative repositories, documents, URLs, configuration files, and media inputs;
- source, content, and media stores resolved from the configuration;
- every ObjectStore;
- `COMPONENT_STATUS`; and
- every unrelated product, workflow, operational, KV, stream, and account resource.

After deletion, capture `nats kv ls`, `nats stream ls`, and `nats object ls` again. The evidence must show that only
approved rows disappeared and every preserved resource remains.

## Recreate, reseed, and prove the graph

Use the same reviewed `semsource.json` path and checksum. Start only writers whose recorded artifact or image digest
contains the beta.148 migration; do not admit an older writer during or after reseed.

```bash
semsource validate --config /absolute/path/semsource.json
semsource run --config /absolute/path/semsource.json
```

Record the exact product-specific start and reseed commands, source revisions, and input counts. Starting SemSource
performs the configured initial ingestion; any additional producer action must also appear literally in the sheet.
Poll the deployment's recorded status endpoint until `phase` is `ready`, and retain the response with its target and
indexed revisions.

Before the window, record one canonical known-answer predicate, entity ID, and expected object from an authoritative
input. Run the exact query before sign-off and repeat the identical query after reseed. For the default NATS query
surface, a canonical renamed predicate query has this shape:

```bash
nats request graph.index.query.predicate '{"predicate":"source.doc.file-path"}'
```

The post-cutover result must contain the recorded entity, whose stored triple has the canonical predicate and
expected object, and must contain no retired predicate identity. Repeat the inventory and confirm that
`PREDICATE_CATALOG` is absent.

The disposable real-NATS rehearsal evidence is produced by:

```bash
go test -tags=e2e -run TestE2E_Beta148CutoverRehearsal -count=1 -timeout=300s ./test/e2e/
```

Release sign-off must attach a passing result from that command. This runbook does not claim that the rehearsal has
passed.

## Downstream predicate handoff

No downstream repository was edited by this SemSource change. The ledger assigns these external owner blockers:

- SemSpec and SemDragon selectors: `source.doc.file_path` to `source.doc.file-path`.
- SemDragon producer: `source.doc.mime_type` to `source.doc.mime-type`.
- SemSpec exact server query: `source.git.commit.sha` to `source.git.commit-sha`.

All other rows in the 92-row migration ledger are SemSource-local surfaces.
