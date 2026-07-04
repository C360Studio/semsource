# Tasks: Versioned-Source Supersession

## 1. Resolve design open questions (decide before coding)

- [ ] 1.1 OQ2 — confirm predicate names against `source/ast/predicates.go` + the vocabulary registry:
      `code.artifact.version`, `code.artifact.project`, `code.lineage.supersedes`,
      `code.lineage.superseded_by`, `code.lineage.change`. Adjust to house convention if needed.
- [ ] 1.2 OQ3 — decide the correspondence grouping key: confirm whether raw `project` suffices or `org`
      must be folded in (two different repos both named `utils`). Default: group key includes `org`.
- [ ] 1.3 OQ1 — pick the pass trigger for v1: reactive on a version scope completing initial index, vs
      periodic, vs on-demand. Record the choice in the component config.

## 2. Vocabulary / predicates

- [ ] 2.1 Register the version + source-identity predicates (`code.artifact.version`,
      `code.artifact.project`) in `source/ast/predicates.go` / the vocabulary registry.
- [ ] 2.2 Register the lineage predicates (`code.lineage.supersedes`, `code.lineage.superseded_by`,
      `code.lineage.change`) with the correct roles (relationship vs metadata).

## 3. Emit version + source-identity triples at ingest (ast-source)

- [ ] 3.1 Thread the `version` and `project` config through the ast triple builders and emit
      `code.artifact.project` (always) and `code.artifact.version` (only when version is non-empty).
- [ ] 3.2 Test — entity indexed at a version carries both triples (spec: "Entity indexed at a version").
- [ ] 3.3 Test — version-less entity emits no version triple and is byte-identical to prior output
      (spec: "Entity indexed without a version"; a golden assertion, like #1's backward-compat test).

## 4. New `supersession` processor component

- [ ] 4.1 Scaffold the component per the semstreams pattern: `config.go` (Config + Validate +
      DefaultConfig, incl. the trigger from 1.3), `component.go` (Discoverable), `factory.go`
      (`Register`). Blank-import in the entry point. Reuses the existing `EntityPayload`
      (`semsource.entity.v1`) — no new payload.
- [ ] 4.2 Enumerate candidate code entities via `graph.query.prefix` (paginated, cursor); read
      project / version / path / name / type / package / body triples.
- [ ] 4.3 Group by source identity (per 1.2), then hash-key each entity by `(path, name, type, package)`
      to form correspondence groups — O(n), not pairwise.
- [ ] 4.4 Implement version ordering: semver-aware comparator with fallback to entity `IndexedAt`, else
      incomparable → no edge (design D4).
- [ ] 4.5 Emit adjacent, directional supersession edges via the existing entitypub publish path
      (append-only EntityPayload with a semantic envelope): `code.lineage.supersedes` (newer→older) +
      inverse `code.lineage.superseded_by`; deterministic triples so re-runs merge to a no-op.
- [ ] 4.6 Classify `code.lineage.change` = changed|unchanged by comparing `code.artifact.body` hashes.
- [ ] 4.7 Guarantee retention-safety: the pass only publishes/append-merges; it never calls delete/retract.

## 5. Tests (one per spec scenario)

- [ ] 5.1 Correspondence — same symbol at two versions corresponds; same path/name across different
      sources does NOT (spec: correspondence requirement, both scenarios).
- [ ] 5.2 Supersession — newer supersedes older with inverse edge; a new-only symbol gets no edge
      (spec: supersession requirement).
- [ ] 5.3 Idempotency — re-running over an unchanged graph creates no duplicates and removes nothing.
- [ ] 5.4 Incomparable versions — non-orderable versions coexist with no edge.
- [ ] 5.5 Changed classification — body differs → changed; identical → unchanged.
- [ ] 5.6 Retention — superseded version's entities + triples remain queryable after the pass.

## 6. Integration & gates

- [ ] 6.1 Integration/e2e — index the same source at two versions, run the pass, assert supersession
      edges + change classification end-to-end (guarded build tag, per the repo's e2e convention).
- [ ] 6.2 `go build ./...`, `go test ./...` (+ `-race` on the new component), all green.
- [ ] 6.3 `task lint` — zero warnings (revive v1.15.0), gofmt, go vet.
- [ ] 6.4 `openspec validate versioned-source-supersession` green; `/opsx:verify` before archive.
