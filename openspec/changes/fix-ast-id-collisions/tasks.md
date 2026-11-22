# One entity ID per declaration — tasks

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md`, and rules on Q1–Q4. Hold: OPEN-Q proposal awaits owner approval

## 2. Verify before code

- [ ] 2.1 Rerun the #333 collision probe across Python, C and C++ fixtures and record any collisions. Any found become a follow-up issue, out of this break.
- [ ] 2.2 Measure decision 3 on OSH against today's edges: how many edges arity keeps, and how many are lost to each of the three Q1 costs (same-arity overloads, external ancestor chains, overrides written differently). Record the numbers here before Q1 is closed.
- [ ] 2.3 Confirm that no TS/JS/Svelte edge targets a local entity that decision 4 removes. Check `calls`, references and destructured bindings (the #103 rule) across dogfood; the reviewer read `ts/calls.go:103-129` as top-level only. If an edge does target one, decision 4 is revised before code.

## 3. Guard (additive; lands first and fails)

- [ ] 3.1 `source/ast` test: per-class fixtures (Java overloads, TS scoped consts, Go `_`) fail on duplicate IDs today. Proven by `TestParseResultIDsAreUnique` failing before 4.x.
- [ ] 3.2 `processor/ast-source`: dedup by ID before `bodiesForResult`, keeping the first, plus the cross-file check over the initial index; `id_collisions` status field; one aggregated warning per file. Proven by a unit test that feeds a result with a duplicate and asserts one publish, one body, and a count of 1. The status field is reviewed by `go-component-reviewer`.

## 4. Identity fixes (breaking; one storage break)

- [ ] 4.1 Java discriminator (kind, type text without type arguments, array dimensions kept, type variables to their first bound) on every method and constructor. Tests: the constructor-versus-method and `int`/`int[]` scenarios, plus stability under a sibling-overload add and a parameter rename.
- [ ] 4.2 Java call edges collect overloads across the in-repo hierarchy and select by arity even with one candidate, or emit none; constructor calls likewise. Tests are the five code-call-graph scenarios.
- [ ] 4.3 TS/JS/Svelte: stop emitting locals of every form (namespace members kept); always discriminate accessor pairs and static members, and carry the discriminated ID through `classMethodNames` for `this.` calls. Tests: a `this.` call to a method and a `ClassName.` call to a static method still edge correctly, plus the `fetcher`, local-function and accessor scenarios, with module-level IDs unchanged.
- [ ] 4.4 Go: skip blank `var`, `const` and `func`, and identify `init` per Q4. Test: the blank-declaration scenario, plus an `init` test under the ruling.
- [ ] 4.5 Asset guard: an `X.js` with an `X.min.js` sibling is file-entity-only, plus `index_vendored_js`. Tests are both asset-ingestion-guards scenarios.
- [ ] 4.6 3.1 passes on all fixtures, and the repo-tree guard reports zero collisions.

## 5. Acceptance

- [ ] 5.1 Probe reruns: zero collisions on dogfood and OSH, and zero hybrids from `/tmp/hybrid`-style replay. Numbers recorded on #329.
- [ ] 5.2 Fresh-seed and restart ENTITY_STATES dumps are identical apart from `dc.terms.created`. This is the #324 shape.
- [ ] 5.3 Scorecard on the reseeded store: R1 36/36 (closes #324), OSH arm not below 10/10, dogfood loose band not below the beta.10 regrade.
- [ ] 5.4 Release note and SETUP 03A record the identity break and the reseed.
