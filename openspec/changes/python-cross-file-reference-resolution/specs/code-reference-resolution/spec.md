# code-reference-resolution Specification

## ADDED Requirements

### Requirement: Reference edges resolve to the definition's entity ID
The AST layer SHALL set every emitted type-dependency edge
(`code.relationship.extends`, `code.relationship.implements`,
`code.relationship.references`) to the exact deterministic entity ID of the
referenced symbol's **definition** — never an ID derived from the referrer's
location. A referenced name that resolves MUST produce a target ID byte-identical
to the ID the definition constructs for itself (same `NewCodeEntity` path: 6-part
ID, `SystemSlug(project)` system segment, definition entity-type segment).

#### Scenario: Same-file base class
- **WHEN** a Python file defines `class Base:` and `class Derived(Base):`
- **THEN** `Derived`'s `extends` edge target equals `Base`'s own entity ID
- **AND** the edge resolves in the graph (relations show `Derived` under `Base`'s `extended_by`)

#### Scenario: Reference id must equal definition id
- **WHEN** a type reference is resolved to a definition in a known file
- **THEN** the constructed target ID is identical to that definition's entity ID
  (segment-for-segment), so the edge is not dangling

### Requirement: Imported type references resolve to their defining module
When a referenced type name is bound by an in-tree import, the AST layer SHALL
resolve the edge target against the **defining module's file**, so cross-file
`extends` / `references` edges connect. Resolution SHALL use the referrer file's own
import bindings and a module-name→file resolver rooted at the ingested source root.

#### Scenario: Cross-module base class via from-import
- **GIVEN** `pkg/base.py` defines `class BaseClient:`
- **AND** `pkg/client.py` contains `from pkg.base import BaseClient` and `class AsyncClient(BaseClient):`
- **WHEN** `pkg/client.py` is parsed
- **THEN** `AsyncClient`'s `extends` target equals `BaseClient`'s entity ID (built against `pkg/base.py`)

#### Scenario: Recognized import binding forms
- **WHEN** a name is bound by `import m`, `import m as a`, `from m import N`, `from m import N as A`, or a relative import (`from . import x`, `from ..p import y`)
- **THEN** the resolver maps the locally-bound name to its origin module and original name for target-ID construction

#### Scenario: Order-independent resolution
- **WHEN** the referrer file is parsed before the file that defines the referenced type
- **THEN** the edge target is still the correct definition ID (IDs are intrinsic and deterministic), and the edge resolves once both entities are present

### Requirement: Unresolvable references never produce a wrong edge
A reference that cannot be resolved to an in-tree definition SHALL NOT be silently
mapped to an incorrect entity. It SHALL remain inert — either a dangling target the
graph engine drops, or an `external:` / `builtin:` marker — never a target that
resolves to a different real entity.

#### Scenario: Third-party / stdlib reference
- **WHEN** a referenced type is imported from a module that does not resolve within the source root (e.g. a stdlib or third-party package)
- **THEN** the reference is left as an external/inert target and no edge to an in-tree entity is emitted

#### Scenario: Out-of-scope reference forms stay inert
- **WHEN** a name would only be resolvable via a star-import (`from m import *`), a re-export through a package `__init__`, or a dynamic import
- **THEN** the reference is left inert (no edge), not resolved to a wrong target
