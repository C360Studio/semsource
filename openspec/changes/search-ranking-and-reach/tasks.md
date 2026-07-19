# Tasks: Search Ranking and Reach

## 1. Test demotion (D1, D2)

- [x] 1.1 `source/ast`: per-language `isTestPath` helper + unit tests (Go, TS/JS,
      Svelte, Python, Java shapes; conservative on unknowns)
- [x] 1.2 `code.artifact.test` presence marker predicate + triple emission next to
      the exported marker; vocabulary registration `WithWeight(-2.0)`; unit test
      pins marker presence for test paths and absence for production paths
- [x] 1.3 Salience-weight pin test (registration carries −2.0)

## 2. Doc authority (D3)

- [x] 2.1 Doc walker skips `openspec/changes/archive` (root-relative) and
      `node_modules`; unit test with a fixture tree
- [x] 2.2 Doc-source component path inherits the same behavior (walk shared or
      mirrored); test

## 3. Reach (D4)

- [x] 3.1 cfgfile entities emit `dc.terms.title` (dependency/module/image/package
      names); unit tests
- [x] 3.2 git entities emit `dc.terms.title` (commit subject, author, branch);
      unit tests
- [x] 3.3 docs-lens scope gains the `config` domain; scope test

## 4. Acceptance

- [x] 4.1 Rebuild the live stack and re-run the audit's failed graded questions
      (Q9–Q13 code_search, Q16–Q17 doc_context, Q19 config reach); record grades
- [x] 4.2 Gates green (revive v1.15.0, gofmt, vet, `go test -race`); openspec
      validate; PR with re-run evidence
