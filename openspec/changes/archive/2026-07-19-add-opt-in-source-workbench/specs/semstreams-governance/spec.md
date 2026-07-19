## MODIFIED Requirements

### Requirement: Current SemStreams target is explicit

SemSource MUST target released SemStreams `v1.0.0-beta.153` for this migration and MUST NOT use a
local replacement, fork, vendored substitute, or unreleased commit as compatibility evidence.

#### Scenario: Migration target is pinned to a release

**GIVEN** SemStreams has released `v1.0.0-beta.153`
**WHEN** SemSource completes this migration
**THEN** `go.mod` requires `github.com/c360studio/semstreams v1.0.0-beta.153`
**AND** the module has no `replace` directive
