## ADDED Requirements

### Requirement: A repo source's index interval governs its ast child
When a `repo` source declares `index_interval`, the `ast` source it expands into SHALL use that interval, for single-branch and per-branch expansion alike. A repo source that leaves `index_interval` unset MUST leave it unset on the `ast` child, so the spawn default applies.

#### Scenario: A declared interval survives expansion
- **WHEN** a `repo` source with `watch: true` declares `index_interval: 30m`
- **THEN** every `ast` source it expands into has `index_interval` `30m`

#### Scenario: An unset interval keeps the spawn default
- **WHEN** a `repo` source with `watch: true` does not declare `index_interval`
- **THEN** the `ast` source it expands into has no `index_interval` and is spawned with the default
