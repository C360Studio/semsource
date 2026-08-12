# code-call-graph Specification

## Purpose
How the AST layer extracts call sites from a function/method body and resolves
each callee to the deterministic entity ID of its definition, so
`code.relationship.calls` edges connect in the graph. Consumed by `code_context`
(caller/callee relations) and `code_impact` (call closure). Resolution is per-file
and filesystem-driven, reusing the import→module→file machinery of
`code-reference-resolution`, and FAILS INERT — a call edge is emitted only for a
confirmed target (a local module-level function, an imported top-level function
verified in its defining module, or a method defined on the current class); a
builtin, a class instantiation, an inherited/mixin method, or an attribute call on
a local variable emits nothing, never a wrong or phantom edge. Covers Go, Python,
Java, TypeScript/JavaScript, Svelte, and C, each under the per-language coverage
contract below; C++ deliberately emits none.
## Requirements
### Requirement: Function bodies emit resolved call edges
The AST layer SHALL walk each function/method body for call sites and set the
owning entity's `code.relationship.calls` to the callee entity IDs it can resolve.
A resolved callee ID MUST be built through the same `NewCodeEntity` /
`NewScopedCodeEntity` path the callee's definition uses (function/method kind,
`SystemSlug(project)` system segment), so the edge target byte-matches the
definition and is not dangling. Duplicate callees within one function are emitted
once.

#### Scenario: Same-file function call
- **GIVEN** a Python module defines `def helper(): ...` and `def run(): helper()`
- **WHEN** the module is parsed
- **THEN** `run`'s `calls` includes `helper`'s own entity ID (byte-identical to the
  `helper` definition), and the edge resolves (relations show `run` under `helper`'s `caller`)

#### Scenario: Intra-class method call
- **GIVEN** a class defines `def a(self): self.b()` and `def b(self): ...`
- **WHEN** the class is parsed
- **THEN** `a`'s `calls` includes `b`'s scoped method entity ID (same `[Class]` scope
  the `b` definition uses)

### Requirement: Imported callees resolve to their defining module
The AST layer SHALL resolve a call whose target is bound by an in-tree import to
the callee's definition in the imported module's file, using the referrer file's
import bindings and the module→file resolver. A `module.func()` call whose module
(or its head) is an import binding SHALL resolve the same way.

#### Scenario: Cross-module imported function call
- **GIVEN** `pkg/util.py` defines `def helper(): ...`
- **AND** `pkg/app.py` contains `from pkg.util import helper` and calls `helper()`
- **WHEN** `pkg/app.py` is parsed
- **THEN** the call target equals `helper`'s entity ID (built against `pkg/util.py`)

#### Scenario: Module-qualified imported call
- **GIVEN** `pkg/app.py` contains `import pkg.util` and calls `pkg.util.helper()`
- **THEN** the call target equals `helper`'s entity ID in `pkg/util.py`

### Requirement: Unresolvable and out-of-scope calls never produce a wrong edge
A call the AST layer cannot resolve to an in-tree definition SHALL NOT be mapped to
an incorrect entity. An imported out-of-tree callee SHALL be left as an `external:`
marker; a builtin call, a bare undefined name, a class instantiation, or an
attribute call on a non-`self`/`cls` receiver SHALL emit no call edge (inert),
never a fabricated in-tree target.

#### Scenario: Stdlib / third-party call stays external
- **WHEN** a call targets a function imported from a module that does not resolve
  within the source root (e.g. `import os; os.getcwd()`)
- **THEN** the call is left as an `external:` marker and no edge to an in-tree entity is emitted

#### Scenario: Builtins and unresolvable receivers are inert
- **WHEN** a body calls a builtin (`len(x)`), a bare name that is neither a local
  function nor an import, or a method on a local variable (`obj.method()`)
- **THEN** no `code.relationship.calls` edge is emitted for that call (never a wrong target)

### Requirement: Go same-package calls resolve across files

For Go sources, unqualified calls to symbols defined in sibling files of the same package SHALL
emit resolved call edges whose target byte-matches the callee's entity ID (the resolution already
required for type references extends to calls).

#### Scenario: Cross-file same-package call

- **WHEN** `entityid/scoped.go` calls `SystemSlug` defined in `entityid/entityid.go`
- **THEN** a `code.relationship.calls` edge exists from the caller to
  the `SystemSlug` entity ID (byte-matching the definition)

