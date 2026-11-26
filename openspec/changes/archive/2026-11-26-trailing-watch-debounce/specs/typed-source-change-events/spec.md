## ADDED Requirements

### Requirement: A watched file emits one change after it goes quiet
The shared file watcher SHALL emit a change for a path only once the path has had no filesystem event for the configured debounce delay, or once five times that delay has passed since the path's first pending event. A burst of writes to one file whose gaps are each shorter than the delay MUST produce one change event.

#### Scenario: A spread-out burst produces one event
- **GIVEN** a watched directory with a 300 ms debounce delay
- **WHEN** a file is written five times, 100 ms apart
- **THEN** the watcher emits one change event for that file, after the last write

#### Scenario: A continuously written file still flushes
- **GIVEN** a watched directory with a 100 ms debounce delay
- **WHEN** a file is written every 50 ms for one second
- **THEN** the watcher emits at least one change event for that file before the writes stop
