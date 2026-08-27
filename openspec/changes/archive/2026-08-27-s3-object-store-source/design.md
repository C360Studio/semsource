## Context

See `proposal.md` — Why. The constraints that shape the approach:

- **The substrate already has the interfaces.** `storage.Store` (`Put`/`Get`/`List`/`Delete`) and
  `StreamableStore.Open` ship in the pinned `v1.0.0-beta.161`, and SemStreams itself ships
  `storage/objectstore` (NATS) while SemSource ships `storage/filestore`. Nothing here waits on a tag.
- **The document pipeline is welded to the filesystem in exactly one place.**
  `ingestFileEntityStates` (`handler/doc/entities.go:292`) calls `os.ReadFile` on line 293 and then
  builds the parent entity and its passages from `content`, a root-relative path, and the file
  extension. Everything after the read is filesystem-agnostic already.
- **The extension gate lives in the walk, not in the ingest function.** `IngestEntityStates` skips
  non-document extensions by returning `nil` from `filepath.Walk` (`entities.go:262`) — silently,
  which is right for a repository full of `.go` files and wrong for a bucket that is supposed to
  contain documents.
- **`ErrBodyStoreRequired` already encodes the distinction we need.** An unreadable file is one
  document's problem (skip); an unavailable body store is the deployment's problem (abort). The new
  source must preserve that split rather than invent its own.
- **`entityid.SanitizeInstance` already handles hostile input** — disallowed runes to `-`, collapsed
  separators, and overlong input truncated with a content-hash suffix computed from the *original*
  string, so two long keys differing only past the cut stay distinct.

## Goals / Non-Goals

**Goals:**

- Object bytes reach the existing document/passage pipeline without ever touching local disk as a
  cache, so identity cannot accidentally depend on where a fetch landed.
- One source type serves Garage, MinIO, and AWS without branching.
- The retraction-safety contract in `specs/typed-source-change-events` is structurally enforced, not
  merely tested — a pass that did not complete must not be *able* to reach the retraction path.

