## 1. Member index and resolver state

- [x] 1.1 Add a `classMembers` type holding a class's declared method names,
      constructor presence, and `extends`/`implements` simple names; extract it
      from a parsed Java file for every type declaration at any nesting depth
- [x] 1.2 Add a `relPath -> classMembers` cache to `Parser`, validated by content
      hash so an edited file is re-parsed rather than served stale (D2 — the
      per-ParseFile reset first built cost 7x wall-clock and was replaced)
- [x] 1.3 Add a lookup that resolves a simple type name to `(relPath, classMembers)`
      through the existing `resolveJavaType`, returning inert for `external:`,
      builtin, and unresolvable names

## 2. Receiver typing

- [x] 2.1 Build the per-class field name → declared-type map once per class body
- [x] 2.2 Build the per-method receiver-type table: fields, then formal parameters,
      then locals from `local_variable_declaration`, enhanced-`for`,
      `catch_formal_parameter`, and try-with-resources
- [x] 2.3 Mark a name declared with more than one distinct type in one method as
      conflicted and resolve it to nothing (D1)
- [x] 2.4 Strip generics and array dimensions to the base type name before
      resolution (`List<Foo>` → `List`, `Foo[]` → `Foo`)

## 3. Call-site resolution

- [x] 3.1 Walk method and constructor bodies for `method_invocation` and
      `object_creation_expression`, deduping callee IDs per body
- [x] 3.2 Resolve bare `foo()` and `this.foo()` against the enclosing class's
      declared methods, building the ID with the same scope chain the definition uses
- [x] 3.3 Resolve `x.foo()` where `x` is in the receiver-type table: resolve the
      type, confirm the member, emit the scoped method ID
- [x] 3.4 Resolve `Type.foo()` where `Type` is not a known variable and resolves
      in-tree, confirming the member before emitting
- [x] 3.5 Resolve `new Type(...)` to `Type`'s constructor entity, and only when an
      explicit constructor is declared (D4)
- [x] 3.6 Emit `external:` markers for receivers typed by out-of-tree imports,
      matching the Go and Python convention
- [x] 3.7 Leave chained receivers, `super.`, `var`, literals, undeclared names, and
      ambiguous type names inert
- [x] 3.8 Set `Calls` on the method/constructor entity in `extractMethod` and
      `extractConstructor`

## 4. Ancestor walk

- [x] 4.1 Walk `extends`/`implements` breadth-first to the nearest ancestor that
      declares the method, resolving each ancestor name via `resolveJavaType`
- [x] 4.2 Guard with a visited set and a depth cap so a cyclic or deep hierarchy
      terminates
- [x] 4.3 Drop the call when more than one ancestor at the same depth declares the
      method, rather than choosing arbitrarily (D3)

## 5. Tests

- [x] 5.1 Unit tests for each resolution path in section 3, asserting the emitted ID
      is byte-identical to the callee definition's own ID (not merely non-empty)
- [x] 5.2 Inert-case tests: chained, `super`, `var`, ambiguous type name, conflicted
      receiver name, resolved type without the method, and default-constructor `new`
- [x] 5.3 Ancestor-walk tests: superclass method, interface method, tie dropped,
      cycle terminates, chain leaving the corpus
- [x] 5.4 Determinism test: parse a fixture tree twice in opposite file order and
      assert identical `Calls` sets
- [x] 5.5 Watch-safety test: parse, add a method to the callee file, re-parse with
      the same parser, and assert the edge appears — proving the content-hash
      validation defeats a stale cached member set
- [x] 5.6 Verify the regression tests by mutating the implementation and confirming
      each fails; a guard that passes vacuously is not a guard

## 6. Acceptance on the real corpus

- [x] 6.1 Run the parser over the full OSH Core tree; report resolved-edge count,
      the share of call sites resolved, and parse failures
- [x] 6.2 Compare against the probe's predicted 34.0% and explain any material gap
      rather than adjusting the target to match — the 27% shortfall found was
      multi-module source roots, fixed rather than explained away (see 8.x)
- [x] 6.3 Measure Java ingest wall-clock before and after on the same tree (D2 risk)
- [x] 6.4 Boot a live stack on OSH Core and confirm `code_impact` returns non-empty
      callers for a known Java method, with Go as the control — the same method that
      exposed the C/C++ gap
- [x] 6.5 Confirm a known-unresolvable case (a chained or third-party call) returns
      no edge on the live stack, so the fail-inert boundary is verified and not
      assumed

## 7. Documentation

- [x] 7.1 Update `docs/design/code-edges-by-language.md`: Java `Calls` → ✅ with the
      measured rate and denominator, the resolution strategy, and what stays unresolved
- [x] 7.2 Record the Java overload ID collision (1,164 declarations, 7.1%) in that
      page's debt list, including why it is not fixed here and how it interacts with
      call resolution (D6)
- [x] 7.3 Re-rank the debt list now that Java's row is filled — TypeScript, C, and
      C++ call edges remain

## 8. Multi-module source roots (added on evidence, not planned)

- [x] 8.6 Bind an ancestor's `extends`/`implements` names using THAT file's own
      import map, not the parsed file's — found by review, and worth +591 edges

- [x] 8.1 Probe sibling module source roots in `fqnToRelPath`, discovered by
      bounded globbing and sorted so enumeration order cannot matter
- [x] 8.2 Keep the referrer's own source root authoritative; resolve only when
      exactly one sibling root matches
- [x] 8.3 Distinguish "ambiguous in-tree" from "external", so an in-repo type is
      never reported as third-party
- [x] 8.4 Cover main→main, test→main, and same-FQN-in-two-modules with tests
- [x] 8.5 Measure the effect on pre-existing hierarchy edges (53.9% → 70.0%)
