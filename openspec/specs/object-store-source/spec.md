# object-store-source Specification

## Purpose
Ingesting document artifacts that live in an S3-compatible object store rather than on a local
filesystem — connection and addressing compatibility across Garage/MinIO/AWS, prefix-scoped
enumeration, ETag-based change detection without a filesystem watcher, and the loud-skip contract
for objects the document pipeline cannot parse.

## Requirements

### Requirement: An object-store source connects to S3-compatible endpoints, not only AWS

An object-store source SHALL accept an explicit endpoint URL and addressing style so that
self-hosted S3-compatible stores are first-class targets rather than a deviation. Path-style
addressing MUST be selectable, and a region value MUST be accepted and forwarded even when the
target store does not implement regions. Connecting to AWS S3 MUST NOT require a different source
type or a separate code path.

#### Scenario: A self-hosted Garage endpoint

- **WHEN** a source is configured with a Garage endpoint URL, path-style addressing, and a region
  value the store ignores
- **THEN** objects under the configured bucket and prefix are enumerated and ingested
- **AND** no AWS-specific endpoint construction is applied to the request

#### Scenario: AWS S3 with the same source type

- **WHEN** the same source type is configured against AWS S3 without an explicit endpoint
- **THEN** the default AWS endpoint and virtual-hosted addressing are used
- **AND** ingestion behavior is otherwise identical to the self-hosted case

#### Scenario: The store is unreachable at startup

- **WHEN** the configured endpoint cannot be reached while the service starts
- **THEN** the failure is reported through the source's status surface with the endpoint and cause
- **AND** the service's other surfaces still become reachable

### Requirement: Enumeration is prefix-scoped and complete

An object-store source SHALL enumerate objects under a configured bucket and optional key prefix,
and MUST consume the store's full paginated listing before treating an enumeration pass as complete.
A partial listing MUST NOT be presented as a complete one.

#### Scenario: A prefix with more objects than one listing page

- **WHEN** the configured prefix contains more objects than the store returns in a single response
- **THEN** every object under the prefix is enumerated across continuation pages
- **AND** the pass is reported complete only after the final page

#### Scenario: Objects outside the prefix

- **WHEN** the bucket contains objects that do not match the configured prefix
- **THEN** they are not enumerated and not ingested

#### Scenario: Enumeration fails partway

- **WHEN** a listing request fails after some pages have been consumed
- **THEN** the pass is reported as failed, not complete
- **AND** the partial result is not used to draw conclusions about which objects exist

### Requirement: Change detection uses object metadata, and unchanged objects are not re-ingested

An object-store source SHALL detect changes by comparing object metadata (ETag, falling back to
size and last-modified where a store's ETag is not content-derived) between enumeration passes. An
object whose metadata is unchanged MUST NOT be re-fetched or re-published. A changed object MUST
re-ingest under the SAME entity identity, updating content in place rather than minting a sibling
entity.

#### Scenario: An unchanged object across two passes

- **WHEN** two consecutive enumeration passes observe the same object with identical metadata
- **THEN** the object body is not re-fetched
- **AND** no entity state is republished for it

#### Scenario: An object is replaced at the same key

- **WHEN** an object at a previously ingested key is replaced with different content
- **THEN** it re-ingests under the identical entity ID
- **AND** its content hash triple reflects the new content

#### Scenario: A new object appears under the prefix

- **WHEN** an object is added under the configured prefix between passes
- **THEN** it is ingested on the next pass without requiring a service restart

### Requirement: Objects the document pipeline cannot parse are skipped loudly

An object whose content type or extension is not supported by the document pipeline MUST NOT be
ingested as an empty or partial document. It SHALL be counted and reported on the source's status
surface as skipped, with the reason, so an operator can tell the difference between "no such
document" and "that document was never parsed".

#### Scenario: An unsupported artifact format

- **WHEN** the prefix contains an object in a format the document pipeline does not support
- **THEN** no document entity is produced for it
- **AND** the source's status reports it among its skipped objects with a reason

#### Scenario: A zero-byte object

- **WHEN** an enumerated object has no content
- **THEN** it is skipped and reported rather than published as a document with an empty body

### Requirement: An object-store source never writes to the bucket

An object-store source SHALL treat the configured bucket as read-only. No ingest, watch, retraction,
or status path may create, overwrite, or delete an object, regardless of what the underlying storage
interface makes available.

#### Scenario: A full ingest and retraction cycle

- **WHEN** a source ingests a prefix, observes changes, and retracts entities for removed objects
- **THEN** no write, copy, or delete request is issued against the bucket

### Requirement: Per-source status is reported on the shared contract

An object-store source SHALL report its status through the same per-source report every other source
uses, including its readiness, backpressure, and skipped-object counts, so operator and agent
surfaces need no source-specific handling.

#### Scenario: Status is legible beside other sources

- **WHEN** an operator reads the status surface with an object-store source configured
- **THEN** its entry carries the same fields as every other configured source
- **AND** its skipped-object count is visible without a source-specific query
