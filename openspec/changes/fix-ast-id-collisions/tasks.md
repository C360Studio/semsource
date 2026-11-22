# One entity ID per declaration — tasks

## 1. Approval

- [x] 1.1 Owner approved `proposal.md` and `design.md` (2026-11-22, sha256:d24556775c82; line entered by claude at the owner's instruction).
- [x] 1.2 Owner rulings 2026-11-22, all four at the design's defaults:
  - **Q1:** Java edges use arity only and stay conservative past external ancestors. Task 2.2's numbers are recorded, but they don't reopen Q1 without a new ruling.
  - **Q2:** test-file locals are dropped.
  - **Q3:** vendored twins become file-entity-only, with opt-in via `index_vendored_js`. This refines the #334 "exclude" ruling.
  - **Q4:** `func init()` is skipped, keeping rule 1 intact.

## 2. Verify before code

- [x] 2.1 Ran the #333 collision probe (python, c, cpp) over two local polyglot trees, 2026-11-22:
  - **Tree 1:** 157k entities, 1,289 colliding IDs.
  - **Tree 2:** 728k entities, 12,657 colliding IDs.
  - **After removing third-party paths** (`.venv`, `site-packages`, uv cache, build output): tree 1 has 0, and tree 2 has 711, all C/C++ in a vendored `llama.cpp` checkout and CMake probe files. Neither tree has first-party Python.
  - **Classes seen:**
    - Python property getter/setter pairs, e.g. `_pytest/_code/code.py:631,638` `traceback`;
    - C/C++ alternative `#if`/`#ifdef` definitions (`ARCHITECTURE_ID`, `move_cursor`);
    - C++ overloads in a `.h` routed to the C parser (`common.h:1088,1091` `add`).
  - The runtime guard covers all of them. Fixes are out of this break; follow-up is #338.
- [x] 2.2 Measured on OSH core, 2026-11-22. The probe reproduced today's edges exactly: 23,319 `calls` edges from 29,373 resolved call sites, 3,329 of them constructors.

  | Outcome | Sites | Share |
  |---|---|---|
  | (a) kept | 23,374 | 79.6% |
  | (b) same-arity ambiguity | 2,043 | 7.0% |
  | (c) hierarchy leaves the repo | 3,914 | 13.3% |
  | (d) no arity match (a wrong edge today) | 6 | 0.02% |
  | (e) override written differently | 36 | 0.1% |

  - Of the (c) sites, 3,318 hit an import outside the repo and 596 hit an unresolved super (`Exception`, `Iterable`, …).
  - Checking only the in-repo hierarchy would keep 3,616 of the (c) sites: about 12.3 points.
  - 18,782 of 23,319 edges are kept (80.5%). 1,549 of 8,564 callers (18.1%) lose every outgoing edge.
  - Top (b) callees: `Asserts.checkNotNull` 315, `getStringValue` 138, `new OWSException` 81.
  - **Owner re-ruled Q1, 2026-11-22: option B.** When the receiver's hierarchy leaves the repo, the call still links if exactly one **in-repo** candidate accepts the argument count. This **amends design decision 3** (its "or when the ancestor chain leaves the repo … no edge" clause) and the code-call-graph delta. Accepted risk: an external ancestor that declares a same-named method of the same arity, a risk today's name-only edges already carry. The expected keep rate is about 92%. Same-arity overloads (7.0%) still emit no edge. Task 5.3 confirms OSH at 10/10 or better.
- [x] 2.3 **The claim holds** on dogfood, measured 2026-11-22.
  - Dogfood has 475 local entities (330 distinct IDs): 352 TS const, 67 JS const, 31 var, 13 Svelte const, 10 TS function.
  - The only statements pointing at them are `code.structure.contains` from their own file. Zero `calls`, references, `extends`, `implements`, `parameter`, `returns` or `belongs` edges come from a non-local subject.
  - Destructured-binding locals receive no inbound edges, and no non-local ID changes.
  - Caveat: `extractLocalKinds` (`ts/imports.go:118`) lets a local's `returns`/`parameter` edge target a local class. Both ends are dropped together, so the claim still holds. Task 4.3 gets a fixture for it.

## 3. Guard (additive; lands first and fails)

- [x] 3.1 `source/ast` test: per-class fixtures (Java overloads, TS scoped consts, Go `_`) fail on duplicate IDs today. Proven by `TestParseResultIDsAreUnique` failing before 4.x. Done 2026-11-22 (`source/ast/idcollision_guard_test.go`, fixtures in `source/ast/testdata/idcollisions/`). It fails on all three classes, and `TestRepositoryTreeIDsAreUnique` fails on dogfood. **Both are expected red until 4.6**, so the draft's CI `test` job is red until then.
- [x] 3.2 `processor/ast-source`: dedup by ID before `bodiesForResult`, keeping the first, plus the cross-file check over the initial index; `id_collisions` status field; one aggregated warning per file. Proven by a unit test that feeds a result with a duplicate and asserts one publish, one body, and a count of 1. The status field is reviewed by `go-component-reviewer`. Done 2026-11-22 (`processor/ast-source/idguard.go`, `idguard_test.go`). A mutant that removes the per-result call fails the test (2 publishes, not 1).

## 4. Identity fixes (breaking; one storage break)

- [x] 4.1 Java discriminator (kind, type text without type arguments, array dimensions kept, type variables to their first bound) on every method and constructor. Tests: the constructor-versus-method and `int`/`int[]` scenarios, plus stability under a sibling-overload add and a parameter rename. Done 2026-11-22 (`source/ast/java/overloads.go`, `overloads_test.go`).
- [x] 4.2 Java call edges collect overloads across the in-repo hierarchy and select by arity even with one candidate. A unique in-repo match links even when the chain leaves the repo (Q1, option B); otherwise no edge. Constructor calls likewise. Tests are the six code-call-graph scenarios. Done 2026-11-22.
  - **OSH Java `calls` edges:** 23,319 → 21,432 (91.9% kept; option B expected ~92%), zero duplicate IDs, zero dangling edges.
  - **Also fixed:** the per-`ParseFile` sibling source-root memo is now keyed by layout. A cross-package ancestor's empty answer used to hide in-repo modules for the rest of the file (`TestForeignAncestorDoesNotHideOtherModules`).
  - **Stricter than javac:** `f(T, Object)` alongside `f(T, String, Object...)` counts as ambiguous at 2 arguments, per the spec's "unique candidate" rule.
- [x] 4.3 TS/JS/Svelte: stop emitting locals of every form (namespace members kept); always discriminate accessor pairs and static members, and carry the discriminated ID through `classMethodNames` for `this.` calls. Tests: a `this.` call from an instance method reaches the instance member, and from a static method the static member, plus the `fetcher`, local-function and accessor scenarios, with module-level IDs unchanged. Done 2026-11-22 (`source/ast/ts/scope_identity_test.go`, `source/ast/svelte/scope_identity_test.go`). Mutants that disable the local rule fail both tests. Review (graph-event-reviewer) corrections:
  - Design decision 4's "a static method is reached through `ClassName.m()`" is wrong. `ClassName.m()` never resolved, before or after this change (`ts/calls.go:605-610` handles namespace imports only), so it stays out of scope.
  - IIFE, AMD and UMD-wrapped JS now emits only its file entity, because everything inside the wrapper function is local. **Owner accepted this on 2026-11-22**; the release note names it (5.4). The guard's `ts` fixture and the repository-tree pass now hold.
- [x] 4.4 Go: skip blank `var`, `const` and `func`, and identify `init` per Q4. Test: the blank-declaration scenario, plus an `init` test under the ruling. Done 2026-11-22 (`source/ast/golang/blank_decls_test.go`). A method named `init` is still emitted, and `type _` is skipped too (review). Functions called only from `init` lose that caller; the release note has to name it (5.4).
- [x] 4.5 Asset guard: an `X.js` with an `X.min.js` sibling is file-entity-only, plus `index_vendored_js`. Tests are both asset-ingestion-guards scenarios. Done 2026-11-22.
  - `isVendoredTwin` lives in `processor/ast-source/assetguard.go`, tested by `vendored_twin_test.go`.
  - The opt-in flows from `semsource.json` `index_vendored_js` through `internal/sourcespawn` to the watch path (`vendored_test.go`).
  - The watch-time twin limit is marked `// shortcut:`.
- [x] 4.6 3.1 passes on all fixtures, and the repo-tree guard reports zero collisions. Done 2026-11-22 at `c45cac9`: `TestParseResultIDsAreUnique` (java, ts, golang) and `TestRepositoryTreeIDsAreUnique` pass with no skips, and `go test ./...`, `task lint` and `task spec:check` are green. The 18 new or changed tests now carry `// spec:` citations. They cite `openspec/specs/`, so the 17 that point at this change's ADDED requirements resolve only after archive; `scripts/ateam test-spec` is red on them until then.

## 5. Acceptance

- [x] 5.1 Probe reruns at `6fc7e40`, 2026-11-22. The probe uses the branch's parsers over go/ts/js/svelte/java, models the asset guard's name and twin rules, and replays through `graph.ReplaceBySource`.

  | Corpus | Entities (before → after) | Colliding IDs, in-file or anywhere (before → after) | Hybrids (before → after) |
  |---|---|---|---|
  | dogfood | 6,888 → 6,353 | 56 → **0** | 0 → **0** |
  | OSH core | 77,405 → 27,903 | 2,264 → **0** | 233 → **0** |
  | this branch's own tree | 6,593 | **0** | **0** |

  - OSH skips 14 vendored files under the guard: 10 `.min.js` and 4 twins.
  - Sensitivity: the branch tree includes the `testdata/idcollisions` fixtures, which collide under `main` and are now clean.
  - Numbers posted on #329.
- [x] 5.2 Fresh-seed and restart ENTITY_STATES dumps are identical apart from `dc.terms.created`. This is the #324 shape. Done 2026-11-22 at `396123d`: a local build on the beta.10 dogfood corpus with the beta.10 `tier0.json`, in compose project `s52` on ports 28080/25222. Three legs: a fresh seed, a restart of `semsource` alone on the same NATS volume, and a fresh seed with `tier0-reversed.json`. Every `ENTITY_STATES` key and value was dumped read-only.

  | Comparison | Keys | Entities differing in content |
  |---|---|---|
  | fresh vs restart | 7,828 = 7,828, same platform `semsource-d7623c` | **0** |
  | fresh vs reversed source order | 7,828 = 7,828, same key set once the per-store platform segment is stripped | **0** |

  - Content means each entity's triples by (subject, predicate, object, source), with `dc.terms.created` excluded. Every differing triple was `dc.terms.created`: 12,954 per comparison, and nothing else.
  - The excluded fields are write-time metadata, not content: per-triple `timestamp` and the entity's `updated_at`.
  - Observed, not judged: a fresh seed leaves `updated_at` at `0001-01-01T00:00:00Z` on all 7,828 entities, and a restart sets it on all 7,828. That is SemEngine graph-ingest behaviour. It does not gate 5.2.
  - Ranking is out of scope for this check; 5.3 measures it (R1).
- [x] 5.3 Scorecard on the reseeded store: R1 36/36 (closes #324), OSH arm not below 10/10, dogfood loose band not below the beta.10 regrade. Done 2026-11-22 at `08227da`; see `scripts/scorecard/results/SUMMARY-fix329-tier0.md`.
  - R1 36/36 identical, RANK-STABLE (beta.10: 35/36).
  - OSH 13/13 (beta.10: 12/13; C03 now correct, recorded and not claimed).
  - Dogfood 32/36 with every verdict identical to beta.10, and the loose band 5/7 = beta.10.
  - The first OSH leg at `396123d` caught a regression: P01 lost its three callers, because an inherited field's type was resolved with the parsed file's package. Fixed in `08227da` (`packageFor`, `TestInheritedFieldTypedInAnotherModuleLinks`) and rerun.
- [x] 5.4 Release note and SETUP 03A record the identity break and the reseed, plus the visible losses (done 2026-11-22: `docs/migration/ast-entity-identity.md`, the ROADMAP "Recently Shipped" entry, and the SETUP 03A section "Post-beta.10 identity break"; the note also names Go blanks, vendored twins and `index_vendored_js`):
  - IIFE/AMD-wrapped JS indexes as files only;
  - `init` callers are gone;
  - same-arity Java overload calls have no edge;
  - test-file locals are no longer entities.
