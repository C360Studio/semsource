# Tasks: S3 / object-store source for document artifacts

Ordering follows the repo rule — behavior-preserving refactors first, then the
new packages, then the wiring that makes the source reachable. Group 1 is
independently mergeable and changes no observable behavior. Groups 2 and 3 have
no dependency on group 1; group 4 depends on all three.

Note on the source-type registry: `config.validSourceTypes`
(`config/source.go:10`) and `sourcespawn.buildSpecs`
(`internal/sourcespawn/sourcespawn.go:330`) are two independent lists and nothing
checks that they agree — `validSourceTypes` is unexported with no accessor, so the
invariant is unenforceable across the package boundary by construction. A type in
the first and missing from the second passes `semsource validate` and then fails at
`semsource run`. It fails *loudly* (typed `CodeUnsupportedType`), so this is not
silent degradation — but `validate` still makes a promise it cannot verify, which is
the same promise `runtime-configuration` already states as *"Validate-pass implies
publishable identity"*. Group 5 closes that gap **before** group 6 adds a tenth
type, so the guard is proven green against today's nine rather than asserted.

Scoping note: this change introduces **no new payload types**. Object-store
entities are document entities produced by the existing doc pipeline, so
`buildPayloadRegistry()` in `cmd/semsource/run.go` is untouched — task 4.5 asserts
that rather than assuming it.

Design open questions (restart-state persistence, content-type dispatch,
multi-prefix entries) are all deferrable and none is baked into a task below.

## 1. Content seam (behavior-preserving)

- [ ] 1.1 Split `handler/doc/entities.go:292` `ingestFileEntityStates` into an
  exported content-taking method on `doc.Handler` accepting
  `(ctx, content []byte, logicalPath, system, org string, now time.Time)`. The
  filesystem caller reads the file, computes the root-relative path, and
  delegates. Verify: `go test ./handler/doc/...` passes with **no test changes** —
  a behavior-preserving split needs no new fixtures.
- [ ] 1.2 Slash-normalize the logical path at the boundary (`filepath.ToSlash` on
  the filesystem side) and derive extension, MIME, and title-fallback inside the
  seam with `path.*` rather than `filepath.*` (design D3). Verify: a unit test
  asserting the seam produces identical entities for `a/b/doc.md` and the
  platform-native equivalent.
- [ ] 1.3 Add a shared table test driving both callers over the same fixture bytes
  and asserting byte-identical entity output, so the two callers cannot drift.
  Verify: the new test fails if either caller's title, MIME, hash, or passage
  ordinals diverge.

## 2. `storage/s3store`

- [ ] 2.1 Add `minio-go` to `go.mod` (design D1). Verify: `go build ./...`,
  `go vet ./...`, and a clean `go install ./cmd/semsource` with no new
  system prerequisites.
- [ ] 2.2 Implement `storage.Store` (`Put`/`Get`/`List`/`Delete`) and
  `StreamableStore.Open` against an S3-compatible endpoint, with explicit
  endpoint URL, path-style toggle, and region passthrough. Verify: a store test
  mirroring `storage/filestore/store_test.go` against a MinIO or Garage test
  container.
- [ ] 2.3 Add `Config` with `json` + `schema` tags, `Validate()`, and
  `DefaultConfig()` following `storage/filestore/config.go`, carrying endpoint,
  bucket, path-style, and region — and **no credential fields**. Verify: a config
  test asserting `Validate()` rejects an empty bucket and an unparseable endpoint.
- [ ] 2.4 Resolve credentials from the process environment at construction only
  (design, `specs/runtime-configuration`). Verify: a test asserting a credential
  key placed on the config struct fails strict decoding, and that neither
  `Validate()` output nor the component's log lines contain a secret value.

## 3. Enumeration and change detection

- [ ] 3.1 Implement the enumeration pass returning an observed key set **only** on
  a listing that completed across every continuation page, and an error otherwise
  (design D9). Verify: a test injecting a listing failure after page one returns
  an error and no key set.
- [ ] 3.2 Consume paginated listings fully and scope to the configured prefix.
  Verify: a test with more objects than one page asserts every object is
  enumerated and that objects outside the prefix are not.
- [ ] 3.3 Maintain the in-memory `key → ETag` map and skip re-fetching unchanged
  objects (design D5, D6). Verify: a test asserting a second pass over unchanged
  metadata issues zero object reads and publishes nothing.
- [ ] 3.4 Apply the document-extension gate **before** fetching a body, counting
  each skip with a reason (design D8). Verify: a test asserting an unsupported
  object produces no entity, no body fetch, and one counted skip carrying a reason.

## 4. Handler, identity, and processor

- [ ] 4.1 Implement `handler/objectstore` satisfying the `SourceHandler` interface
  (`Ingest`, `Watch`, `Supports`, `SourceType`), calling the group 1 content seam
  for each object it ingests. Verify: `handler` interface compliance test plus an
  ingest test over a fake store.
- [ ] 4.2 Construct identity as `system` = project override else
  `entityid.SystemSlug(bucket)`, `instance` = `entityid.SanitizeInstance(objectKey)`
  (design D7). Verify: tests covering the `specs/entity-identity-safety` scenarios —
  identical IDs across differing local paths, identical IDs across a restart, keys
  differing only past truncation staying distinct, and `ValidateEntityID` passing
  for keys containing `/`, spaces, and non-ASCII bytes.
