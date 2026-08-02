# Java call-graph extraction

## Why

`code_impact` answers *"what breaks if I change this?"* — most of why SemSource
exists — but it emits call edges for **Go and Python only**. For Java it answers
from type hierarchy alone, and the failure is invisible: a language with no call
edges returns a clean, confident, **empty** answer rather than an error. Java is
the largest corpus in the driver-authoring A/B, so this is the single largest gap
between what the tool appears to do and what it does.

Java is also the cheapest of the four missing languages to fix. It has mandatory
declared types and the package↔path determinism that `code-reference-resolution`
already exploits, so a receiver's type is knowable from declarations alone —
no type inference, no build system, no classpath.

## What Changes

- The Java parser walks method and constructor bodies for call sites and sets
  `Calls` to the callee entity IDs it can **confirm**, reusing the existing
  `resolveJavaType` / `fqnToRelPath` resolver from `code-reference-resolution`.
- A per-method **receiver-type table** is built from declarations only — class
  fields, formal parameters, local variables, enhanced-`for` variables, `catch`
  parameters, and try-with-resources. This is the core of the change: 46% of Java
  call sites invoke through a typed variable.
- Four resolution paths, each of which must confirm the target declares the
  method before an edge is emitted:
  - bare `foo()` and `this.foo()` → the enclosing class;
  - `x.foo()` where `x` has a declared in-tree type → that type's class;
  - `Type.foo()` where `Type` resolves in-tree → static call;
  - `new Foo(...)` → `Foo`'s explicit constructor entity.
- An **ancestor walk** resolves calls to inherited methods: when the receiver's
  class does not declare the method, its `extends`/`implements` chain is followed
  to the declaring ancestor. Depth-capped, cycle-safe, and ambiguity-dropping.
- Out-of-tree callees become `external:` markers, matching Go and Python.
  Everything else — chained calls, `super.`, `var`-typed receivers, static
  imports, lambdas, ambiguous type names — emits **nothing**.
- `docs/design/code-edges-by-language.md` moves Java's `Calls` column to ✅ with
  the measured resolution rate, and records what remains unresolved.

Not breaking: this only populates a field that is currently empty for Java.

## Capabilities

**New Capabilities**: none.

**Modified Capabilities**:

- `code-call-graph` — currently seeded for Python with Go-specific requirements
  appended. This change adds the Java requirements: declaration-derived receiver
  typing, inherited-method resolution through the type hierarchy, constructor
  call edges, and the fail-inert boundary for Java's unresolvable shapes.
- `code-reference-resolution` — **added during implementation, on evidence.** The
  original scoping said this capability was reused unchanged. Measuring the first
  working build falsified that: 4,682 call targets were marked `external:` while
  naming an *in-repo* package. OSH Core is a multi-module Gradle repo with 15
  source roots, and `fqnToRelPath` probed only the referrer's own. That is a
  pre-existing defect — it degrades Java `extends`/`implements`/`references`
  today, independent of call edges — so the fix belongs to this capability, and
  is specced here rather than smuggled in as an implementation detail.

## Measurement that scoped this

Probed on OSH Core (`opensensorhub/osh-core`, 1,932 Java files, 313k LOC,
**71,262 call sites**) by classifying every `method_invocation` by receiver shape
and testing whether the callee could be confirmed:

| receiver shape | share of call sites |
| --- | ---: |
| typed variable (`x.foo()`) | 46.0% |
| bare / `this.` | 19.9% |
| chained (`a().b()`) | 17.2% |
| identifier that is not a variable (static call) | 10.4% |
| field access, `super.`, literals, casts | 6.5% |

Confirmed-target resolution reaches **28.5%** of all call sites with direct
declarations only, and **34.0%** with the ancestor walk. Separately, 3,604 of
7,822 `new X(...)` sites (46%) target an in-corpus class with an explicit
constructor. Intra-class and static calls *without* the receiver-type table would
reach only ~13%, which is why the table is the change rather than an optimisation
of it.

The 17.2% chained bucket is the largest thing left unresolved; it needs
return-type propagation and is a non-goal here.

### What the implementation actually produced

Measured with the same corpus, parsing all 1,932 files (0 parse failures):

| | before | after |
| --- | ---: | ---: |
| resolved call edges | 0 | **23,566** |
| dangling call edges (targets no entity) | — | **0** |
| `external:` markers naming an in-repo package | — | 157 (1.9%) |
| callables carrying ≥1 call edge | 0 | 9,810 (54.3%) |
| resolved `extends`/`implements` edges | 1,136 (53.9%) | **1,475 (70.0%)** |
| parse wall-clock | 1.46s | 8.40s |

The hierarchy row is the pre-existing defect being repaired: +339 edges that this
change did not create and was not originally scoped to fix.

Live-stack acceptance on `lib-ogc/swe-common-core` (6,317 entities, `phase: ready`
and `index: ready`): `code_impact` on `addToProcessorTree` returns 11 callers
across 10 files, and on `errorLocationString` — whose calls are all third-party —
returns no `callee` relation at all. Both previously returned empty.

## Impact

- **Code**: `source/ast/java/` — a new `calls.go` plus per-file resolver state on
  `Parser`; `extractMethod` / `extractConstructor` gain a body walk. Other files'
  class member sets are cached and **validated by content hash**, so an edited
  file is always re-parsed while the expensive tree-sitter pass is shared across
  the run. Type-name resolutions are memoized per `ParseFile`.
- **Graph**: more `code.relationship.calls` triples on Java corpora. No new
  predicate, payload, entity type, or ID shape — `Calls` is already published and
  already rendered by `code_context` and `code_impact`.
- **Consumers**: `code_impact` and `code_context` (MCP, GraphQL, `graph.query.*`)
  return non-empty call relations for Java. SemSpec and SemDragon consume these
  through the existing contracts; no consumer change is required.
- **Cost**: cross-file confirmation parses referenced files once per `ParseFile`
  cycle. Ingest time on a large Java corpus must be measured, not assumed.
- **Docs**: `docs/design/code-edges-by-language.md` is updated in this change.

## Non-goals

- **Overload disambiguation.** Java method entity IDs are name-level, so the
  1,164 overloaded declarations measured in OSH Core (7.1% of all method
  declarations) already share an entity ID *today*, independent of this change.
  A call therefore targets "some overload of `foo`", which is the honest answer
  given no overload resolution. Adding a C++-style parameter-type discriminator
  to Java IDs would require real overload resolution at every call site to pick a
  target; that is a separate change, and this one is written so it does not
  foreclose it. Recorded as debt in `docs/design/code-edges-by-language.md`.
- **Chained calls** (`a().b()`), which need the return type of an arbitrary
  expression.
- **`var`-declared receivers** (Java 10+, 5.8% of call sites) — needs initializer
  type inference.
- **Static imports** and wildcard imports — `extractImportMap` already skips
  wildcards because they cannot bind a simple name.
- **`super.foo()`** — resolvable in principle via the ancestor walk, but only
  0.8% of call sites; left inert rather than adding a path that carries its own
  correctness risk for a fraction of a percent.
- **TypeScript, C, and C++ call edges** — same gap, different resolvers. Java
  first because it is the largest corpus and the resolver already exists.
- **A runtime counter for dropped/ambiguous references** — still absent for C++
  too; it belongs in one change that covers every language.
- Any change to SemStreams substrate: this is entirely within SemSource's
  source-parsing and entity-projection boundary.