**Non-Goals** (design-level, beyond the proposal's scope section):

- No persistence of enumeration state across restarts. See D5.
- No streaming parse. Document bodies are read whole, as they are today; passage splitting needs the
  full document anyway.
- No parallel object fetching in the first pass. Correctness of the completeness gate first;
  throughput is a measurement question, not a design assumption.

## Decisions

### D1: `minio-go` over `aws-sdk-go-v2`

One package rather than a multi-module SDK, built S3-compatible-first so Garage is a first-class
target instead of a deviation, and no threat to `go install`. `go.mod` gains one dependency tree
rather than several.

**Alternative considered:** `aws-sdk-go-v2` — official, and its credential chain (IMDS, IRSA, SSO,
profile files) is richer and more battle-tested than minio-go's. Rejected because the first and only
target is a self-hosted Garage bucket reached with static credentials from the environment, and the
AWS credential chain solves a problem nobody has yet. **This is the reversal cost to accept
knowingly:** if AWS IRSA or SSO becomes a requirement, minio-go's `credentials` package can cover it
but with less mileage behind it, and switching clients means rewriting `storage/s3store` — though
nothing above the `storage.Store` interface would change, which is what keeps the cost bounded.

### D2: Build in SemSource; a second consumer triggers the promotion question

An S3-compatible `Store` is plausibly framework-shaped. The decision is to build it here now and
record the revisit trigger — **a second sem\* service needing an artifact bucket** — as a triaged
candidate in `docs/upstream/semstreams-asks.md`, so the question stays visible instead of becoming
permanent by default. Promotion, if it ever happens, goes through a GitHub issue against semstreams,
never a PR.

### D3: A content seam, not a local materialization cache

Split `ingestFileEntityStates` into a content-taking method the doc handler exports, taking
`(content []byte, logicalPath string, system, org string, now time.Time)`. The filesystem path keeps
its current behavior exactly: read the file, compute the root-relative path, call the seam. The
object-store source calls the same seam with the object key as `logicalPath`.

The logical path is **slash-normalized at the boundary** (`filepath.ToSlash` on the filesystem side;
object keys are already slash-delimited), and the seam derives extension, MIME, and title-fallback
from it with `path.*` rather than `filepath.*`. Without that, the two callers would disagree on any
platform where the separator is not `/`.

**Alternative considered:** materialize objects into a local cache directory and point the existing
doc walk at it. Rejected on two counts — identity would derive from the cache path, which is exactly
what `specs/entity-identity-safety` now forbids, and it reimplements the `rclone` shim inside the
product while inheriting its cache lifecycle, eviction, and double-disk-usage problems.

### D4: A peer source type that depends on the doc handler's seam

`handler/objectstore` + `processor/objectstore-source` as a peer source type, owning enumeration,
change detection, skip accounting, and status. It constructs a `doc.Handler` (with the body store)
and calls the exported content seam for each object it decides to ingest.

**Alternative considered:** teach `handler/doc` to read from a `storage.Store` directly. Rejected —
it would give the doc handler two ingestion modes, two watch mechanisms, and two failure taxonomies
inside one type, and the doc handler's job is documents, not transports.

### D5: Enumeration state is in-memory; a restart costs one redundant pass

The source holds a `key → ETag` map for the current prefix, rebuilt by the first pass after start.
A restart therefore re-ingests every object once. That is idempotent by construction: same object
key → same entity ID, per-predicate replace, same content hash. It is also exactly the project's
existing stance that **initial seeding is the first pass of the continuous event loop**, not a
separate batch mode.

**Alternative considered:** persist the map in KV, or reconstruct prior state by querying existing
entities' content-hash triples from the graph. The KV option adds a second source of truth that can
disagree with the bucket; the graph-query option couples ingest startup to query readiness, which
inverts the readiness gate. Both are deferrable without changing the specs — recorded as an open
question rather than built.

### D6: The listing's ETag drives change detection; `HeadObject` is the exception, not the rule

`ListObjectsV2` already returns key, size, last-modified, **and ETag** for every object, so a full
enumeration pass has everything change detection needs in one round trip per page. `HeadObject`
is reserved for the case where content type must be resolved and the key carries no usable
extension.

**ETag is a change signal, never a content hash.** For multipart uploads, S3 and Garage both return
a composite `<hash>-<partcount>` that is not the MD5 of the object. The `DocFileHash` triple must be
computed from the fetched bytes with the doc handler's existing `contentHash`, exactly as the
filesystem path does. Using the ETag there would produce a hash that changes when an object is
re-uploaded with identical content by a different multipart chunking.

### D7: Identity — bucket slug as system, object key as instance

`system` = the configured project override, else the bucket slug through `entityid.SystemSlug`.
`instance` = `entityid.SanitizeInstance(objectKey)`. Both are existing, tested functions; the
hostile-input cases in `specs/entity-identity-safety` are satisfied by machinery already in place,
which is the point — the requirement exists to pin the behavior, not to build new sanitization.

### D8: The skip gate moves to the source and becomes loud

The object-store source applies the document-extension check itself, **before fetching the body**, so
an unsupported object costs one listing entry rather than a download. Every skip is counted with a
reason into `internal/sourcestatus.Report`. This is a deliberate divergence from the filesystem walk,
which skips silently: a `.go` file in a repository is not a failure, but an unparseable artifact in a
bucket of reports is something an operator needs to see.

### D9: Completeness is a value the retraction path requires, not a flag it consults

The enumeration pass returns a result type that carries the observed key set **only** when the
listing ran to completion across every continuation page. A failed or partial pass returns an error
and no key set. The retraction path takes the key set as its input, so there is no code path in which
a partial listing can reach it — the failure mode the spec calls out is prevented by the signature
rather than by remembering to check a boolean.

### D10: MinIO gates every PR; Garage compatibility is a tracked follow-up

Three test layers, deliberately split by what each can actually prove:

1. **No container** — the completeness gate, ETag map, skip accounting, retraction safety, and
   identity all test against a fake implementing the source's own enumerate/fetch interface. The
   retraction-safety cases need failure *injected mid-pagination*, which is more deterministic
   against a fake than against any real server. This is where most of groups 3 and 4 verify.
2. **MinIO container** — proves `storage/s3store` speaks S3 on the wire: path-style addressing,
   pagination continuation, ETag parsing. One container, env-var credentials, S3 API live on start,
   bucket created through the S3 API itself. Cheap enough to gate every PR, and consistent with the
   existing house pattern — `natsclient.NewTestClient` already self-provisions NATS via
   testcontainers (`.github/workflows/ci.yml:37-42`).
3. **Garage** — deferred to #202.

**The honest cost:** this design is Garage-first, and MinIO passing does not prove Garage works. That
claim currently rests on Garage's published compatibility matrix and the adopter's own usage rather
than on our test evidence. #202 exists to close that, and carries the bootstrap trap with it: Garage's
S3 API is unusable until `garage layout apply` completes, so a testcontainers wait strategy keyed on
port availability passes too early and yields intermittent auth failures that read as client bugs.

**Alternative considered:** `gofakes3` in-process, avoiding a container entirely. Rejected because
composite multipart ETags are exactly the behavior D6 singles out as dangerous, and an in-process
fake is the least likely to be faithful there — it would produce confidence on the one property most
in need of real verification.

### D11: A HEAD 404 is ambiguous, so the store asks a second question

Found while implementing group 2, and worth recording because the code that prevents it looks like
an unnecessary round trip to anyone reading it later.

`storage.Store` promises a not-found sentinel, and the client resolves a single object with `HEAD`.
A `HEAD` response carries no body, so a 404 arrives with nothing in it to say *what* was missing, and
minio-go fills in `NoSuchKey` because an object is what it asked for. A wrong bucket therefore
answers `NoSuchKey` for every key in it — the exact shape of the risk this design already names, where
a deployment fault reads as a corpus that is legitimately empty.

The store resolves it by consulting bucket existence when, and only when, a `HEAD` 404 arrives, and by
routing every other operation through the error body the store does send. `Get` issues a plain `GET`
rather than reusing the `HEAD` path, so the ingest path pays one round trip per document rather than
two. When the bucket check is itself inconclusive — a store answering with a denial rather than a
verdict — the sentinel stands: inventing a bucket fault from a permission error trades one wrong
answer for another.

### D12: Unbounded-prefix measurement — what it establishes, and what it does not

Task 7.5 exists so that nothing calls a whole-bucket ingest safe on the strength of nobody having
tried it. An artifact bucket differs from a repository in the way that matters: it grows without
anyone deciding to grow it, and #178 records the GRAPH stream refusing the tail of a large corpus.

Measured by `TestIntegration_UnboundedPrefixIngestMeasurement` (`internal/governance`, MinIO-backed,
empty prefix — the whole bucket):

| Figure | Value |
| --- | --- |
| Documents | 1000 |
| Body size | 2069 bytes each |
| Entities produced | 4000 (one parent plus three passages per document) |
| Offered / delivered | 4000 / 4000 |
| Lost | 0 |
| Seed loss | 0 |
| Backpressure observed | no |
| Ingest duration | 2.7s to `phase: ready` |

**What this establishes:** a whole-bucket ingest at this size completes, publishes every document,
and loses nothing. No ceiling was reached and no prefix-scoping guidance is required at this scale.

**What it does not establish:** a safe upper bound. It is one corpus at one size against a test
stream whose bounds are the test helper's (64MiB), not production's 256MiB. The #178 behavior
appears at a corpus tail far above this, so this measurement rules the ceiling *out* at 1000
documents rather than locating it. An adopter with a bucket an order of magnitude larger is outside
what has been measured, and prefix scoping remains the operator-side mitigation.

The test asserts only that nothing was lost. That is deliberate: a measurement that tolerated
silent loss would be evidence for the wrong conclusion, and a measurement that asserted a duration
would be a flake.

## Risks / Trade-offs

- **A transient listing failure retracts the entire corpus** → D9 makes it unrepresentable; tested by
  injecting a listing error mid-pagination and asserting zero staleness publications.
- **An expired credential looks identical to an emptied bucket** → same gate: authentication failure
  is an error, not an empty result. Covered by its own scenario in the spec.
- **ETag semantics vary by store and upload method** → treated as an opaque change token only (D6);
  the content hash is always computed from the bytes.
- **An artifact bucket grows without bound, unlike a repository** → verify against #178 before
  documenting this as unbounded-safe. Prefix-scoping is the operator-side mitigation available today.
- **The Garage-first claim is not backed by our own test evidence until #202 lands** → the
  compatibility matrix and the adopter's usage carry it in the meantime; the specific behaviors to
  verify are enumerated in that issue rather than left to whoever picks it up.
- **New integration tests silently never run in CI** → `.github/workflows/ci.yml:42` runs a
  hand-maintained package allowlist, not `./...`. Task 7.4 adds the new packages; this is the same
  two-lists-no-cross-check shape group 5 fixes for source types, one layer up.
- **`minio-go`'s credential chain is thinner than the AWS SDK's** → env-supplied static credentials
  are the documented and specified path (`specs/runtime-configuration`); revisit only if an AWS
  deployment needs IRSA.
- **A restart re-ingests every object once** → idempotent, and consistent with SEED being the first
  pass of the loop (D5). The cost is bandwidth on restart, which is bounded by prefix size.
- **Two callers of the content seam can drift** → the filesystem caller keeps its existing tests
  unchanged; a shared table test drives both callers over the same fixtures to assert identical
  entity output for identical bytes.

## Migration Plan

Purely additive. No existing configuration changes shape, no stored data changes, and **no reseed is
required** — deployments that add no object-store source see byte-identical behavior. The capability
activates only when an operator adds a source entry.

Rollback is source removal through the existing lifecycle path (`specs/source-lifecycle` —
"Removal is real and observable"), which retracts the corpus the same way any other source removal
does. Reverting the dependency means dropping `storage/s3store` and the new packages; the content
seam split is behavior-preserving and can stay.

## Open Questions

- **Persisting enumeration state across restarts** (D5). Deferrable — it changes only how much work a
  restart repeats, not the specs, the approach, or the task breakdown.
- **Content-type-based dispatch instead of extension-based** — worth revisiting when PDF lands
  (#198), since objects may carry a correct `Content-Type` and no extension. The loud-skip contract
  already accommodates either.
- **Multiple prefixes per source entry** versus one entry per prefix. One-per-entry works today and
  gives each prefix its own identity and status line; multi-prefix is an ergonomics question.
