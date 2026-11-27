# SemSource beta.12 — consumer adoption note

Audience: SemTeams, SemDragon, and any deployment moving from `v1.0.0-beta.11` to `v1.0.0-beta.12` (SemEngine
`v0.1.0-alpha.8` plus two graph-index fixes, pinned at untagged commit `3d823c8`, #428). This is the delta since beta.11; the tag is tracked on #408. Coming from beta.10 or
earlier, read the [beta.11 adoption note](beta11-adoption-note.md) first (or the
[SemEngine adoption note](semengine-adoption-note.md) from beta.9): one reseed covers every break in the chain.

## What you must do

**1. Remove `streams` from `semsource.json` if it sets it.** The key is gone, and the strict decoder refuses a file
that still carries it (`semsource validate` prints):

```text
semsource: validation failed: config: parse JSON: json: unknown field "streams"
```

Replace it with `graph_stream` ([below](#graph-stream-posture)), or drop it to take the defaults. Run
`semsource validate` before you restart.

**2. Provision fresh storage and re-ingest.** Config-file entity IDs changed and a server that still holds the beta.11
`GRAPH` stream refuses boot. There is no alias, rewrite, or migration. For Compose:

```bash
docker compose down -v   # removes the nats-data volume
docker compose up -d     # fresh provision + reseed from source
```

Wait for `/source-manifest/status` → `phase: "ready"`, then `index.ready: true`, before the first query (`ready` does
not wait for entities still in flight; see the `ingestion-readiness` spec). Two reasons an in-place upgrade is not
enough:

1. Config-file dependency and image IDs changed ([next section](#entity-id-changes-since-beta11)). IDs are never
   aliased or rewritten, so beta.11's entities stay in `ENTITY_STATES` beside the new ones, as orphans.
2. A server still holding the beta.11 memory `GRAPH` stream refuses boot ([GRAPH stream](#graph-stream-posture)).

`nats stream rm GRAPH` clears only the second. It leaves the old IDs, so it is not a substitute for the reseed.

## Entity-ID changes since beta.11

Only config-file entities change. Code, doc, git, url and media IDs are the same as in beta.11. Anything that stores
or hard-codes a config dependency or image ID must re-resolve it after the reseed (#393/#422).

- **Dependency IDs no longer carry the version.** The instance is `hash(coordinate)-hash(declaring file)`: the
  coordinate is the Go module path, the npm package name, the Maven `groupId:artifactId` (plus `:type:classifier`
  when either is set), or the Gradle `group:name`. A version bump now updates the same entity in place instead of
  minting a new one. The old ID is never retracted, aliased or rewritten.
- **Image IDs no longer carry the tag or digest.** The instance is built from the name alone. A colon is a tag
  separator only after the last slash, so a registry port stays in the name (`registry:5000/team/app:1.0` is named
  `registry:5000/team/app`).
- **Several versions or tags in one manifest fold into one entity.** `source.config.dependency-version` and
  `source.config.image-name` are sorted multi-valued facts; each image reference (`golang:1.22`, `golang:1.23`) is one
  value of `source.config.image-name`, and the image's `dc.title` is the bare name. A consumer that read a single
  value must accept several.
- **A Gradle `:classifier` or `@ext` is part of the ID, not the version.** A plain jar and its `natives-*` variant stay
  two entities, as Maven's type and classifier already did. The classifier is `source.config.dependency-classifier`
  and the extension `source.config.dependency-type`; neither is in `dependency-version` any longer.
- **Dependencies carry their declaring manifest's absolute path** in `source.config.file-path`, as modules, packages,
  projects and images already did. The staleness pass judges them by it.

A restart does not reconcile a version that became `dependencyManagement`-managed while the process was down: a managed
version emits no fact, so the old value stays until the next known change to that manifest
(`// shortcut:` in `handler/cfgfile/entities.go`).

## GRAPH stream posture

The raw `streams` block is replaced by one setting. Everything else about the `GRAPH` stream is fixed for both
postures: the five `graph.ingest.*` subjects, workqueue retention, discard `new`, a one-hour age bound, one replica.

```json
{
  "namespace": "acme",
  "sources": [{"type": "ast", "path": "./", "language": "go"}],
  "graph_stream": {"posture": "durable", "max_bytes": 1073741824}
}
```

- **`posture`**: `durable` (the default) stores `GRAPH` on file, so a fact NATS accepted but graph-ingest has not yet
  applied survives a NATS restart. `ephemeral` keeps beta.11's memory storage, so a NATS restart loses those facts. An
  unknown value fails the load and names the posture.
- **`max_bytes`**: optional, positive, bounds the unapplied backlog. The default is 256 MiB. At the bound a publish is
  refused and retried within a bounded budget, then counted as a failure on the source's status; nothing is evicted
  ([entity-publish-integrity](../../openspec/specs/entity-publish-integrity/spec.md)).
- Durability is bounded by the one-hour age bound and needs NATS to keep its disk. The bundled Compose NATS keeps
  JetStream state on the `nats-data` volume, so `docker compose down -v` also clears `GRAPH`. `GRAPH` still holds
  transport only, never graph state; that lives in `ENTITY_STATES`.

**A server holding the beta.11 `GRAPH` stream refuses boot.** The memory stream's storage differs from the declared
file storage, and the server refuses a storage-type change on a live stream. SemEngine reports it as `reconcile stream
"GRAPH" failed: existing stream diverges in a field that cannot be reconciled in place`, naming
`storage: observed=memory declared=file`. The stream is left untouched. `docker compose down -v` (already required) or
`nats stream rm GRAPH` clears it. Switching posture later, in either direction, needs the same reset.

**Measured cost of `durable`** (#395/#423, one machine: M3 Pro arm64, Docker Desktop, fresh NATS per run,
`watch: false`). On an 8,439-entity self-seed (a `git archive` of `main`; ast go, doc and config sources;
`max_bytes` 2 GiB; three seeds per posture, alternating) the median drain was 13.75 s durable against 13.25 s
ephemeral: +0.5 s, +3.8%, inside the 15% or 2 s bar. The run-to-run spread (7.50–14.79 s durable, 9.57–19.47 s
ephemeral) is larger than the difference. All 8,439 entities landed in every run with no refusals, and peak `GRAPH`
size was about 400 KB in both. On a 1,041-entity fixture the roughly one-second ready tick dominates both postures
(1005 ms against 1004 ms), so it cannot separate them. This is one author-run measurement, not reproduced elsewhere.

## Fixes that need no action

None of these changes an ID or a config key.

- **#399**: a full `GRAPH` stream at boot no longer crash-loops. `source-manifest` retries its manifest and
  predicate-schema broadcasts in the background (500 ms backing off to 10 s) instead of failing `Start`. `phase:
  "ready"` does not wait for them.
- **#400**: a `repo` source's `index_interval` now reaches its `ast` child. It was dropped, so the 60 s default applied
  whenever `watch: true`.
- **#401**: the periodic reindex no longer rewrites unchanged folder and repo entities. A container whose statements,
  apart from `dc.terms.created`, equal the last this process published is skipped. Seed, watch and reconcile publishes
  still send, and the reindex still re-parses every file, so reparse CPU is unchanged.
- **#402**: the core image is also tagged with the `v` prefix, `ghcr.io/c360studio/semsource:v1.0.0-beta.12`, as the
  UI image already was. beta.11 is not retagged; the beta.12 tag run is the first to exercise it, and
  `docker buildx imagetools inspect` on both tags should resolve to one digest.
- **#405**: ADR-0003 no longer claims that runtime `add_source`/`remove_source` republish the manifest. It is
  published once, at start. Documentation only.
- **#406**: a stop during an index backlog exits 0 again. graph-index's `Stop` now drops queued index work instead of
  draining it (semengine#768, in the `3d823c8` pin, #428); the next boot replays the store anyway. Compose also sets
  `stop_grace_period: 40s` for `semsource`, so Docker's 10 s SIGKILL (exit 137) no longer pre-empts the process's
  own 30 s stop deadline.
- **#419**: the doc, config, image, video and audio watchers flush a path once it goes quiet for the debounce delay,
  or five delays after its first pending event. A burst no longer emits one event per tick boundary. A single save
  now lands about one delay later (500 ms by default). The ast watcher is separate and unchanged.
- **#424**: a config-file lifecycle pass that loses a compare-and-set to the source's own republish
  (`revision_mismatch`) re-runs, up to three attempts, instead of leaving a stale mark in place. Any other error
  returns at once.
- **#377**: `code_search`, `code_context` and `doc_context` expose the substrate's `merge` answer, and the tool
  descriptions name `merge.honoured`. A test pins it at Tier 1 (merged and provider down) and absent at Tier 0.

## Behavior you can observe

- **Deleting a manifest marks its entities stale** (#393/#422). Every entity it declared carries
  `entity.lifecycle.stale`, and re-creating the manifest clears the marker. This runs only for `watch: true` sources:
  a watching source also sweeps its roots once the seed settles, so a manifest deleted while SemSource was down is
  marked on the next start. A `watch: false` source is a frozen snapshot and never goes stale.
- **A dependency a manifest stops declaring is kept, and not marked stale.** Only the manifest's edge to it goes.
  Marking it is [#421](https://github.com/C360Studio/semsource/issues/421), which needs a design.
- **Fewer idle writes.** After the first reindex an idle tree no longer rewrites every folder and repo entity each
  `index_interval` (#401). Idle `ENTITY_STATES` write rate and CPU on the released image are measured under #407.

## Known open items

- **#403, pending re-measure** ([semengine#750](https://github.com/C360Studio/semengine/issues/750),
  [#765](https://github.com/C360Studio/semengine/issues/765)): at beta.11, after a warm restart of a large corpus,
  graph-index took about 45 minutes to reach `index.ready`, and slow-consumer drops appeared on KV-watch
  subscriptions. The pin carries semengine#772 (the follower watch carries keys only, so the boot replay should stay
  under nats.go's 64 MiB subscription buffer), but the restart has not been re-measured on it yet. The dogfood-host
  re-measure will be recorded on #403 before the tag. Until then, gate queries on `index.ready`.
- **Untagged SemEngine pin.** beta.12 builds on SemEngine commit `3d823c8` (`v0.1.0-alpha.8.0.20261126222333-3d823c823ed7`),
  not a tag. `go.sum` fixes it exactly; the next SemSource release moves to the first SemEngine tag that carries it.
- **#421**: a dependency a manifest stops declaring is not marked stale (above).
- **Post-tag measurements**: the scorecard regrade (#387) and the dogfood measurement on the released image (#407)
  are recorded after the tag.

Questions: file a SemSource issue that references #408.
