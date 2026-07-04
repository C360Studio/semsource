# Tasks: Versioned-Source Current Marker & Historical Demotion

## 1. Vocabulary — demote superseded_by

- [ ] 1.1 In `source/ast/vocabulary.go` `registerLineagePredicates()`, add `vocabulary.WithWeight(-2.0)`
      to the `code.lineage.superseded_by` registration (currently unweighted). Add a comment tying the
      sign to gh#441 signed salience and the "current is emergent (un-superseded)" model (design D1/D2).
      Leave `supersedes` and `change` unweighted (structural, not a ranking signal).

## 2. Tests

- [ ] 2.1 Unit — `code.lineage.superseded_by` is registered with a negative weight (assert
      `semvocab.GetPredicateMetadata(CodeSupersededBy).Weight < 0`); `supersedes`/`change` stay 0.
      Confirms the demote is wired and scoped to the one predicate.
- [ ] 2.2 Integration (`//go:build integration`, extend the item-#2 governance harness) — index the
      same symbol at two versions, run the supersession pass so `v1.10.0` supersedes `v1.9.0`, then
      query via the fusion engine and assert the **current** (`v1.10.0`, un-superseded) entity ranks
      **above** the **historical** (`v1.9.0`, superseded) entity, and that the historical entity is
      still present in the results (bounded reorder, not exclusion — spec scenarios 1 & 2).

## 3. Gates

- [ ] 3.1 `go build ./...`, `go test ./...` green; `task lint` zero warnings (revive v1.15.0), gofmt,
      go vet.
- [ ] 3.2 `openspec validate versioned-source-current-marker` green; `/opsx:verify` before archive.
