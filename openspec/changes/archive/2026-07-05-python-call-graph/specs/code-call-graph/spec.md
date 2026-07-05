# code-call-graph Specification

## ADDED Requirements

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