### Requirement: Go in-repo cross-package calls resolve to the defining entity

For Go sources, package-qualified calls SHALL resolve to the defining entity's ID whenever the
import path maps to a module indexed from the same source root; only genuinely external imports SHALL
remain `external:` markers, and no call SHALL ever resolve to a guessed or wrong entity.

#### Scenario: In-repo qualified call

- **WHEN** `handler/git/entities.go` calls `entityid.SanitizeInstance` and both packages are
  indexed from the same source root
- **THEN** the call edge resolves to the `SanitizeInstance` entity ID

#### Scenario: External stays external

- **WHEN** a Go file calls `strings.Contains`
- **THEN** the callee remains an `external:` marker (no fabricated in-graph edge)

### Requirement: Impact reflects the resolved Go call graph

With the above resolution in place, the reverse-dependency closure for a Go symbol SHALL include
its cross-file and in-repo cross-package callers, so blast-radius answers match developer intuition
instead of same-file-only edges.

#### Scenario: SanitizeInstance blast radius

- **WHEN** code_impact is asked about `SanitizeInstance` with handler/git indexed
- **THEN** the closure includes the handler/git callers (non-zero cross-package impact)

### Requirement: Java call sites resolve through declared receiver types

For Java sources, the AST layer SHALL determine a call receiver's type from
**declarations alone** — class fields, formal parameters, local variable
declarations, enhanced-`for` variables, `catch` parameters, and try-with-resources
resources — and resolve `receiver.method()` to the entity ID of `method` as
declared on that type. The receiver's type SHALL be resolved to its defining file
by the same import/same-package machinery type references already use, and the
target file SHALL be confirmed to declare the method before an edge is emitted.
Generic type arguments and array dimensions are stripped before resolution, so
`List<Foo> x` types `x` as `List` and `Foo[] a` types `a` as `Foo`.

#### Scenario: Call through a field of an in-tree type

- **GIVEN** `Service.java` declares `private Repo repo;` and calls `repo.load()`
- **AND** `Repo.java` declares `public Data load()`
- **WHEN** `Service.java` is parsed
- **THEN** the calling method's `calls` includes `load`'s scoped method entity ID
  built against `Repo.java`, byte-identical to the `load` definition's own ID

#### Scenario: Call through a parameter and through a local variable

- **GIVEN** a method `void run(Repo repo) { Helper h = new Helper(); repo.load(); h.go(); }`
- **AND** `Repo` declares `load()` and `Helper` declares `go()`
- **THEN** `run`'s `calls` includes both `Repo.load` and `Helper.go` entity IDs

#### Scenario: Generic and array receiver types resolve to the element type

- **GIVEN** a field `private Repo[] repos;` and a local `Repo r = repos[0];`
  followed by a call `r.load()`
- **THEN** the call resolves to `Repo.load`'s entity ID

### Requirement: Java intra-class and static calls resolve to the declaring type

For Java sources, an unqualified call (`foo()`) and a `this.foo()` call SHALL
resolve against the enclosing class's own declared methods. A call qualified by a
type name that resolves in-tree (`Type.foo()`) SHALL resolve against that type's
declared methods. In both cases the callee's scoped entity ID MUST be built
through the same `NewScopedCodeEntity` path the definition uses, so the edge
target byte-matches the definition. Duplicate callees within one body are emitted
once.

#### Scenario: Unqualified call to a sibling method

- **GIVEN** a class declares `void a() { b(); }` and `void b() {}`
- **THEN** `a`'s `calls` includes `b`'s scoped entity ID with the same `[Class]`
  scope the `b` definition uses

#### Scenario: Static call to an in-tree type

- **GIVEN** `Util.java` declares `public static String slug(String s)`
- **AND** another file imports `Util` and calls `Util.slug(x)`
- **THEN** the call resolves to `slug`'s entity ID built against `Util.java`

#### Scenario: The same callee invoked twice yields one edge

- **WHEN** a method body calls `helper()` on three separate lines
- **THEN** `calls` contains that callee entity ID exactly once

### Requirement: Java calls to inherited methods resolve to the declaring ancestor

