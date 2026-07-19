# Delta: code-call-graph

## ADDED Requirements

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
