# Tasks: call-graph-completeness

## 1. Go parameter-call suppression (#143, D2)

- [x] 1.1 Suppress call edges whose callee binds to a function-typed parameter/local in the Go
      extractor; table-driven tests pin the inert forms (param, local, closure-captured) and the
      still-resolving forms (package function, method) — also suppresses same-file type
      conversions (same dangling class)
- [ ] 1.2 Prove the dogfood signal: seed the dogfood corpus and confirm zero
      `not found: ...lifecycle-go-stat`-class graph-query errors

## 2. Java declared-type receiver resolution (#141, D1)

- [x] 2.1 Build per-method declared-type tables (params, locals, fields incl. supers walk) in
      `java/calls.go`; resolve `x.m(...)` via `declaringClass(typeOf(x), m)`; ties, generics,
      unbindable types drop — tests pin resolving and inert forms. (Param/local tables already
      existed; the real gap was INHERITED fields — classMembers now carries visibility-aware
      field tables and classFieldsWithInherited merges them nearest-wins)
- [ ] 2.2 Verify on the OSH corpus: `cloneAsTemplatePermission` impact names ConSysApiSecurity +
      SOSSecurity + SPSSecurity callers (the P01 shape) in a local stack check

## 3. TS + Svelte call edges (#149, D3)

- [x] 3.1 `ts/calls.go`: same-file direct calls + named-import-bound calls through the existing
      module→file resolution; `this.m()` own-class only; property chains/computed callees inert —
      table-driven tests for every form
- [x] 3.2 Default and namespace import forms with tested inert fallbacks; barrel re-exports one
      hop max
- [x] 3.3 Wire the pass into the Svelte parser's script blocks; test a component→module edge and
      the ui/ corpus parses without regression (entity counts stable ± the new edges)

## 4. C direct calls (#149, D4)

- [x] 4.1 Measured on redis via our own parser (collision_measure_test.go, C_MEASURE_CORPUS):
      src/ = 6,141 defs / 5,596 names / **1.41%** defined in >1 file; with vendored deps 1.80%.
      Under the 3% C++ precedent → **corpus-unique binding ships**; the colliding tail drops
- [x] 4.2 C call pass: name-bound direct calls resolving to the DEFINITION (index counts
      function_definition nodes only — prototypes would self-collide every header-declared
      function); function-pointer (param/local/member) and multi-definition names inert; eager
      index for order-independence, hash-revalidated per ParseFile; 7 tests pin both sides

## 5. Contract, verification, and gates (D5)

- [ ] 5.1 Sync the per-language coverage contract into the current-truth `code-call-graph` spec
      (delta already drafted) and the README/docs surface where consumers read coverage
- [ ] 5.2 Scorecard re-runs, both corpora: OSH P01 flips to correct (expect B 10/10), dogfood
      unchanged 26/26; commit results + SUMMARY delta; note OSH seed wall-clock vs the 1,311 s
      baseline (D1 parse-cost check)
- [ ] 5.3 Review gate: go-component-reviewer + graph-event-reviewer + high-effort code review;
      findings addressed before merge
