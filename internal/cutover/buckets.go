// Package cutover holds the reviewed classification of SemStreams
// framework-owned KV buckets for the documented upgrade path — purge graph
// state, restart, let the graph re-derive from source.
//
// The classification is a SAFETY INTERLOCK on a destructive operation, not a
// mirror of the framework's catalog. Every bucket the framework owns must be
// explicitly listed as either purged or retained. A bucket the framework adds
// appears in neither, which fails TestFrameworkBucketsAreClassified and forces
// a human to decide whether a cutover should destroy it — instead of the
// deletion boundary silently widening on a dependency bump.
//
// Deliberately NOT derived from graph.FrameworkOwnedBuckets() at runtime:
// deriving it is exactly the silent widening the interlock exists to prevent.
package cutover

// Purged lists framework-owned buckets a cutover deletes. Everything here is
// derived state that re-derives from source on the next seed, so destroying it
// costs ingest time and nothing else.
var Purged = []string{
	"ENTITY_STATES",
	"ENTITY_SUFFIX_INDEX",
	"GRAPH_INGEST_APPLIED_SEQ",
	"OUTGOING_INDEX",
	"INCOMING_INDEX",
	"ALIAS_INDEX",
	"PREDICATE_INDEX",
	"NAME_INDEX",
	"SPATIAL_INDEX",
	"TEMPORAL_INDEX",
	"TEMPORAL_INDEX_REVERSE",
	"EMBEDDING_INDEX",
	"EMBEDDING_DEDUP",
	"COMMUNITY_INDEX",
	"COMMUNITY_SUMMARIES",
	"ANOMALY_INDEX",
	"GRAPH_STATUS",
	"STORAGE_REPORT",
}

// Retained lists framework-owned buckets a cutover deliberately does NOT
// delete, despite the framework owning them.
//
// TOOL_CALL_OUTCOMES (reviewed at the beta.161 bump): agentic-tools' immutable
// COMPLETED ledger for durable tool-result replay, keyed by opaque hashes of
// ToolCall.ID. It is the one entry in the catalog that does NOT re-derive from
// source — every other bucket rebuilds on re-ingest, while a completed-call
// ledger cannot be reconstructed by re-reading repositories. Destroying it
// would lose the record of what had already run, which is the single thing a
// replay ledger exists to preserve.
//
// It is also written by semstreams' agentic-tools processor, not by SemSource.
// A SemSource cutover deleting another component's durable state would reach
// outside its own blast radius. Retaining it also proves the cutover does not
// overreach, which is worth an assertion in its own right.
var Retained = []string{
	"TOOL_CALL_OUTCOMES",
}
