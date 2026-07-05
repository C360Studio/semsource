# code-reference-resolution Specification

## MODIFIED Requirements

### Requirement: Reference edges resolve to the definition's entity ID
The AST layer SHALL set every emitted type-dependency edge
(`code.relationship.extends`, `code.relationship.implements`,
`code.relationship.references`, `code.relationship.embeds`) to the exact
deterministic entity ID of the referenced symbol's **definition** — never an ID
derived from the referrer's location. A referenced name that resolves MUST produce
a target ID byte-identical to the ID the definition constructs for itself (the same
`NewCodeEntity` path: 6-part ID, `SystemSlug(project)` system segment, definition
entity-type segment). This holds for **every supported language** — Python, Java,
TypeScript/JavaScript, and Go — not only Python.

#### Scenario: Same-file base class
- **WHEN** a Python file defines `class Base:` and `class Derived(Base):`
- **THEN** `Derived`'s `extends` edge target equals `Base`'s own entity ID
- **AND** the edge resolves in the graph (relations show `Derived` under `Base`'s `extended_by`)

#### Scenario: Reference id must equal definition id
- **WHEN** a type reference is resolved to a definition in a known file
- **THEN** the constructed target ID is identical to that definition's entity ID
  (segment-for-segment), so the edge is not dangling

#### Scenario: Java / TS same-file hierarchy edge equals definition id
- **WHEN** a Java or TypeScript file defines a base `class Animal` (or interface)
  and a subtype in the same file (`class Dog extends Animal`, `class C implements I`)
- **THEN** the subtype's `extends`/`implements` target equals the base's own entity
  ID segment-for-segment — the kind segment is `class`/`interface` (not `type`) and
  the system segment is `SystemSlug(project)` (not the raw project)

#### Scenario: Go same-file embed equals definition id
- **WHEN** a Go file defines `type Base struct{}` and `type Derived struct{ Base }`
- **THEN** `Derived`'s `embeds` target equals `Base`'s own entity ID — the kind
  segment is `struct` (the definition's real kind), not the hard-coded `type`

## ADDED Requirements

### Requirement: Type-dependency edge kind is inferred from syntactic position
The AST layer SHALL infer a type-dependency edge's target kind from syntactic
position wherever the grammar makes it unambiguous, so the kind segment matches the
definition without parsing the target. A class
`extends` clause targets a class; an `implements` clause targets an interface; an
interface `extends` clause targets an interface. For references whose kind is not
knowable from position (a field/parameter/return/alias type), a **same-file**
definition SHALL be resolved to its real kind via the file's own definitions; a
cross-file reference of unknown kind SHALL stay inert rather than guess.

#### Scenario: implements targets an interface, extends targets a class
- **WHEN** a Java or TS class declares `extends Base` and `implements Iface`
- **THEN** the `extends` target is built with kind `class` and the `implements`
  target with kind `interface`, each matching its definition's kind segment

#### Scenario: interface extends targets an interface
- **WHEN** a Java or TS interface declares `extends Other`
- **THEN** the `extends` target is built with kind `interface`

#### Scenario: same-file unknown-kind reference resolves via the file's definitions
- **WHEN** a field/return type names a class/interface/enum defined in the same file
- **THEN** the reference resolves to that definition's real kind and its entity ID

### Requirement: Cross-file references resolve per-language to the defining file
The AST layer SHALL resolve a cross-file type reference — in Java, TypeScript/
JavaScript, and Go — against the **defining file's** repo-root-relative path using a
language-appropriate resolver, so the edge target byte-matches that definition.
Resolution stays
per-file / per-directory and filesystem-driven (no whole-project index), and is
order-independent because entity IDs are intrinsic and deterministic.

#### Scenario: Java import resolves to the package file
- **GIVEN** `a/b/Animal.java` defines `class Animal`
- **AND** `x/Dog.java` contains `import a.b.Animal;` and `class Dog extends Animal`
- **WHEN** `x/Dog.java` is parsed
- **THEN** `Dog`'s `extends` target equals `Animal`'s entity ID (built against `a/b/Animal.java`)

#### Scenario: Java same-package reference without an import
- **GIVEN** `a/b/Animal.java` and `a/b/Dog.java` share package `a.b`
- **AND** `Dog.java` contains `class Dog extends Animal` (no import — same package)
- **THEN** `Dog`'s `extends` target resolves to `Animal`'s entity ID via the referrer's package directory

#### Scenario: TS relative import resolves to the module file
- **GIVEN** `base.ts` exports `class Base`
- **AND** `client.ts` contains `import { Base } from './base'` and `class Derived extends Base`
- **WHEN** `client.ts` is parsed
- **THEN** `Derived`'s `extends` target equals `Base`'s entity ID (built against `base.ts`)

#### Scenario: Go same-package cross-file embed
- **GIVEN** `a.go` defines `type Base struct{}` and `b.go` (same directory/package)
  defines `type Derived struct{ Base }`
- **WHEN** `b.go` is parsed
- **THEN** `Derived`'s `embeds` target equals `Base`'s entity ID (built against `a.go` with kind `struct`)

#### Scenario: Out-of-tree and out-of-scope references stay inert
- **WHEN** a reference resolves only via a bare/`node_modules` specifier, a
  fully-qualified stdlib/third-party name, a wildcard/star import, a re-export
  through a barrel/package, a namespace package, or a tsconfig path alias
- **THEN** the reference is left `external:`/`builtin:`/inert — never mapped to a wrong in-tree entity
