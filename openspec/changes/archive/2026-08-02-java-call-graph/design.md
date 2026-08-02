## Context

See `proposal.md` — Why. The constraint that shapes everything below is the rule
carried over from the C++ inheritance work: **never emit a wrong edge.** An
ambiguous or unconfirmable target is dropped, not guessed, because `code_impact`
reporting a dependent that no compiler agrees with is a confident falsehood, while
a missing edge is a known gap.

What already exists and is reused unchanged:

- `resolveJavaType(name, fromRelPath)` → the defining file of a simple type name,
  via import binding first and same-package sibling layout second, returning an
  `external:` FQN when the import leaves the tree (`source/ast/java/imports.go`).
- `fqnToRelPath` / `sourceRootPrefix`, which make `a.b.C` → `src/main/java/a/b/C.java`
  deterministic.
- `NewScopedCodeEntity`, which builds a method's ID from `(org, java, project,
  kind, scope-chain, name, relPath)`. A call edge must be built through this same
  path or it dangles.
- Per-`ParseFile` resolver state on `Parser` (`pkg`, `importMap`, `localKinds`),
  refreshed at the top of `ParseFile`.

The Python parser (`source/ast/python/calls.go`) is the shape to follow: walk the
body, resolve each call site, confirm the target actually declares the callee, and
emit nothing otherwise.

## Goals / Non-Goals

**Goals:**

- Resolve the receiver-typed, intra-class, static, and constructor call shapes to
  entity IDs that byte-match their definitions.
- Keep resolution a pure function of source-tree content — no parse-order
  dependence, no cross-`ParseFile` cache that can go stale under watch.
- Make every drop deliberate and named, so the gap is documented rather than
  discovered.

**Non-Goals (design-level, beyond the proposal's):**

- No full lexical scope tree for method bodies. Measured below: it buys 0.5%.
- No expression type inference of any kind, which rules out chained calls and
  `var`.
- No change to Java entity identity, including the overload collision — see
  Decision 6.

## Decisions

### D1 — Type receivers from declarations, with a flattened scope and a conflict drop

A receiver-type table maps a simple name to its declared type, populated from
class fields, then formal parameters, then every local declaration anywhere in the
body (`local_variable_declaration`, enhanced-`for`, `catch`, try-with-resources).
Block nesting and shadowing are **ignored**; instead, a name declared with **more
than one distinct type** within one method is marked conflicted and made inert.

*Why not a real scope tree?* Because the flattened table is only unsafe when one
name carries two types in one method, and that is measurably rare: **337 of 71,262
call sites (0.5%)** on OSH Core. Dropping those 337 removes the entire class of
mis-typed receivers for a few lines of code, where a correct scope tree would be a
substantial subsystem. The trade is 0.5% recall for 100% of the risk.

*Rejected:* resolving shadowing by declaration position (a local shadows a field
only after its own line). It is more code, still approximate around control flow,
and recovers a fraction of 0.5%.

### D2 — Confirm the callee against the target file's declared members

An edge is emitted only after the resolved target file is parsed and confirmed to
declare a method of that name. This is the fail-inert guard, and it is what
separates "resolved" from "guessed": a field typed `Repo` whose `load()` does not
exist on `Repo` yields nothing.

Member sets are read through a cache keyed by `relPath`, holding each class's
declared method names, constructor presence, and `extends`/`implements` names —
one small parse per referenced file.

**The cache is validated by content hash, not scoped to one `ParseFile`.** The
first build followed the Python parser's per-`ParseFile` reset, and it cost 7x
parse wall-clock on OSH Core (1.46s → 10.5s) because every file re-parsed each
file it referenced. Hash validation gets the same guarantee more directly: each
lookup re-reads the file and compares hashes, so edited content — which cannot
hash equal — always forces a re-parse, while the expensive tree-sitter pass is
shared across the run. Combined with a per-`ParseFile` memo of type-name
resolutions, that lands at 7.14s with byte-identical output.

The member set a class exposes mirrors `parser.go` **exactly**: enum and record
bodies contribute nothing, because `extractEnum` and `extractRecord` create no
member entities. Naming an enum method as a call target would emit an edge to an
entity that does not exist — a dangling edge, caught by mutation testing.

*Rejected:* a corpus-wide post-parse index like `cpp.ResolveTypeRefs`. C++ needs
one because a name does not imply a path there; Java's package↔path determinism
makes a per-file probe sufficient, and the probe keeps resolution local, so it
composes with incremental watch re-parses instead of requiring a whole-tree pass.

### D2a — Multi-module source roots (added on evidence)

Discovered by measuring, not by design review: 4,682 call targets were marked
`external:` while naming an in-repo package, because `fqnToRelPath` probed only
the referrer's own source root and OSH Core has **15**. Sibling roots are now
probed too, discovered by bounded globbing (`*/src/*/java`, `*/*/src/*/java`)
rather than a tree walk, sorted so enumeration order cannot matter, and memoized
per `ParseFile`.

The middle path segment stays a wildcard so a referrer under `src/test/java`
reaches `src/main/java` — without that, tests resolved almost nothing, which is
what the residual 1,810 markers turned out to be.

A name found under several roots is **ambiguous, not external**. Conflating the
two would report an in-repo type as third-party; the two states are now
distinguished so an ambiguous name emits nothing at all. That distinction was
found by tightening a test to assert the *exact* edge set — see Risks.

### D3 — Ancestor walk, breadth-first, ties dropped

When the receiver's own class does not declare the method, walk `extends` and
`implements` breadth-first to the nearest declaring ancestor. Cycle-safe via a
visited set, depth-capped, and each ancestor name resolved through the same
`resolveJavaType`, so an unresolvable or ambiguous ancestor terminates that branch.

If **more than one ancestor at the same depth** declares the method, the call is
dropped rather than resolved to an arbitrary one. Measured cost: **27 ties across
the entire corpus**. Benefit of the walk overall: **+5.5 percentage points** of all
call sites (28.5% → 34.0%).

This is the one place where a modest amount of extra machinery buys a real share of
the graph, which is why it is in scope rather than deferred.

### D4 — Constructor edges target the explicit constructor entity

`new Type(...)` resolves `Type`, and emits an edge only when that file declares an
explicit constructor — because an implicit default constructor has no entity to
point at, and inventing one would be a dangling edge. Measured: 3,604 of 7,822
`new` sites (46%) qualify.

Constructors are already extracted as `TypeMethod` entities named for the class and
scoped `[Class]`, so the target ID is built exactly as `extractConstructor` builds
it.

### D5 — Where the walk hangs off the parser

`extractMethod` and `extractConstructor` gain a body walk that returns callee IDs,
mirroring `extractFunctionCalls` in the Go parser. The receiver-type table is built
once per method, from the enclosing class's field map (computed once per class
body) plus that method's parameters and locals.

The per-file state added to `Parser` — the class-member memo and the current class
field map — follows the existing per-`ParseFile` refresh convention, and is
serialized by `ast-source`'s existing `parseFileWithWatcher` lock, so no new
concurrency surface appears.

### D6 — Java entity IDs stay name-level; the overload collision is recorded, not fixed here

Probing OSH Core surfaced a pre-existing defect: **1,164 method declarations (7.1%)
collide with a same-named sibling on `(class, name)`**, so overloaded Java methods
already share one entity ID today, before any call edge exists. This is the same
defect PR #121 fixed for C++ with a parameter-type discriminator.

It is deliberately **not** fixed in this change, and the ordering matters. Adding a
discriminator to Java IDs would mean every call site had to perform real overload
resolution — argument count and static argument types — to name a target. That is a
type-inference problem, precisely what this design excludes. Shipping call edges
first against name-level IDs means a call resolves to "some overload of `foo`",
which is the honest answer available without inference; a later identity change can
then be scoped with call edges as a known consumer.

Recorded as debt in `docs/design/code-edges-by-language.md` so it is a tracked
decision rather than an oversight.

## Risks / Trade-offs

- **Cross-file confirmation slows Java ingest** → each referenced file is parsed at
  most once per `ParseFile` cycle, and only member names are kept. Ingest wall-clock
  on the full OSH Core tree is measured before and after, and reported; if the cost
  is disproportionate the memo can be widened, but not past the `ParseFile` boundary
  without giving up watch correctness.
- **Recall looks low in isolation (34%)** → the honest framing is that it is 34% of
  *all* call sites including the 17.2% chained and 18.4% third-party buckets that
  are not resolvable in principle without inference or out-of-tree indexing. The
  number to publish in `code-edges-by-language.md` is the measured one, with the
  denominator stated, not a rounded impression.
- **A wrong edge from a mis-typed receiver** → the conflict drop (D1), the ambiguity
  drop, and the member confirmation (D2) each independently prevent it. The
  acceptance run asserts a known-empty case stays empty, not only that known edges
  appear.
- **Ancestor walk on a deep or cyclic hierarchy** → visited set plus depth cap; a
  cycle terminates rather than looping.
- **A green suite proving nothing** → every prior change in this area had defects
  invisible to passing tests, and this one was no exception. All 20 tests passed on
  first run; mutating the implementation then showed **three of them were
  vacuous**. The shared cause was an assertion of "no edge to a real entity",
  which happily tolerates a **dangling** edge — the exact failure that matters.
  Rewriting those to assert the *exact* edge set immediately exposed a live bug:
  an ambiguous in-tree type was being emitted as an `external:` marker. Every
  guard is now mutation-verified, and one mutation that "passed" turned out to be
  a no-op against the real AST shape (enum methods sit under
  `enum_body_declarations`, not directly under `enum_body`) — a reminder that a
  mutation must be confirmed to actually change behavior before its result means
  anything.

## Migration Plan

None required. `Calls` is an existing field on an existing payload with an existing
predicate (`code.relationship.calls`), already published and already rendered by
`code_context` and `code_impact`. This change only populates it for a language where
it was empty, so consumers see more edges with no contract change and nothing to
roll back beyond the parser change itself.

## Open Questions

None that affect the specs, the approach, or the task breakdown. The one number not
yet known — Java ingest wall-clock delta — is a measurement task, not a design
fork; it changes what is documented, not what is built.