For Java sources, when a receiver's resolved type does not itself declare the
invoked method, the AST layer SHALL follow that type's `extends` and `implements`
chain and resolve the call to the **nearest ancestor that declares it**. The walk
SHALL be depth-capped and cycle-safe, and SHALL stop without emitting an edge when
an ancestor's name cannot be resolved unambiguously to one in-tree file.

#### Scenario: Method inherited from a superclass

- **GIVEN** `Base.java` declares `void start()` and `Impl extends Base` does not
- **AND** a caller holds `Impl x` and calls `x.start()`
- **THEN** the call resolves to `start`'s entity ID built against `Base.java`,
  not against `Impl.java`

#### Scenario: Method declared on an implemented interface

- **GIVEN** `IModule.java` declares `void init()` and `Module implements IModule`
  declares it too
- **AND** a caller holds `IModule m` and calls `m.init()`
- **THEN** the call resolves to `IModule.init` — the declaration named by the
  receiver's declared type, not every implementor

#### Scenario: Ancestor chain that leaves the corpus

- **GIVEN** a receiver typed as a class whose superclass is a third-party type not
  present in the source tree, and the method is declared by neither
- **THEN** no call edge is emitted for that call site

### Requirement: Java constructor calls emit edges to the constructor entity

For Java sources, `new Type(...)` SHALL emit a call edge to `Type`'s constructor
entity when `Type` resolves to an in-tree file that declares an explicit
constructor. A `new Type(...)` whose target declares no explicit constructor (the
implicit default constructor, which has no entity) SHALL emit no edge.

#### Scenario: Instantiating a class with an explicit constructor

- **GIVEN** `Repo.java` declares `public Repo(String path)`
- **AND** a method body contains `new Repo("/tmp")`
- **THEN** the calling method's `calls` includes the `Repo` constructor's entity ID

#### Scenario: Instantiating a class with only a default constructor

- **GIVEN** `Plain.java` declares no constructor
- **WHEN** a body contains `new Plain()`
- **THEN** no call edge is emitted (no constructor entity exists to target)

### Requirement: Unconfirmed Java calls never produce a wrong edge

For Java sources, a call the AST layer cannot confirm SHALL NOT be mapped to an
entity. A callee whose receiver type is bound by an import that resolves outside
the source tree SHALL be left as an `external:` marker. A call SHALL emit nothing
when the receiver is a chained call, `super`, a `var`-declared variable, an
undeclared name, or a literal; when the receiver's type name matches more than one
in-tree definition; or when the resolved target and its ancestors declare no method
of that name.

#### Scenario: Third-party receiver stays external

- **WHEN** a body calls `logger.info(msg)` where `logger` is typed by an import
  that resolves outside the source tree
- **THEN** the callee is an `external:` marker and no in-tree entity edge is emitted

#### Scenario: Ambiguous type name is dropped

- **GIVEN** two files in the source tree both declare a class named `Config`
- **AND** a receiver is declared with type `Config` and the name cannot be bound to
  one file by import or same-package layout
- **THEN** no call edge is emitted for calls through that receiver

#### Scenario: Chained, super, and var receivers are inert

- **WHEN** a body contains `getRepo().load()`, `super.start()`, or
  `var r = make(); r.load()`
- **THEN** no `code.relationship.calls` edge is emitted for those call sites

#### Scenario: Resolved type that does not declare the method

- **WHEN** a receiver resolves to an in-tree class, but neither that class nor any
  ancestor declares the invoked method
- **THEN** no call edge is emitted, rather than an edge to a fabricated target

### Requirement: Java call resolution is deterministic and watch-safe

Java call edges SHALL depend only on the content of the source tree, never on the
order in which files are parsed. A cached view of another file's members SHALL be
validated against that file's current content before reuse, so a parse after an
edit observes the edited file rather than a stale member set.

#### Scenario: Parse order does not change the result

- **WHEN** the same source tree is parsed twice with the file order reversed
- **THEN** every entity's `calls` set is identical between the two runs

#### Scenario: An edited callee is observed on the next parse

- **GIVEN** `A.java` calls `b.run()` and `B.java` does not yet declare `run()`
- **WHEN** `run()` is added to `B.java` and `A.java` is parsed again
- **THEN** the call edge to `B.run` is now emitted

### Requirement: Caller-edge coverage is a per-language contract