- [ ] 4.3 Compute the content hash from fetched bytes with the doc handler's
  existing `contentHash`, never from the ETag (design D6). Verify: a test using a
  multipart-style composite ETag fixture asserts `DocFileHash` matches the bytes'
  hash and not the ETag.
- [ ] 4.4 Preserve the `ErrBodyStoreRequired` split — an unreadable object is one
  document's problem (skip and count), an unavailable body store aborts the pass.
  Verify: two tests, one per failure mode, asserting skip-and-continue versus
  abort.
- [ ] 4.5 Implement `processor/objectstore-source` with `component.go`,
  `config.go`, and `factory.go` per the component checklist, registered without
  touching `buildPayloadRegistry()`. Verify: component discovery test plus a build
  asserting `cmd/semsource/run.go` is unchanged.
- [ ] 4.6 Emit canonical typed `EntityStates` for every ingested or re-ingested
  object, with no `RawEntity` population, and treat a non-delete event lacking
  valid states as a bounded contract error that publishes nothing
  (`specs/typed-source-change-events`). Verify: tests for both the happy path and
  the missing-state contract failure.
- [ ] 4.7 Publish staleness markers for objects absent from a **completed** pass,
  and for no entity otherwise. Verify: the four spec scenarios — genuine removal,
  listing failure mid-pagination, authentication failure, and a legitimately
  emptied prefix.
- [ ] 4.8 Populate `internal/sourcestatus.Report` including readiness,
  backpressure, and skipped-object counts. Verify: a status test asserting the
  entry carries the same fields as every other source and that skip counts are
  visible without a source-specific query.
- [ ] 4.9 Assert the source is read-only: no ingest, watch, retraction, or status
  path issues a write, copy, or delete against the bucket. Verify: a fake store
  that fails the test on any mutating call, exercised across a full
  ingest-change-retract cycle.

## 5. Source-type registry invariant (pre-existing gap)

Lands before group 6 so the guard is proven green against today's nine source
types, rather than introduced alongside the tenth and assumed to work.

- [ ] 5.1 Export a `config.SourceTypes()` accessor over the private
  `validSourceTypes` map (`config/source.go:10`) so other packages can enumerate
  the supported types, and keep `:209` reading through it. Verify: `go build ./...`
  and `go test ./config/...` pass with no existing test changes.
- [ ] 5.2 Add an in-package test in `internal/sourcespawn` asserting every type
  from `config.SourceTypes()` builds component specs through `buildSpecs`
  (`sourcespawn.go:330`) without returning `CodeUnsupportedType`. The test asserts
  only the **config↔spawn** direction: a type valid in `semsource.json` with no
  `semsource add` subcommand is a UX gap, not a correctness bug, and asserting
  config↔CLI would force CLI surface nobody asked for — record that reasoning in
  the test's doc comment. Verify: the test passes against the nine types that exist
  today, before any object-store type is added.

## 6. Configuration and CLI

- [ ] 6.1 Add the object-store case to `sourcespawn.buildSpecs`
  (`internal/sourcespawn/sourcespawn.go:330`), mapping the type to the
  `objectstore-source` factory. Verify: a spawn test asserts the produced spec
  carries the expected factory name and source type.
- [ ] 6.2 Add the source type to `config/source.go:10` `validSourceTypes` and the
  type switch at `:209`, with validation rejecting a missing bucket and an
  unparseable endpoint. The group 5 invariant test is the guard here — it goes red
  if this lands without 6.1, so no same-commit discipline is required of the
  author. Verify: `semsource validate` tests for both rejection cases and one
  accepted entry, plus a green group 5 test.
- [ ] 6.3 Add `semsource add s3` to `cli/add.go:83` accepting bucket, prefix,
  endpoint, and the existing `--project` / `--version` identity flags. Verify:
  `cli/add_test.go` cases for explicit identity, omitted identity falling back to
  the bucket slug, and two prefixes of one bucket registered as distinct projects.
- [ ] 6.4 Make registration failure actionable — an unreachable or unauthenticated
  bucket fails with endpoint, bucket, and cause, leaving the config file
  unchanged. Verify: a CLI test asserting the error text and an untouched config
  file on failure.

## 7. Evidence, docs, and follow-ups

- [ ] 7.1 Add the source type to the README source-type table and supply the test
  evidence `advertised-surface-coverage` requires for an advertised surface.
  Verify: the advertised-surface test suite covers the new row.
- [ ] 7.2 Record the ownership revisit trigger in
  `docs/upstream/semstreams-asks.md` as a triaged framework-shaped **candidate** —
  a second sem\* service needing an artifact bucket (design D2). Verify: the entry
  exists and names the trigger, following the file's existing entry format.
- [ ] 7.3 Add an integration test ingesting a fixture corpus from a containerized
  Garage (or MinIO) instance end to end, reaching `phase: ready` and answering one
  query. Verify: `go test -tags=integration ./...` passes with the container
  running.
- [ ] 7.4 Measure an unbounded-prefix ingest against the #178 GRAPH ceiling
  behavior before any documentation calls this unbounded-safe. Verify: a recorded
  measurement in the change or issue, with prefix-scoping guidance if a ceiling is
  hit.
- [ ] 7.5 Run the full gate before push — `gofmt`, `go vet`, `revive` (pinned
  v1.15.0, warnings fail), and `go test -race -tags=integration ./...`. Verify: CI
  green.
