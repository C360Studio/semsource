# S3 / object-store source for document artifacts

Tracks [#197](https://github.com/C360Studio/semsource/issues/197).

## Why

An early adopter drops research-report artifacts into a [Garage](https://garagehq.deuxfleurs.fr/)
bucket and needs them ingested continuously into the governed graph. SemSource has no object-store
source: `url-source` polls a **static list** of URLs (`processor/url-source/config.go:19`) with no
bucket listing, prefix walk, or change signal. The only path that works today is syncing the bucket
to local disk with `rclone` and pointing a `docs` source at the result — a shim that owns neither
change detection nor entity identity, and that silently re-derives identity from a cache path that
has nothing to do with the artifact.

Now, because the work is **fully decoupled from the pending SemStreams tag**. The pinned
`v1.0.0-beta.161` already ships `storage.Store` (`Put`/`Get`/`List`/`Delete`) and
`StreamableStore.Open`, and the interface's own documentation names S3 as an example of a mutable
store. Nothing on this path waits for the next release.

## What Changes

- **New `storage/s3store`** implementing SemStreams `storage.Store` and `StreamableStore` against
  S3-compatible endpoints. Garage-first — custom endpoint URL, path-style addressing, dummy region —
  and AWS-compatible without a second code path. Sits beside `storage/filestore` as a peer.
- **New `s3` source type** — config entry, CLI registration, handler, and processor. Objects
  enumerate through `ListObjectsV2` under a configured bucket + key prefix.
- **Ingest stops being filesystem-bound.** `ingestFileEntityStates`
  (`handler/doc/entities.go:292`) reads the file (line 293) and builds entities in the same
  function. Split it into a content-taking seam so object bytes reach the existing document and
  passage pipeline directly. **No behavior change for the filesystem doc path** — same entities,
  same identities, same tests. This split is what avoids a local materialization cache and lets
  everything downstream (passages, embeddings, and later #198 and #201) work unchanged.
- **Entity identity derives from the object key**, used as the logical path — purely intrinsic,
  stable across re-ingest, and re-derivable on SEED. The bucket slug is the system segment; the
  existing `--project` override (`handler/doc/handler.go:60`) still applies. Object keys are
  arbitrary strings, so sanitization into valid 6-part-ID segments is a first-class requirement,
  not an afterthought.
- **Change detection by polling** — `ListObjectsV2` plus `HeadObject` ETag comparison at a
  configurable interval. The ETag and content hash ride as triples for change detection and are
  **never** part of identity, matching the stance the doc handler already takes
  (`handler/doc/entities.go`, `DocFileHash`).
- **Credentials come from the environment, never from config KV** — the same posture the MCP
  gateway already takes with `SEMSOURCE_API_TOKEN` (`processor/mcp-gateway/component.go:64`),
  which keeps the secret out of the watched configuration.
- **Per-source status** through `internal/sourcestatus.Report`, consistent with every other source.

## Capabilities

### New Capabilities

- `object-store-source`: ingesting document artifacts from an S3-compatible object store —
  connection and addressing compatibility, prefix enumeration, ETag-based change detection,
  object-key-derived entity identity, and the loud-skip contract for objects whose content type
  the document pipeline does not handle.

### Modified Capabilities

- `entity-identity-safety`: the requirement "Produced entity IDs satisfy the graph-ingest segment
  contract" gains a new identity source. Object keys are arbitrary UTF-8 with `/`, spaces, and
  characters that are not legal NATS KV key segments; the sanitization must be total and
  collision-free, not best-effort.
- `runtime-configuration`: three additions. A new source type in `semsource.json`
  (`config/source.go:10` `validSourceTypes`, and the type switch at `:209`); the requirement that
  object-store credentials resolve from the environment and are never persisted into the watched
  runtime configuration; and the promise `semsource validate` makes about source types — a type
  configuration accepts must be spawnable. That last one is a pre-existing gap this change
  surfaces rather than creates: `validSourceTypes` and `sourcespawn.buildSpecs` are two
  unconnected lists, and the map is unexported, so the agreement between them is unverifiable
  from any other package.
- `cli-onboarding`: `semsource add s3` joins the non-interactive registration set
  (`cli/add.go:83`), including the existing "declare explicit identity" requirement — a bucket has
  no repo to infer org or project from, so explicit identity matters more here than anywhere else.
- `typed-source-change-events`: the requirement currently reads "Doc **and URL** watch changes
  publish typed entity state". Object-store watch changes join that set, including the delete case —
  an object removed from the bucket must publish typed state, not vanish silently.

## Non-goals

- **Event-driven ingest (SQS/SNS/webhook).** Garage does not implement
  `PutBucketNotificationConfiguration` — it is listed as missing in their
  [S3 compatibility matrix](https://garagehq.deuxfleurs.fr/documentation/reference-manual/s3-compatibility/).
  Polling is the only mechanism available against the first target, so the notification path is out
  of scope entirely rather than half-built behind a flag.
- **Object version as entity version.** Garage has no object versioning (`GetBucketVersioning` is a
  stub returning "versioning not enabled"), so version-id cannot carry entity version. This closes
  one of the identity options floated in #197.
- **Writing to the bucket.** `s3store` will implement `Put` and `Delete` because `storage.Store`
  requires them, but no ingest path calls either. SemSource reads artifacts; it does not manage them.
- **PDF and other formats the document pipeline does not already handle** — that is #198. Objects
  whose content type is unsupported are reported on the status surface and skipped **loudly**, never
  ingested as empty documents.
- **Claim and citation extraction** — that is #201, and it is blocked on data shapes we do not have.
- **Public or multi-tenant exposure** of the resulting graph — that is #200.
- **Bucket lifecycle or retention management.** The graph stays retention-first (ADR-0008); nothing
  here introduces reference-blind eviction.

## Impact

**New dependency — decided: `minio-go`.** `go.mod` currently has no AWS, MinIO, or S3 dependency at
all. `minio-go` wins over `aws-sdk-go-v2` on the axes that matter here: one light package rather
than a multi-module SDK, built S3-compatible-first so Garage is a first-class target rather than a
deviation, and no threat to `go install`. Recorded in `design.md` rather than an ADR — the choice is
contained to one package behind the `storage.Store` interface, so it is reversible without a
cross-repo contract change.

**Ownership — decided: build here, revisit on a second consumer.** SemStreams ships
`storage/objectstore` (NATS) while SemSource ships `storage/filestore`, so a `Store` implementation
has precedent on both sides, and an S3-compatible one is plausibly framework-shaped. The call is to
build it in SemSource now and treat a **second sem\* service needing an artifact bucket** as the
trigger to reconsider promoting it upstream. That revisit condition is recorded as a triaged
candidate in `docs/upstream/semstreams-asks.md` so the question stays visible instead of quietly
becoming permanent by default. If it is ever promoted, that goes through a GitHub issue against
semstreams — never a PR.

**Code touched:** `storage/s3store` (new), `handler/doc/entities.go` (seam split, no behavior
change), the new source handler and `processor/s3-source` (new), `config/source.go` (`:10`, `:209`),
`cli/add.go` (`:83`), plus the README source-type table — which brings the
`advertised-surface-coverage` requirement for test evidence with it.

**Scale check before promising continuous ingest.** An artifact bucket only grows, and its growth
profile is unlike a repository's. Verify against #178 (GRAPH stream ceiling behavior) before this
is documented as unbounded-safe.

**Consumers:** no sem* product consumes this capability today — it is driven by an external
adopter. SemOps is the plausible first internal consumer, since report and artifact corpora fit its
COP semantics, but that is not committed and this change does not assume it.
