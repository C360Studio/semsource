# One entity ID per declaration — tasks

## 1. Approval

- [ ] 1.1 Owner approves `proposal.md` and `design.md`, and rules on Q1–Q3. Hold: OPEN-Q proposal awaits owner approval

## 2. Verify before code

- [ ] 2.1 Rerun the #333 collision probe across Python, C and C++ fixtures and record any collisions. Any found become a follow-up issue, out of this break.
- [ ] 2.2 Measure decision 3 on OSH: the share of Java call sites to overloaded names that arity resolves, versus same-arity sites that would lose their edge. Record the numbers in this task before Q1 is closed.
- [ ] 2.3 Confirm that no TS/JS/Svelte call edge targets a function- or block-local variable entity that decision 4 removes (`source/ast/ts/calls.go` `localFuncs`). If one does, decision 4 is revised before code.

## 3. Guard (additive; lands first and fails)

- [ ] 3.1 `source/ast` test: per-class fixtures (Java overloads, TS scoped consts, Go `_`) fail on duplicate IDs today. Proven by `TestParseResultIDsAreUnique` failing before 4.x.
- [ ] 3.2 `processor/ast-source`: dedup by ID before `bodiesForResult`, keeping the first; `id_collisions` status field; one aggregated warning per file. Proven by a unit test that feeds a result with a duplicate and asserts one publish, one body, and a count of 1.

## 4. Identity fixes (breaking; one storage break)

- [ ] 4.1 Java erased-parameter-type discriminator on every method and constructor. Test: stable under a sibling-overload add and a parameter rename.
- [ ] 4.2 Java call edges select an overload by arity, or emit none; constructor calls likewise. Tests are the three code-call-graph scenarios.
- [ ] 4.3 TS/JS/Svelte: stop emitting function- and block-local variable declarations; scope colliding nested function declarations by enclosing symbol. Test: the `fetcher` scenario, with module-level IDs unchanged.
- [ ] 4.4 Go: skip `var _` declarations. Test: the blank-declaration scenario.
- [ ] 4.5 `*.min.js` / `.min.js`-sibling default exclusion plus `include_vendored`. Tests are both ast-source-configuration scenarios.
- [ ] 4.6 3.1 passes on all fixtures, and the repo-tree guard reports zero collisions.

## 5. Acceptance

- [ ] 5.1 Probe reruns: zero collisions on dogfood and OSH, and zero hybrids from `/tmp/hybrid`-style replay. Numbers recorded on #329.
- [ ] 5.2 Fresh-seed and restart ENTITY_STATES dumps are identical apart from `dc.terms.created`. This is the #324 shape.
- [ ] 5.3 Scorecard on the reseeded store: R1 36/36 (closes #324), OSH arm not below 10/10, dogfood loose band not below the beta.10 regrade.
- [ ] 5.4 Release note and SETUP 03A record the identity break and the reseed.
