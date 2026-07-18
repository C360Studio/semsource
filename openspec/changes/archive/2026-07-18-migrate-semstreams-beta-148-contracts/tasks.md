## 1. Freeze the migration contract

- [x] 1.1 Add a reviewed ledger for the 90 retired registrations and 92 canonical targets, including
  producers, SemSource exact-query consumers, positive fixtures, and downstream handoff owners.
  - Test: a ledger test fails on a missing source identity, duplicate target, unmapped surface, or
    collision and reports exactly 90 replacements plus two additions.
- [x] 1.2 Write failing contract tests for canonical/unique/registered parity, retired identity hits,
  and the three required entity-reference datatypes.
  - Test: the focused contract suite fails against the pre-migration code for the expected reasons.

## 2. Adopt beta.148 and cut over vocabulary

- [x] 2.1 Pin SemStreams `v1.0.0-beta.148`, remove any module replacement, run `go mod tidy`, and
  record the compile delta without changing the component/payload registration inventory.
  - Test: `go list -m github.com/c360studio/semstreams` reports beta.148, `go.mod` has no matching
    `replace`, and `go mod tidy` is clean on a second run.
- [x] 2.2 Add the two missing canonical body-reference registrations, then **BREAKING** atomically
  replace all 90 invalid identities across registrations, producers, SemSource exact queries, and
  positive fixtures.
  - Test: the migration contract reports exactly 92 canonical, unique, registered targets and zero
    retired identity hits in production or SemSource exact-query code.
- [x] 2.3 Set SemStreams `EntityReferenceDatatype` on guaranteed relationship triples in
  `handler/git/entities.go`, `handler/video/entities.go`, and `processor/supersession/edges.go`.
  - Test: focused entity tests prove all three references are typed and their objects are canonical
    six-part IDs, while literal lookalikes remain literal.
- [x] 2.4 Prove current binary initialization and composition with the existing explicit component and
  payload registrations.
  - Test: the init/composition smoke starts without panic and matches the reviewed current inventory.

## 3. Prove runtime behavior and cutover

- [x] 3.1 Run existing unit and integration suites after the atomic semantic cutover.
  - Test: `go test ./...` and `go test -tags=integration ./...` pass uncached.
- [x] 3.2 Add one disposable real-NATS seed/query smoke using the normal SemSource composition.
  - Test: canonical fixtures seed successfully, status reaches ready, and one exact known-answer
    query returns the canonical predicate/object with no retired identity.
- [x] 3.3 Rehearse the destructive cutover with writers stopped and a reviewed literal account/resource
  inventory.
  - Test: evidence shows deletion of `semstreams_config` and only observed incompatible `GRAPH`,
    enabled `FrameworkOwnedBuckets`, `ENTITY_SUFFIX_INDEX`, `GRAPH_INGEST_APPLIED_SEQ`, and observed
    `PREDICATE_CATALOG`; preserved source/content/media/object/status/unrelated resources remain.
- [x] 3.4 Recreate configuration from the reviewed `semsource.json`, start only migrated writers,
  reseed, wait for ready, and repeat the known-answer query.
  - Test: post-cutover evidence binds the reviewed config, seed, ready status, and canonical query
    result; no preservation, in-place rewrite, or mixed-writer path is used.

## 4. Review and release gates

- [x] 4.1 Publish the exact predicate handoff to downstream sem* owners without editing their
  repositories, and document only the supported cutover.
  - Test: technical-writer and architect confirm each known downstream exact-query owner is named and
    the runbook contains the literal inventory and preservation boundary.
- [x] 4.2 Run gofmt, vet, revive with warnings failing, race, strict OpenSpec validation, and final
  reviewer checks.
  - Test: gofmt is clean; `go vet ./...`, revive, `go test -race ./...`, and
    `openspec validate migrate-semstreams-beta-148-contracts --strict` pass with no unresolved
    architect or go-reviewer finding.
