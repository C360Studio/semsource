# Delta: code-call-graph

## ADDED Requirements

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