The AST layer SHALL document and honor, per supported language, which call forms produce
`code.relationship.calls` edges and which are deliberately dropped, so an empty `code_impact`
closure is interpretable as "no resolvable dependents under the stated coverage" rather than an
unstated parser gap. The contract after this change: **Go** — direct and method calls (existing),
excluding callees that bind to function-typed parameters or local variables; **Python** — the
existing seeded coverage; **Java** — class-qualified static calls, own-class/`this.` calls, and
instance calls whose receiver's declared type resolves through the supers walk; **TypeScript and
Svelte** — direct calls bound through in-file definitions or in-tree import bindings, plus
own-class/`this.` method calls; **C** — name-bound direct calls to in-tree function definitions.
**C++** SHALL emit no call edges (deferred whole to a build-integration decision), and that
absence is part of this contract, not an omission.

#### Scenario: Coverage is disclosed where consumers read

- **WHEN** an operator or consumer consults the code-call-graph documentation for a supported
  language
- **THEN** the resolvable call forms and the deliberately-dropped forms for that language are
  stated, including C++'s total absence

### Requirement: Java instance calls resolve through declared receiver types

For Java, a call `x.m(...)` where `x` is a local variable, parameter, or field with an explicit
declared type SHALL resolve `m` against that declared type using the same nearest-declaring-
ancestor walk as own-class calls, and SHALL be dropped (no edge) when the declared type cannot be
bound in-tree, when the receiver's type is generic or inferred, or when more than one candidate
declares `m` at the same depth. A resolved edge MUST byte-match the entity ID of the callee's
definition.

#### Scenario: Local-variable receiver resolves

- **GIVEN** a Java method declares `ModulePermissions p = ...` and calls `p.cloneAsTemplatePermission(...)`
- **AND** `ModulePermissions` declares `cloneAsTemplatePermission` in-tree
- **WHEN** the file is parsed
- **THEN** the calling method's `calls` includes the entity ID of
  `ModulePermissions.cloneAsTemplatePermission`, and `code_impact` on that method names the caller

#### Scenario: Ambiguous or unbindable receivers stay inert

- **WHEN** the receiver's declared type is not defined in-tree, is a type variable, or two
  ancestors at equal depth declare the method
- **THEN** no call edge is emitted for that call site

### Requirement: Function-typed parameters never become callees

A call whose callee identifier binds to a function-typed parameter or function-typed local
variable of the enclosing scope SHALL NOT produce a call edge in any language: the parameter is
not a definition, so any such edge dangles by construction.

#### Scenario: Go function-valued parameter call is inert

- **GIVEN** a Go function `decide(..., stat func(string) bool)` whose body calls `stat(p)`
- **WHEN** the file is parsed
- **THEN** no `code.relationship.calls` edge names a `stat` entity, and graph-query resolution
  logs no missing-entity error for it

### Requirement: TypeScript and Svelte emit deterministic call edges

TS/JS and Svelte function and method bodies SHALL emit call edges for: direct calls to functions
defined in the same file; direct calls bound by in-tree import bindings, resolved through the
existing module→file machinery; and `this.`-receiver method calls resolved on the own class.
Everything else — property-chain receivers, dynamically computed callees, framework dispatch —
SHALL stay inert. Svelte components SHALL apply the same pass to their script blocks.

#### Scenario: Cross-module TS import call resolves

- **GIVEN** `lib/util.ts` exports `function helper()` and `lib/app.ts` imports and calls it
- **WHEN** `lib/app.ts` is parsed
- **THEN** the caller's `calls` includes `helper`'s entity ID built against `lib/util.ts`

#### Scenario: Svelte script block participates

- **GIVEN** a `.svelte` component whose script imports an in-tree function and calls it
- **WHEN** the component is parsed
- **THEN** the same call edge is emitted as for an equivalent `.ts` module

### Requirement: Model-assisted edge synthesis is prohibited

Call-edge extraction SHALL be a deterministic function of the corpus. No language pass may use a
model (LLM or learned ranker) to infer, disambiguate, or "recover" call edges: a guessed edge is
a fabricated fact, and zero fabrication outranks coverage. Ambiguity is always resolved by
dropping the edge.

#### Scenario: Ambiguity never escalates to inference

- **WHEN** a call site cannot be resolved by the deterministic rules of its language
- **THEN** the call site produces no edge, and no fallback inference path exists to produce one
