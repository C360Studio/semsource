# Restore reproducible CI MinIO fixtures

Closes [SemSource #214](https://github.com/C360Studio/semsource/issues/214).
Addresses the fixture gate tracked by #212; coordinated with migration draft #213.

## Why

The pinned MinIO fixture cannot be acquired in the observed baseline and migration environments.
This blocks the full S3/governance integration gate. The Garage workflow deliberately selects both
`integration` and `garage` tests, so a MinIO setup failure also prevents its combined compatibility
lane from completing. Passing a reduced suite does not replace the missing original cases.

This change starts independently from main commit `34bda6406fb06fd723a040988647b204204a1583`,
with SemStreams `v1.0.0-beta.161`. The fixture repair must stand on its own before a separate overlay
is validated against migration #213. It must not acquire migration behavior as an accidental dependency.

## What Changes

- Use the architect-approved Thanos mirror of the exact existing upstream MinIO release, pinned
  by the verified manifest digest recorded in design.md. Runtime qualification remains pending.
- Record immutable artifact identity, upstream mirror provenance, supported architecture, and
  clean-acquisition proof; use the same selected fixture contract locally and in CI.
- Keep all existing S3, Garage, and governance cases and assertions. Selected tests fail visibly
  when their fixture is unavailable; acquisition failure must not become an implicit skip.
- Retain bounded readiness and isolation; clean up partial-start handles, fail otherwise-green suites
  on teardown errors, and explicitly terminate the governance MinIO fixture.
- Validate the mainline fix first, then record a separate migration-overlay run with its original pins.

## Capabilities

### New Capabilities

- `integration-fixture-reproducibility`: immutable S3 fixture identity, honest availability,
  lifecycle ownership, retained coverage, and independently attributable validation evidence.

### Modified Capabilities

None. Production object-store, ingestion, and graph contracts remain unchanged. Proposed requirements
live only in this change's delta until implementation is verified; no current-truth spec is backfilled.

## Impact

Surfaces are `internal/miniotest`, shared fixture/TestMain cleanup, governance MinIO teardown, and
the Garage workflow path filter for `internal/miniotest/**`. Preserve the Garage bootstrap and image;
Garage remains its own compatibility backend. No source build or image publication is required.

SemSource developers and CI directly consume this capability. SemSpec, SemDragon, and SemOps benefit
from credible downstream integration evidence but receive no changed runtime API. Fixture ownership
belongs to SemSource testing; SemStreams still owns substrate behavior.

## Non-goals

- SemStreams or embedding-provider pin changes, SemEngine linkage, or migration #213 implementation.
- Changing production storage behavior, entity identity, graph retention, or query semantics.
- Replacing Garage with MinIO, dropping cases, weakening assertions, or treating skipped tests as green.
- Rebuilding or publishing MinIO, selecting an unverified mirror, or changing the Garage image/bootstrap.
- Solving source-removal replay, discovery, broker-generation recovery, or GRAPH capacity blockers.
