Commit boundaries follow the additive-first rule: group 1 is pure deletion (nothing reads it),
groups 2–5 are additive, group 6 is the breaking cut, groups 7–9 are documentation and proof.

## 1. Delete the dead-code cascade (D8, D9 — no behaviour change)

- [x] 1.1 Delete `source/parser/` wholesale (6 parsers + 6 test files); confirm zero importers first
- [x] 1.2 Delete `source/types.go` and `source/requests.go` — orphaned once `source/parser/` is gone
      (includes the never-instantiated `Chunk` struct and both `ChunkCount` fields)
- [x] 1.3 Delete the dead half of `source/vocabulary/enums.go` (`DocCategoryType`, `DocSeverityType`,
      `DocScopeType`, `StatusType`, `TypeValue`, `DomainType`) and their `predicates_test.go` assertions
- [x] 1.4 Delete the ~25 registered-but-never-emitted predicates from `source/vocabulary/predicates.go`
      (`DocCategory`, `DocAppliesTo`, `DocSeverity`, `DocScope`, `DocDomain`, `DocRelatedDomains`,
      `DocKeywords`, `DocRequirements`, and the `Web*` analogues) plus their `init()` registrations
- [x] 1.5 Delete the matching assertions in `predicates_test.go` and `beta148_contract_test.go`
- [x] 1.6 Delete the four `explicitRoles` entries in `processor/source-manifest/status.go:299,301-304`
      that classify predicates nothing emits
- [x] 1.7 Collapse `source/vocabulary/iris.go` to its four live constants (`DcAbstract`, `DcFormat`,
      `DcType`, `MaNamespace`); delete every `Class*`/`Prop*` referenced only from comments, including
      the dead chunk IRIs
- [x] 1.8 Delete the doc `RawEntity` path: `handler/doc/handler.go` `Ingest()` and `ingestFile()`,
      `handler/doc/exclude_test.go`, and the `Ingest`-based tests in `handler_test.go`
- [x] 1.9 Delete write-only fields `Entity.System` and `Entity.Org` (`handler/doc/entities.go:30-31,66-67`)
- [x] 1.10 Delete every stale comment in the doc path: "bypassing the normalizer" (no such package
      exists), the ID-parity comments justifying two paths, and `predicates.go:62-68`'s
      "absent until the doc body producer lands"
- [x] 1.11 `go build ./... && go test ./...` green; `go mod tidy` (check whether `yaml.v3` is now unused)

## 2. Build the splitter (D4)

- [x] 2.1 Write the splitter in `handler/doc/` as a pure function of document bytes → ordered passages
      carrying (ordinal, heading, anchor, body)
- [x] 2.2 Implement heading splitting (ATX + setext), then paragraph subdivision above the ceiling,
      then sentence split, then hard cut as last resort
- [x] 2.3 Implement the floor-merge so consecutive trivial sections share a passage
- [x] 2.4 Guarantee fenced code blocks are never split across passages
- [x] 2.5 Handle content before the first heading as a passage
- [x] 2.6 Test: determinism property — same bytes yield byte-identical boundaries across runs
- [x] 2.7 Test: no passage exceeds the ceiling, for every document in this repository
- [x] 2.8 Test: golden-file boundaries over real repository documents (README, an ADR, a spec)
- [x] 2.9 Test: reconstruction — concatenating passages in ordinal order reproduces indexed content

## 3. Emit passage entities (D1, D2, D6, D7)

- [x] 3.1 Add the chunk entity ID constructor: `{org}.semsource.web.{system}.chunk.{path-slug}-{ordinal}`
      via `entityid.Build`, zero-padded ordinal
- [x] 3.2 Test: ID determinism, repeated-heading distinctness, headingless coverage, and stability
      under heading rename
- [x] 3.3 Rewrite `ingestFileEntityState` to emit one parent plus N passage entity states
- [x] 3.4 Emit passage triples: body handle, `DocChunkIndex`, `DocSection`, qualified `dc.terms.title`,
      and `code.structure.belongs` to the parent with `EntityReferenceDatatype`
- [x] 3.5 Offload each passage body content-addressed (`doc:<sha-of-passage>`) and set its `StorageRef`
- [x] 3.6 Emit `DocChunkCount` on the parent; keep `DcTitle`, `DocFilePath`, `DocMimeType`,
      `DocFileHash`, provenance
- [x] 3.7 Update the watch path (`enrichEventEntityStates`) to emit the same parent + passage set
- [x] 3.8 Test: parent/passage triple shapes, chunk count correctness, entity-reference datatype

## 4. Retract vanished passages (D3)

- [ ] 4.1 Fast path: on re-ingest with fewer passages, mark ordinals `[new_count, old_count)` with
      `entity.lifecycle.stale`, reading the prior count from the parent's `DocChunkCount`
