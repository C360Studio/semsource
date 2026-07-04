# Tasks: Versioned-Source Current Marker & Historical Demotion

## 1. Vocabulary — demote superseded_by

- [x] 1.1 In `source/ast/vocabulary.go` `registerLineagePredicates()`, add `vocabulary.WithWeight(-2.0)`
      to the `code.lineage.superseded_by` registration (currently unweighted). Add a comment tying the
      sign to gh#441 signed salience and the "current is emergent (un-superseded)" model (design D1/D2).
      Leave `supersedes` and `change` unweighted (structural, not a ranking signal).

## 2. Tests

- [x] 2.1 Unit — `code.lineage.superseded_by` is registered with a negative weight; `supersedes`/`change`
      stay 0. Extended `TestPredicateSalienceWeights` (source/ast/salience_test.go): `CodeSupersededBy:
      -2.0` in the weighted map, `CodeSupersedes`/`CodeLineageChange` in the unweighted set.
- [x] 2.2 Integration (`//go:build integration`, `internal/governance/supersession_demote_integration_test.go`)
      — index the same symbol at two versions, run the pass so `v1.10.0` supersedes `v1.9.0`, then query
      via the fusion engine (prefixLens over `acme.semsource.golang`) and assert the **current** entity
      ranks **above** the **historical** one, both present (retention). PASS. Scale math: the -2.0 demote
      (`salienceScale·2=6.0`) beats the worst-case resolve-position gap (`resolveScale·1=4.0`), so the
      order is deterministic regardless of resolution order.

## 3. Gates

- [x] 3.1 `go build ./...`, `go test ./...` green; `task lint` zero warnings (revive v1.15.0), gofmt,
      go vet.
- [x] 3.2 `openspec validate versioned-source-current-marker` green; `/opsx:verify` before archive.
