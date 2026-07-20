# Delta: typed-source-change-events

## ADDED Requirements

### Requirement: Delete and rename events publish typed state changes

Watch and periodic-reindex paths SHALL treat file deletion and rename as first-class typed change
events that publish the staleness marker for affected entities — never silently discarded
(the audit found OpDelete events dropped on the floor).

#### Scenario: Delete event reaches the graph

- **WHEN** a watcher or reindex pass observes that a previously indexed file is gone
- **THEN** a typed change event publishes staleness markers for that file's entities within one
  index interval