- [ ] 4.2 Clear the marker for ordinals that come back when a document regrows (`RemoveTriples`)
- [ ] 4.3 Backstop: extend `decideLifecycleActions` in `processor/supersession/lifecycle.go` so that for
      a path present on disk, any passage whose `DocChunkIndex >= DocChunkCount` is marked stale
- [ ] 4.4 Preserve the existing caller-side pre-filter (the add lane appends; only mark entities not
      already carrying the marker)
- [ ] 4.5 Test: shrink marks the tail, entities remain present, live passages are never marked
- [ ] 4.6 Test: regrowth clears markers and re-points content
- [ ] 4.7 Test: the backstop marks an orphaned tail when the fast path never ran (interrupted ingest)

## 5. Expand passages through the docs lens (D5)

- [ ] 5.1 Implement `Edges()` in `source/fusion/lens/docs/docs.go` declaring `source.CodeBelongs`
      with parent/chunk roles, restricted to `FacetRelations`
- [ ] 5.2 Set `Locator.Fragment` to the section anchor for headed passages
- [ ] 5.3 Verify `Label()` and `Location()` produce usable references for passage entities
- [ ] 5.4 Test: passage seed resolves to parent; document seed resolves to passages
- [ ] 5.5 Test: containment contributes no edges to impact or paths walks

## 6. Breaking cut (D10, D11, D12)

- [ ] 6.1 Make the verbatim body store mandatory in `processor/doc-source/component.go`; replace the
      "store unavailable" warning with a hard startup error
- [ ] 6.2 Delete the inline `DocContent` fallback and the tests pinning it
      (`StoreFailureFallsBackToInline`, `NoStoreAllInline`)
- [ ] 6.3 Test: unavailable store fails startup loudly and the component is not ready
- [ ] 6.4 Delete `DocSummary`: the emit, the lens `Label()` fallback, the `explicitRoles` entry, and the
      three tests pinning them; reduce `Label()` to `DcTitle`
- [ ] 6.5 Remove the parent's body handle, `StorageRef`, and content-indexing profile so it contributes
      no content embedding
- [ ] 6.6 Test: parent carries no body handle and produces no content embedding, but stays name-resolvable

## 7. Ranking behaviour (retrieval-ranking delta)

- [ ] 7.1 Verify a body-less parent does not outrank passages carrying content
- [ ] 7.2 Ensure a result set never returns both a parent and its own passages as competing evidence
- [ ] 7.3 Test both as integration assertions against a real graph stack, not unit mocks

## 8. Documentation and vocabulary prose

- [ ] 8.1 Rewrite `source/vocabulary/doc.go` from scratch to describe the model actually shipped
      (delete the "Parent-Chunk Model" block and the wrong ID schemes)
- [ ] 8.2 Reconcile the `docs.go` package comment with `predicates.go` (they currently contradict)
- [ ] 8.3 Fix the lens/handler extension mismatch (`.rst` listed but never ingested; `.mdx` ingested but
      not listed)
- [ ] 8.4 Document the rebuild-from-empty migration and why in-place reindex is insufficient (gh#260)
- [ ] 8.5 Update `ROADMAP.md` — passage chunking moves out of the dilution limitation
- [ ] 8.6 File the two framework asks in `docs/upstream/semstreams-asks.md` (offloaded entities never
      embed their title; the 8000-character cap is not configurable) and open the GitHub issues
- [x] 8.7 File ask #20: `CONTENT` ObjectStore's hard-coded 24h TTL silently expires verbatim bodies,
      coupled to the missing orphan sweep (track only — folds into the on-hold retention ADR)
- [x] 8.8 Correct the stale status on ask #10 (semstreams#430 is RESOLVED in beta.127, not open)

## 9. Proof

- [ ] 9.1 `task lint` clean at revive v1.15.0 (warnings fail), `gofmt`, `go vet`
- [ ] 9.2 `go test ./...` and `go test -race -tags=integration ./...` green
- [ ] 9.3 Integration test on a real stack: ingest a document larger than the truncation limit, then
      retrieve a phrase that appears only in its tail
- [ ] 9.4 Measure entity count and time-to-ready on this repository before and after; record both in
      this change and treat a large regression as a blocker
- [ ] 9.5 Choose the chunk ceiling and floor empirically — A/B the graded interrogation, do not guess
      (the one open question in design.md)
- [ ] 9.6 Re-run the graded interrogation; confirm Q13/Q17 improve and no previously correct answer
      regresses; record the score
- [ ] 9.7 Verify no reserved-but-unemitted vocabulary remains: every registered predicate has a live
      emitter or is gone
