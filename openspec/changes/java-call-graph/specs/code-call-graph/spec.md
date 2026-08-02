# code-call-graph delta — Java

## ADDED Requirements

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
