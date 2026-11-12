// Package cutover holds the reviewed classification of SemEngine
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
// Deliberately NOT derived from kvcatalog.KVCatalog() at runtime:
// deriving it is exactly the silent widening the interlock exists to prevent.
package cutover

// Purged lists framework-owned buckets a cutover deletes. Everything here is
// derived state that re-derives from source on the next seed, so destroying it
// costs ingest time and nothing else.
var Purged = []string{
	"ENTITY_STATES",
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
	"GRAPH_STATUS",
	"STORAGE_REPORT",
}

// Retained lists framework-owned buckets a cutover deliberately does NOT
// delete, despite the framework owning them.
//
// semengine_config includes the create-once deployment authority. A graph-only
// reset must retain it; a full deployment cutover provisions a separate store.
var Retained = []string{
	"semengine_config",
}
