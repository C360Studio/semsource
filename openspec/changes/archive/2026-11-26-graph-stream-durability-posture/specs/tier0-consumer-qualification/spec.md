## REMOVED Requirements

### Requirement: GRAPH stream overrides stay on the ingest subjects

**Reason**: `streams.GRAPH.subjects` no longer exists. The GRAPH subjects are fixed to the five `graph.ingest.*`
subjects, so no override can bind a request/reply subject SemSource serves.

**Migration**: remove `streams` from `semsource.json`; use `graph_stream.posture` and `graph_stream.max_bytes`.
